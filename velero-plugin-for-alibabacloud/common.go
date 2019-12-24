package main

import (
	"github.com/joho/godotenv"
	"os"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"fmt"
)

const (
	metadataURL       = "http://100.100.100.200/latest/meta-data/"
	metadataRegionKey = "region"
	metadataZoneKey   = "zone-id"
	regionConfigKey   = "region"
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

// getOssEndpoint return oss endpoint in format "oss-%s.aliyuncs.com"
func getOssEndpoint(config map[string]string) string {
	if value := config[regionConfigKey]; value == "" {
		if value, err := getMetaData(metadataRegionKey); err != nil {
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
		if value, err := getMetaData(metadataRegionKey); err != nil {
			// set default region
			return "cn-hangzhou"
		} else {
			return value
		}
	} else {
		return value
	}
}