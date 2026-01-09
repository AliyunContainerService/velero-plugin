package main

import (
	"context"
	"fmt"
	"os"

	"github.com/AliyunContainerService/ack-ram-tool/pkg/ecsmetadata"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
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

	kindKey                  = "kind"
	persistentVolumeKey      = "PersistentVolume"
	persistentVolumeClaimKey = "PersistentVolumeClaim"

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
func getOssEndpoint(config map[string]string) string {

	if endpoint := config[endpointConfigKey]; endpoint != "" {
		return endpoint
	}

	region := getEcsRegionID(config)
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
	// AliyunCSVeleroRole
	ctx := context.Background()
	roleInfo, err := MetaClient.GetRoleCredentials(ctx, ramrole)
	if err != nil {
		return "", "", "", err
	}
	return roleInfo.AccessKeyId, roleInfo.AccessKeySecret, roleInfo.SecurityToken, nil
}

// credentials holds OSS authentication credentials
type credentials struct {
	accessKeyID     string
	accessKeySecret string
	stsToken        string
	ramRole         string
}

// getCredentials retrieves OSS credentials based on the environment and configuration
// It supports multiple authentication methods:
// 1. Load credentials from file (if ALIBABA_CLOUD_CREDENTIALS_FILE is set) and/or environment variables
// 2. For ACK environments: fallback to RAM role credentials if env credentials are not available
// 3. For non-ACK environments: return error if env credentials are not available
func getCredentials(veleroForAck bool) (*credentials, error) {
	cred := &credentials{}

	// Step 1: Load credentials from file if specified (this may set env vars)
	if err := loadCredentialFileFromEnv(); err != nil {
		return nil, err
	}

	// Step 1.2: Get credentials from environment variables
	// These may be set by loadCredentialFileFromEnv or directly by the user
	cred.accessKeyID = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")
	cred.accessKeySecret = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")
	cred.stsToken = os.Getenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN") // Token may be empty

	// Step 1.3: If we have both accessKeyID and accessKeySecret, we can return
	if len(cred.accessKeyID) != 0 && len(cred.accessKeySecret) != 0 {
		return cred, nil
	}

	// Step 2: Handle cases where credentials are not available from env
	if !veleroForAck {
		// For non-ACK environment: if credentials are not available, return error
		return nil, errors.Errorf("ALIBABA_CLOUD_ACCESS_KEY_ID or ALIBABA_CLOUD_ACCESS_KEY_SECRET environment variable is not set")
	}

	// For ACK environment: try to get credentials from RAM role
	ramRole, err := getRamRole()
	if err != nil {
		return nil, errors.Errorf("Failed to get ram role with err: %v", err)
	}
	cred.ramRole = ramRole
	cred.accessKeyID, cred.accessKeySecret, cred.stsToken, err = getSTSAK(ramRole)
	if err != nil {
		return nil, errors.Errorf("Failed to get sts token from ram role %s with err: %v", ramRole, err)
	}
	return cred, nil
}

func updateOssClient(ramRole string, endpoint string, client bucketGetter) (bucketGetter, error) {
	bucketGetter := &ossBucketGetter{}
	if len(ramRole) == 0 {
		return client, nil
	}
	accessKeyID, accessKeySecret, stsToken, err := getSTSAK(ramRole)
	if err != nil {
		return nil, err
	}
	ossClient, err := oss.New(endpoint, accessKeyID, accessKeySecret, oss.SecurityToken(stsToken))
	if err != nil {
		return nil, err
	}

	bucketGetter.client = ossClient
	return bucketGetter, err
}
