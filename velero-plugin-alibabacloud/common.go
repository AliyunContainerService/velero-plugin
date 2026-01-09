package main

import (
	"encoding/json"
	"fmt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

const (
	metadataURL              = "http://100.100.100.200/latest/meta-data/"
	metadataRegionKey        = "region-id"
	metadataZoneKey          = "zone-id"
	regionConfigKey          = "region"
	minReqVolSizeBytes       = 21474836480
	minReqVolSizeString      = "20Gi"
	kindKey                  = "kind"
	persistentVolumeKey      = "PersistentVolume"
	persistentVolumeClaimKey = "PersistentVolumeClaim"
	networkTypeConfigKey     = "network"
	networkTypeAccelerate    = "accelerate"
	networkTypeInternal      = "internal"
)

// RoleAuth define STS Token Response
type RoleAuth struct {
	AccessKeyID     string
	AccessKeySecret string
	Expiration      time.Time
	SecurityToken   string
	LastUpdated     time.Time
	Code            string
}

// load environment vars from $ALIBABA_CLOUD_CREDENTIALS_FILE, if it exists
func loadEnv() error {
	envFile := os.Getenv("ALIBABA_CLOUD_CREDENTIALS_FILE")
	if envFile == "" {
		return nil
	}

	if err := godotenv.Overload(envFile); err != nil {
		return errors.Wrapf(err, "error loading environment from ALIBABA_CLOUD_CREDENTIALS_FILE (%s)", envFile)
	}

	return nil
}

// get region or available zone information
func getMetaData(resource string) (string, error) {
	resp, err := http.Get(metadataURL + resource)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// getOssEndpoint:
// return oss public endpoint in format "oss-%s.aliyuncs.com"
// return oss accelerate endpoint in format "oss-accelerate.aliyuncs.com"
// return oss internal endpoint in format "oss-%s-internal.aliyuncs.com"
func getOssEndpoint(config map[string]string) string {
	if networkType := config[networkTypeConfigKey]; networkType != "" {
		switch networkType {
		case networkTypeInternal:
			if value := config[regionConfigKey]; value != "" {
				return fmt.Sprintf("oss-%s-internal.aliyuncs.com", value)
			} else {
				if value, err := getMetaData(metadataRegionKey); err != nil || value == "" {
					// set default region
					return "oss-cn-hangzhou-internal.aliyuncs.com"
				}
			}
		case networkTypeAccelerate:
			return "oss-accelerate.aliyuncs.com"
		default:
			if value := config[regionConfigKey]; value != "" {
				return fmt.Sprintf("oss-%s.aliyuncs.com", value)
			} else {
				if value, err := getMetaData(metadataRegionKey); err != nil || value == "" {
					// set default region
					return "oss-cn-hangzhou.aliyuncs.com"
				}
			}
		}
	}

	if value := config[regionConfigKey]; value == "" {
		if value, err := getMetaData(metadataRegionKey); err != nil || value == "" {
			// set default region
			return "oss-cn-hangzhou.aliyuncs.com"
		} else {
			return fmt.Sprintf("oss-%s.aliyuncs.com", value)
		}
	} else {
		return fmt.Sprintf("oss-%s.aliyuncs.com", value)
	}
}

// getEcsRegionID return ecs region id
func getEcsRegionID(config map[string]string) string {
	if value := config[regionConfigKey]; value == "" {
		if value, err := getMetaData(metadataRegionKey); err != nil || value == "" {
			// set default region
			return "cn-hangzhou"
		} else {
			return value
		}
	} else {
		return value
	}
}

// getRamRole return ramrole name
func getRamRole () (string, error) {
	subpath := "ram/security-credentials/"
	roleName, err := GetMetaData(subpath)
	if err != nil {
		return "", err
	}
	return roleName, nil
}

//getSTSAK return AccessKeyID, AccessKeySecret and SecurityToken
func getSTSAK(ramrole string) (string, string, string, error) {
	// AliyunCSVeleroRole
	roleAuth := RoleAuth{}
	ramRoleURL := fmt.Sprintf("ram/security-credentials/%s", ramrole)
	roleInfo, err := GetMetaData(ramRoleURL)
	if err != nil {
		return "", "", "", err
	}

	err = json.Unmarshal([]byte(roleInfo), &roleAuth)
	if err != nil {
		return "", "", "", err
	}
	return roleAuth.AccessKeyID, roleAuth.AccessKeySecret, roleAuth.SecurityToken, nil
}

//GetMetaData get metadata from ecs meta-server
func GetMetaData(resource string) (string, error) {
	resp, err := http.Get(metadataURL + resource)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
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
