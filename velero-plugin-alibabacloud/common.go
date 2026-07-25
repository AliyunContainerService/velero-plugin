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

	// Optional config keys for per-BSL/VSL credentials.
	// Velero v1.10+ supports spec.credential on BSL/VSL objects, which references a
	// Kubernetes Secret. Velero mounts the secret and injects credentialsFile into the
	// plugin config. These keys are the alternative path for passing credentials directly
	// via the BSL/VSL config map (lower precedence than credentialsFile/env vars from
	// a mounted secret). When both accessKeyId and accessKeySecret are present, they
	// take highest priority over all other credential sources for that location.
	accessKeyIDConfigKey     = "accessKeyId"
	accessKeySecretConfigKey = "accessKeySecret"
	stsTokenConfigKey        = "stsToken"

	networkTypeAccelerate = "accelerate"
	networkTypeInternal   = "internal"

	DefaultRegion = "cn-hangzhou"

	kindKey             = "kind"
	persistentVolumeKey = "PersistentVolume"

	// Constants for volume ID conversion
	OriginStr = "volumeId"
	TargetStr = "VolumeId"
)

var validConfigKeys = []string{
	regionConfigKey,
	zoneConfigKey,
	networkTypeConfigKey,
	endpointConfigKey,
	notOnECSConfigKey,
	credFileConfigKey,
	accessKeyIDConfigKey,
	accessKeySecretConfigKey,
	stsTokenConfigKey,
}

// loadCredentialFileFromEnv loads environment variables from a credentials file.
// The file path can be specified either via config["credentialsFile"] or the
// ALIBABA_CLOUD_CREDENTIALS_FILE environment variable. Config takes precedence.
func loadCredentialFileFromEnv(config map[string]string) error {
	var filePath string
	if config != nil && config[credFileConfigKey] != "" {
		filePath = config[credFileConfigKey]
	} else {
		// Deprecated
		filePath = os.Getenv("ALIBABA_CLOUD_CREDENTIALS_FILE")
	}
	if filePath == "" {
		return nil
	}

	if err := godotenv.Overload(filePath); err != nil {
		return errors.Wrapf(err, "error loading credientials file (%s)", filePath)
	}

	return nil
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

// getCredentials retrieves OSS credentials based on the environment and configuration.
// It supports two usage patterns:
//
// Pattern 1 — Shared credential (existing approach, recommended for most cases):
//   - A single Kubernetes Secret is referenced by the Velero installation (e.g. --secret-file).
//   - Both BSL and VSL use the same credential source.
//   - Credential resolution order: credentialsFile config key → ALIBABA_CLOUD_CREDENTIALS_FILE
//     env var → ALIBABA_CLOUD_ACCESS_KEY_ID/SECRET env vars → ALIBABA_CLOUD_RAM_ROLE env var
//     → ECS instance RAM role (ACK only).
//
// Pattern 2 — Per-location credential via Kubernetes Secret (new approach):
//   - Each BSL/VSL has its own spec.credential field referencing a separate Kubernetes Secret.
//   - Velero v1.10+ mounts the secret and injects credentialsFile=/tmp/... into plugin config.
//   - The plugin reads the file via loadCredentialFileFromEnv (step 2 below).
//   - This allows BSL and VSL to use different credentials (e.g. different RAM users).
//
// Full credential resolution priority order within this function:
//
// 1. Per-location config keys (accessKeyId + accessKeySecret in BSL/VSL config map):
//   - When both are present, they are used directly as the highest-priority fallback.
//   - Note: the recommended per-location approach is Pattern 2 above (spec.credential).
//     These config keys are an alternative when a Kubernetes Secret is not available.
//
// 2. Credentials file (Pattern 2 — spec.credential path lands here):
//   - Config key: credentialsFile (takes precedence over ALIBABA_CLOUD_CREDENTIALS_FILE env var)
//   - File format: dotenv key=value pairs (ALIBABA_CLOUD_ACCESS_KEY_ID, etc.)
//
// 3. AccessKey credentials from environment variables:
//   - ALIBABA_CLOUD_ACCESS_KEY_ID, ALIBABA_CLOUD_ACCESS_KEY_SECRET (required)
//   - ALIBABA_CLOUD_ACCESS_STS_TOKEN (optional)
//
// 4. Custom RAM Role (via environment variable):
//   - ALIBABA_CLOUD_RAM_ROLE — custom RAM role name
//
// 5. ECS Instance RAM Role (ACK environment fallback):
//   - Automatically detected from ECS metadata service
//
// 6. Error (non-ACK environment without any credentials)
func getCredentials(config map[string]string) (*ossCredentials, error) {
	cred := &ossCredentials{}

	// Step 1: Per-location config keys (highest priority).
	// If accessKeyId and accessKeySecret are both present in the BSL/VSL config map,
	// use them directly — credentialsFile and env vars are not consulted for this location.
	if config != nil && config[accessKeyIDConfigKey] != "" && config[accessKeySecretConfigKey] != "" {
		cred.accessKeyID = config[accessKeyIDConfigKey]
		cred.accessKeySecret = config[accessKeySecretConfigKey]
		cred.stsToken = config[stsTokenConfigKey] // optional
		return cred, nil
	}

	// Step 2: Load credentials from file if specified (this may set env vars)
	if err := loadCredentialFileFromEnv(config); err != nil {
		return nil, err
	}

	// Step 3: Get credentials from environment variables
	// These may be set by loadCredentialFileFromEnv or directly by the user
	cred.accessKeyID = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")
	cred.accessKeySecret = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")
	cred.stsToken = os.Getenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN") // Token may be empty
	cred.ramRole = os.Getenv("ALIBABA_CLOUD_RAM_ROLE")          // Custom RAM role name

	// Step 4: If we have both accessKeyID and accessKeySecret, use them directly
	// AccessKey credentials take precedence over RAM role
	if len(cred.accessKeyID) != 0 && len(cred.accessKeySecret) != 0 {
		cred.ramRole = ""
		return cred, nil
	}

	// Step 5: Handle RAM role authentication
	// If no AccessKey credentials are available, try to use RAM role
	if !veleroForAck(config) && cred.ramRole == "" {
		// For non-ACK environment: if no AccessKey and no custom RAM role, return error
		return nil, errors.Errorf("ALIBABA_CLOUD_ACCESS_KEY_ID or ALIBABA_CLOUD_ACCESS_KEY_SECRET environment variable is not set")
	}

	// Determine which RAM role to use:
	// - If custom RAM role is specified via ALIBABA_CLOUD_RAM_ROLE, use it
	// - Otherwise, for ACK environment, try to get RAM role from ECS metadata
	if cred.ramRole == "" {
		ramRole, err := getRamRole()
		if err != nil {
			return nil, errors.Errorf("Failed to get ram role with err: %v", err)
		}
		cred.ramRole = ramRole
	}

	// Step 6: Get STS credentials from the RAM role
	var err error
	cred.accessKeyID, cred.accessKeySecret, cred.stsToken, err = getSTSAK(cred.ramRole)
	if err != nil {
		return nil, errors.Errorf("Failed to get sts token from ram role %s with err: %v", cred.ramRole, err)
	}
	return cred, nil
}
