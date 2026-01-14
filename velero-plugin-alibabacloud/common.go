package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/AliyunContainerService/ack-ram-tool/pkg/ecsmetadata"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

var MetaClient = ecsmetadata.DefaultClient
var MetaRegion string

const (
	regionConfigKey      = "region"
	networkTypeConfigKey = "network"
	endpointConfigKey    = "endpoint"

	networkTypeAccelerate = "accelerate"
	networkTypeInternal   = "internal"

	DefaultRegion = "cn-hangzhou"

	kindKey             = "kind"
	persistentVolumeKey = "PersistentVolume"

	// Constants for volume ID conversion
	OriginStr = "volumeId"
	TargetStr = "VolumeId"
)

// load environment vars from $ALIBABA_CLOUD_CREDENTIALS_FILE, if it exists
func loadCredentialFileFromEnv() error {
	envFile := os.Getenv("ALIBABA_CLOUD_CREDENTIALS_FILE")
	if envFile == "" {
		return nil
	}

	if err := godotenv.Overload(envFile); err != nil {
		return errors.Wrapf(err, "error loading environment from ALIBABA_CLOUD_CREDENTIALS_FILE (%s)", envFile)
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

// getCredentials retrieves OSS credentials based on the environment and configuration.
// It supports multiple authentication methods with the following priority order:
//
// 1. AccessKey credentials (highest priority):
//   - Load from file (if ALIBABA_CLOUD_CREDENTIALS_FILE is set) and/or environment variables
//   - Environment variables: ALIBABA_CLOUD_ACCESS_KEY_ID, ALIBABA_CLOUD_ACCESS_KEY_SECRET
//   - Optional: ALIBABA_CLOUD_ACCESS_STS_TOKEN
//   - If both AccessKey ID and Secret are provided, they take precedence over RAM role
//
// 2. Custom RAM Role (via environment variable):
//   - Environment variable: ALIBABA_CLOUD_RAM_ROLE
//   - Allows specifying a custom RAM role name instead of using the ECS instance's default role
//   - Works in both ACK and non-ACK environments
//   - The function will use this role to obtain STS credentials via getSTSAK()
//
// 3. ECS Instance RAM Role (ACK environment fallback):
//   - For ACK environments: automatically detect the RAM role from ECS metadata
//   - Only used if no AccessKey credentials and no custom RAM role are provided
//   - Requires the ECS instance to have a RAM role attached
//
// 4. Error (non-ACK environment without credentials):
//   - For non-ACK environments: returns error if no AccessKey and no custom RAM role are provided
//
// Parameters:
//   - veleroForAck: indicates if running in ACK (Alibaba Cloud Container Service) environment
//
// Returns:
//   - ossCredentials: contains accessKeyID, accessKeySecret, stsToken, and ramRole
//   - error: if credentials cannot be obtained
func getCredentials(veleroForAck bool) (*ossCredentials, error) {
	cred := &ossCredentials{}

	// Step 1: Load credentials from file if specified (this may set env vars)
	if err := loadCredentialFileFromEnv(); err != nil {
		return nil, err
	}

	// Step 2: Get credentials from environment variables
	// These may be set by loadCredentialFileFromEnv or directly by the user
	cred.accessKeyID = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")
	cred.accessKeySecret = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")
	cred.stsToken = os.Getenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN") // Token may be empty
	cred.ramRole = os.Getenv("ALIBABA_CLOUD_RAM_ROLE")          // Custom RAM role name

	// Step 3: If we have both accessKeyID and accessKeySecret, use them directly
	// AccessKey credentials take precedence over RAM role
	if len(cred.accessKeyID) != 0 && len(cred.accessKeySecret) != 0 {
		cred.ramRole = ""
		return cred, nil
	}

	// Step 4: Handle RAM role authentication
	// If no AccessKey credentials are available, try to use RAM role
	if !veleroForAck && cred.ramRole == "" {
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

	// Step 5: Get STS credentials from the RAM role
	var err error
	cred.accessKeyID, cred.accessKeySecret, cred.stsToken, err = getSTSAK(cred.ramRole)
	if err != nil {
		return nil, errors.Errorf("Failed to get sts token from ram role %s with err: %v", cred.ramRole, err)
	}
	return cred, nil
}
