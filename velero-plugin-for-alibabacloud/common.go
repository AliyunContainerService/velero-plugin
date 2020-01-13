package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"os"
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
