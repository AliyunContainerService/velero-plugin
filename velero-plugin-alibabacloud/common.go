package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AliyunContainerService/ack-ram-tool/pkg/ecsmetadata"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

var MetaClient = ecsmetadata.DefaultClient
var MetaRegion string
var MetaZone string

const (
	regionConfigKey      = "region"
	zoneConfigKey        = "zone"
	networkTypeConfigKey = "network"
	endpointConfigKey    = "endpoint"
	notOnECSConfigKey    = "notOnECS"
	credFileConfigKey    = "credentialsFile"

	networkTypeAccelerate = "accelerate"
	networkTypeInternal   = "internal"

	DefaultRegion = "cn-hangzhou"

	kindKey             = "kind"
	persistentVolumeKey = "PersistentVolume"

	// Constants for volume ID conversion
	OriginStr = "volumeId"
	TargetStr = "VolumeId"

	// Credential environment variable / file key names
	envAccessKeyID     = "ALIBABA_CLOUD_ACCESS_KEY_ID"
	envAccessKeySecret = "ALIBABA_CLOUD_ACCESS_KEY_SECRET"
	envStsToken        = "ALIBABA_CLOUD_ACCESS_STS_TOKEN"
	envRamRole         = "ALIBABA_CLOUD_RAM_ROLE"
	envCredFile        = "ALIBABA_CLOUD_CREDENTIALS_FILE"
)

var validConfigKeys = []string{
	regionConfigKey,
	zoneConfigKey,
	networkTypeConfigKey,
	endpointConfigKey,
	notOnECSConfigKey,
	credFileConfigKey,
}

// getCredFilePath returns the credentials file path from config or environment.
// Config takes precedence over ALIBABA_CLOUD_CREDENTIALS_FILE env var.
func getCredFilePath(config map[string]string) string {
	if config != nil && config[credFileConfigKey] != "" {
		return config[credFileConfigKey]
	}
	// Deprecated
	return os.Getenv(envCredFile)
}

// readCredentialFile reads credentials from a file without polluting process-wide
// environment variables. This enables per-location credential isolation when
// BSL and VSL reference different Kubernetes Secrets via spec.credential.
func readCredentialFile(filePath string) (map[string]string, error) {
	envMap, err := godotenv.Read(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "error loading credentials file (%s)", filePath)
	}
	return envMap, nil
}

// getOssEndpoint:
// return customized oss endpoint
// return oss public endpoint in format "oss-%s.aliyuncs.com"
// return oss accelerate endpoint in format "oss-accelerate.aliyuncs.com"
// return oss internal endpoint in format "oss-%s-internal.aliyuncs.com"
func getOssEndpoint(region string, config map[string]string) string {

	if endpoint := config[endpointConfigKey]; endpoint != "" {
		return endpoint
	}

	if region == "" {
		region = DefaultRegion
	}

	switch config[networkTypeConfigKey] {
	case networkTypeInternal:
		return fmt.Sprintf("https://oss-%s-internal.aliyuncs.com", region)

	case networkTypeAccelerate:
		return "https://oss-accelerate.aliyuncs.com"
	default:
		return fmt.Sprintf("https://oss-%s.aliyuncs.com", region)
	}

}

// getEcsRegionID return ecs region id
func getEcsRegionID(config map[string]string) string {
	region := config[regionConfigKey]
	if region != "" {
		return region
	}

	if MetaRegion != "" {
		return MetaRegion
	}
	region, err := MetaClient.GetRegionId(context.Background())
	if err != nil {
		klog.Errorf("get MetaRegion failed with error: %v", err)
		return ""
	}

	klog.Infof("set MetaRegion to %s", region)
	MetaRegion = region
	return region
}

// getEcsZoneID return ecs region id
func getEcsZoneID(config map[string]string) string {
	zone := config[zoneConfigKey]
	if zone != "" {
		return zone
	}

	if MetaZone != "" {
		return MetaZone
	}
	zone, err := MetaClient.GetZoneId(context.Background())
	if err != nil {
		klog.Errorf("get MetaZone failed with error: %v", err)
		return ""
	}

	klog.Infof("set MetaZone to %s", zone)
	MetaZone = zone
	return zone
}

// getRamRole return ramrole name
func getRamRole() (string, error) {
	return MetaClient.GetRoleName(context.Background())
}

// getSTSAK return AccessKeyID, AccessKeySecret and SecurityToken
func getSTSAK(ramrole string) (string, string, string, error) {
	// Use context with timeout to avoid hanging in non-ECS environments
	// The timeout is set to 10 seconds to fail fast in test environments
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	roleInfo, err := MetaClient.GetRoleCredentials(ctx, ramrole)
	if err != nil {
		return "", "", "", err
	}
	return roleInfo.AccessKeyId, roleInfo.AccessKeySecret, roleInfo.SecurityToken, nil
}

// ossCredentials holds OSS authentication credentials
type ossCredentials struct {
	accessKeyID     string
	accessKeySecret string
	stsToken        string
	ramRole         string
}

func veleroForAck(config map[string]string) bool {
	if config != nil && strings.ToLower(config[notOnECSConfigKey]) == "true" {
		return false
	}
	// Deprecated
	return !(strings.ToLower(os.Getenv("VELERO_FOR_ACK")) == "false")
}

// getCredentials retrieves credentials based on the environment and configuration.
// It supports per-location credential isolation: each BSL/VSL can reference a different
// Kubernetes Secret via spec.credential, and Velero injects the file path into the
// config map under the credFileConfigKey key.
//
// Credential resolution priority order:
//
//  1. Credentials file (per-location, highest priority):
//     - Config key: credFileConfigKey (injected by Velero when spec.credential is set)
//     - Fallback: envCredFile environment variable
//     - File is read without setting process-wide env vars (isolation-safe)
//     - File format: dotenv key=value pairs (envAccessKeyID, envAccessKeySecret, etc.)
//
//  2. Process environment variables (shared, lower priority):
//     - envAccessKeyID, envAccessKeySecret
//     - Optional: envStsToken
//     - Only consulted when no credentials file is available
//
//  3. Custom RAM Role (via credentials file or environment variable):
//     - envRamRole — custom RAM role name
//     - Used to obtain STS credentials via ECS metadata service
//
//  4. ECS Instance RAM Role (ACK environment fallback):
//     - Automatically detected from ECS metadata service
//     - Only used if no AccessKey credentials and no custom RAM role are provided
//
//  5. Error (non-ACK environment without any credentials)
func getCredentials(config map[string]string) (*ossCredentials, error) {
	cred := &ossCredentials{}

	// Step 1: Try to load credentials from a per-location file.
	// This does NOT set process-wide env vars, enabling BSL/VSL credential isolation.
	if filePath := getCredFilePath(config); filePath != "" {
		envMap, err := readCredentialFile(filePath)
		if err != nil {
			return nil, err
		}
		cred.accessKeyID = envMap[envAccessKeyID]
		cred.accessKeySecret = envMap[envAccessKeySecret]
		cred.stsToken = envMap[envStsToken]
		cred.ramRole = envMap[envRamRole]
	} else {
		// Step 2: No credentials file — fall back to process environment variables.
		cred.accessKeyID = os.Getenv(envAccessKeyID)
		cred.accessKeySecret = os.Getenv(envAccessKeySecret)
		cred.stsToken = os.Getenv(envStsToken)
		cred.ramRole = os.Getenv(envRamRole)
	}

	// Step 3: If we have both accessKeyID and accessKeySecret, use them directly.
	// AccessKey credentials take precedence over RAM role.
	if len(cred.accessKeyID) != 0 && len(cred.accessKeySecret) != 0 {
		cred.ramRole = ""
		return cred, nil
	}

	// Step 4: Handle RAM role authentication.
	// If no AccessKey credentials are available, try to use RAM role.
	if !veleroForAck(config) && cred.ramRole == "" {
		return nil, errors.Errorf("%s or %s environment variable is not set", envAccessKeyID, envAccessKeySecret)
	}

	// Determine which RAM role to use:
	// - If custom RAM role is specified (from file or env), use it
	// - Otherwise, for ACK environment, try to get RAM role from ECS metadata
	if cred.ramRole == "" {
		ramRole, err := getRamRole()
		if err != nil {
			return nil, errors.Errorf("Failed to get ram role with err: %v", err)
		}
		cred.ramRole = ramRole
	}

	// Step 5: Get STS credentials from the RAM role
	var err error
	cred.accessKeyID, cred.accessKeySecret, cred.stsToken, err = getSTSAK(cred.ramRole)
	if err != nil {
		return nil, errors.Errorf("Failed to get sts token from ram role %s with err: %v", cred.ramRole, err)
	}
	return cred, nil
}
