/*
Copyright 2017, 2019 the Velero contributors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"io"
	"os"
	"strings"
	"time"

	ossv2 "github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
)

// ossClientInterface defines the interface for OSS client operations
// This allows for easier testing with mocks
type ossClientInterface interface {
	PutObject(ctx context.Context, request *ossv2.PutObjectRequest, optFns ...func(*ossv2.Options)) (*ossv2.PutObjectResult, error)
	HeadObject(ctx context.Context, request *ossv2.HeadObjectRequest, optFns ...func(*ossv2.Options)) (*ossv2.HeadObjectResult, error)
	GetObject(ctx context.Context, request *ossv2.GetObjectRequest, optFns ...func(*ossv2.Options)) (*ossv2.GetObjectResult, error)
	ListObjectsV2(ctx context.Context, request *ossv2.ListObjectsV2Request, optFns ...func(*ossv2.Options)) (*ossv2.ListObjectsV2Result, error)
	DeleteObject(ctx context.Context, request *ossv2.DeleteObjectRequest, optFns ...func(*ossv2.Options)) (*ossv2.DeleteObjectResult, error)
	Presign(ctx context.Context, request any, optFns ...func(*ossv2.PresignOptions)) (*ossv2.PresignResult, error)
}

// ossClientWrapper wraps ossv2.Client to implement ossClientInterface
type ossClientWrapper struct {
	client *ossv2.Client
}

func (w *ossClientWrapper) PutObject(ctx context.Context, request *ossv2.PutObjectRequest, optFns ...func(*ossv2.Options)) (*ossv2.PutObjectResult, error) {
	return w.client.PutObject(ctx, request, optFns...)
}

func (w *ossClientWrapper) HeadObject(ctx context.Context, request *ossv2.HeadObjectRequest, optFns ...func(*ossv2.Options)) (*ossv2.HeadObjectResult, error) {
	return w.client.HeadObject(ctx, request, optFns...)
}

func (w *ossClientWrapper) GetObject(ctx context.Context, request *ossv2.GetObjectRequest, optFns ...func(*ossv2.Options)) (*ossv2.GetObjectResult, error) {
	return w.client.GetObject(ctx, request, optFns...)
}

func (w *ossClientWrapper) ListObjectsV2(ctx context.Context, request *ossv2.ListObjectsV2Request, optFns ...func(*ossv2.Options)) (*ossv2.ListObjectsV2Result, error) {
	return w.client.ListObjectsV2(ctx, request, optFns...)
}

func (w *ossClientWrapper) DeleteObject(ctx context.Context, request *ossv2.DeleteObjectRequest, optFns ...func(*ossv2.Options)) (*ossv2.DeleteObjectResult, error) {
	return w.client.DeleteObject(ctx, request, optFns...)
}

func (w *ossClientWrapper) Presign(ctx context.Context, request any, optFns ...func(*ossv2.PresignOptions)) (*ossv2.PresignResult, error) {
	return w.client.Presign(ctx, request, optFns...)
}

// ObjectStore represents an object storage entity
type ObjectStore struct {
	log             logrus.FieldLogger
	client          ossClientInterface
	encryptionKeyID string
	privateKey      []byte
	ramRole         string
	endpoint        string
	region          string
	rawClient       *ossv2.Client // Keep raw client for updateOssClient
}

// newObjectStore init ObjectStore
func newObjectStore(logger logrus.FieldLogger) *ObjectStore {
	return &ObjectStore{log: logger}
}

// interfaces refers: https://github.com/vmware-tanzu/velero/blob/v1.17.1/pkg/plugin/velero/object_store.go
// ObjectStore exposes basic object-storage operations required

// Init prepares the ObjectStore for usage using the provided map of
// configuration key-value pairs. It returns an error if the ObjectStore
// cannot be initialized from the provided config.
// Init init oss client with os envs
func (o *ObjectStore) Init(config map[string]string) error {
	if err := veleroplugin.ValidateObjectStoreConfigKeys(config, regionConfigKey, networkTypeConfigKey, endpointConfigKey); err != nil {
		return errors.Wrapf(err, "failed to validate object store config keys")
	}

	region := getEcsRegionID(config)
	if region == "" {
		region = DefaultRegion
	}
	o.region = region

	o.endpoint = getOssEndpoint(region, config)
	o.encryptionKeyID = os.Getenv("ALIBABA_CLOUD_ENCRYPTION_KEY_ID")

	veleroForAck := os.Getenv("VELERO_FOR_ACK") == "true"
	cred, err := getCredentials(veleroForAck)
	if err != nil {
		return errors.Wrapf(err, "failed to get credentials")
	}

	o.ramRole = cred.ramRole
	rawClient, err := o.getOssClient(cred)
	if err != nil {
		return errors.Wrapf(err, "failed to create OSS client")
	}

	o.rawClient = rawClient
	o.client = &ossClientWrapper{client: rawClient}
	return nil
}

// PutObject creates a new object using the data in body within the specified
// object storage bucket with the given key.
func (o *ObjectStore) PutObject(bucket, key string, body io.Reader) error {
	// Update OSS client if needed (for STS token refresh)
	if err := o.updateOssClient(); err != nil {
		return errors.Wrapf(err, "failed to update OSS client for putting object %s", key)
	}

	ctx := context.Background()
	request := &ossv2.PutObjectRequest{
		Bucket: ossv2.Ptr(bucket),
		Key:    ossv2.Ptr(key),
		Body:   body,
	}

	if o.encryptionKeyID != "" {
		request.ServerSideEncryption = ossv2.Ptr("KMS")
		request.ServerSideEncryptionKeyId = ossv2.Ptr(o.encryptionKeyID)
	}

	_, err := o.client.PutObject(ctx, request)
	if err != nil {
		if o.encryptionKeyID != "" {
			return errors.Wrapf(err, "failed to put object %s to bucket %s with encryption", key, bucket)
		}
		return errors.Wrapf(err, "failed to put object %s to bucket %s", key, bucket)
	}
	return nil
}

// ObjectExists checks if there is an object with the given key in the object storage bucket.
func (o *ObjectStore) ObjectExists(bucket, key string) (bool, error) {
	// Update OSS client if needed (for STS token refresh)
	if err := o.updateOssClient(); err != nil {
		return false, errors.Wrapf(err, "failed to update OSS client for checking object %s", key)
	}

	ctx := context.Background()
	request := &ossv2.HeadObjectRequest{
		Bucket: ossv2.Ptr(bucket),
		Key:    ossv2.Ptr(key),
	}

	// Note: V2 SDK handles encryption automatically based on client config
	// If encryption is needed, it should be configured at client level
	_, err := o.client.HeadObject(ctx, request)
	if err != nil {
		// Check if it's a 404 error (object doesn't exist)
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "NoSuchKey") {
			return false, nil
		}
		return false, errors.Wrapf(err, "failed to check if object %s exists in bucket %s", key, bucket)
	}
	return true, nil
}

// GetObject retrieves the object with the given key from the specified
// bucket in object storage.
func (o *ObjectStore) GetObject(bucket, key string) (io.ReadCloser, error) {
	// Update OSS client if needed (for STS token refresh)
	if err := o.updateOssClient(); err != nil {
		return nil, errors.Wrapf(err, "failed to update OSS client for getting object %s", key)
	}

	ctx := context.Background()
	request := &ossv2.GetObjectRequest{
		Bucket: ossv2.Ptr(bucket),
		Key:    ossv2.Ptr(key),
	}

	// Note: V2 SDK handles encryption automatically based on client config
	// If encryption is needed, it should be configured at client level
	result, err := o.client.GetObject(ctx, request)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get object %s from bucket %s", key, bucket)
	}
	return result.Body, nil
}

// ListCommonPrefixes gets a list of all object key prefixes that start with
// the specified prefix and stop at the next instance of the provided delimiter.
//
// For example, if the bucket contains the following keys:
//
//	a-prefix/foo-1/bar
//	a-prefix/foo-1/baz
//	a-prefix/foo-2/baz
//	some-other-prefix/foo-3/bar
//
// and the provided prefix arg is "a-prefix/", and the delimiter is "/",
// this will return the slice {"a-prefix/foo-1/", "a-prefix/foo-2/"}.
func (o *ObjectStore) ListCommonPrefixes(bucket, prefix, delimiter string) ([]string, error) {
	// Update OSS client if needed (for STS token refresh)
	if err := o.updateOssClient(); err != nil {
		return nil, errors.Wrapf(err, "failed to update OSS client for listing common prefixes")
	}

	ctx := context.Background()
	var res []string
	continuationToken := ""
	maxKeys := int32(50)

	for {
		request := &ossv2.ListObjectsV2Request{
			Bucket:    ossv2.Ptr(bucket),
			Prefix:    ossv2.Ptr(prefix),
			Delimiter: ossv2.Ptr(delimiter),
			MaxKeys:   maxKeys,
		}
		if continuationToken != "" {
			request.ContinuationToken = ossv2.Ptr(continuationToken)
		}

		result, err := o.client.ListObjectsV2(ctx, request)
		if err != nil {
			return res, errors.Wrapf(err, "failed to list objects with prefix %s in bucket %s", prefix, bucket)
		}

		if result.CommonPrefixes != nil {
			for _, cp := range result.CommonPrefixes {
				if cp.Prefix != nil {
					res = append(res, *cp.Prefix)
				}
			}
		}

		if result.IsTruncated && result.NextContinuationToken != nil {
			continuationToken = *result.NextContinuationToken
		} else {
			break
		}
	}

	return res, nil
}

// ListObjects gets a list of all keys in the specified bucket
// that have the given prefix.
func (o *ObjectStore) ListObjects(bucket, prefix string) ([]string, error) {
	// Update OSS client if needed (for STS token refresh)
	if err := o.updateOssClient(); err != nil {
		return nil, errors.Wrapf(err, "failed to update OSS client for listing objects")
	}

	ctx := context.Background()
	var res []string
	continuationToken := ""
	maxKeys := int32(50)

	for {
		request := &ossv2.ListObjectsV2Request{
			Bucket:  ossv2.Ptr(bucket),
			Prefix:  ossv2.Ptr(prefix),
			MaxKeys: maxKeys,
		}
		if continuationToken != "" {
			request.ContinuationToken = ossv2.Ptr(continuationToken)
		}

		result, err := o.client.ListObjectsV2(ctx, request)
		if err != nil {
			return res, errors.Wrapf(err, "failed to list objects with prefix %s in bucket %s", prefix, bucket)
		}

		if result.Contents != nil {
			for _, obj := range result.Contents {
				if obj.Key != nil {
					res = append(res, *obj.Key)
				}
			}
		}

		if result.IsTruncated && result.NextContinuationToken != nil {
			continuationToken = *result.NextContinuationToken
		} else {
			break
		}
	}

	return res, nil
}

// DeleteObject removes the object with the specified key from the given
// bucket.
func (o *ObjectStore) DeleteObject(bucket, key string) error {
	// Update OSS client if needed (for STS token refresh)
	if err := o.updateOssClient(); err != nil {
		return errors.Wrapf(err, "failed to update OSS client for deleting object %s", key)
	}

	ctx := context.Background()
	request := &ossv2.DeleteObjectRequest{
		Bucket: ossv2.Ptr(bucket),
		Key:    ossv2.Ptr(key),
	}

	_, err := o.client.DeleteObject(ctx, request)
	if err != nil {
		return errors.Wrapf(err, "failed to delete object %s from bucket %s", key, bucket)
	}
	return nil
}

// CreateSignedURL creates a pre-signed URL for the given bucket and key that expires after ttl.
func (o *ObjectStore) CreateSignedURL(bucket, key string, ttl time.Duration) (string, error) {
	// Update OSS client if needed (for STS token refresh)
	if err := o.updateOssClient(); err != nil {
		return "", errors.Wrapf(err, "failed to update OSS client for creating signed URL")
	}

	ctx := context.Background()
	request := &ossv2.GetObjectRequest{
		Bucket: ossv2.Ptr(bucket),
		Key:    ossv2.Ptr(key),
	}

	result, err := o.client.Presign(ctx, request, ossv2.PresignExpires(ttl))
	if err != nil {
		return "", errors.Wrapf(err, "failed to create signed URL for object %s in bucket %s", key, bucket)
	}
	return result.URL, nil
}

// ============================================================================
// ObjectStore internal utility functions (not part of Velero plugin interface)
// ============================================================================

// newCredentialsProvider creates a credentials provider from access key, secret, and optional STS token
func newCredentialsProvider(accessKeyID, accessKeySecret, stsToken string) credentials.CredentialsProvider {
	if len(stsToken) == 0 {
		return credentials.NewStaticCredentialsProvider(accessKeyID, accessKeySecret)
	}
	return credentials.NewStaticCredentialsProvider(accessKeyID, accessKeySecret, stsToken)
}

// buildOssConfig builds OSS client configuration with credentials, endpoint, and region
// V2 SDK supports both Region+Endpoint or just Endpoint (like V1)
func buildOssConfig(credProvider credentials.CredentialsProvider, endpoint, region string) (*ossv2.Config, error) {
	cfg := ossv2.LoadDefaultConfig().
		WithCredentialsProvider(credProvider)

	// If endpoint is specified, use it directly (like V1 behavior)
	// Otherwise, use region to let SDK construct the endpoint
	if endpoint != "" {
		cfg = cfg.WithEndpoint(endpoint)
		// If endpoint is specified, we still need region for some operations
		if region != "" {
			cfg = cfg.WithRegion(region)
		}
	} else if region != "" {
		cfg = cfg.WithRegion(region)
	} else {
		return nil, errors.New("either endpoint or region must be specified")
	}

	return cfg, nil
}

// getOssClient creates a new OSS client using the provided credentials
// This function only handles OSS client initialization, credentials should be obtained separately
// V2 SDK supports both Region+Endpoint or just Endpoint (like V1)
func (o *ObjectStore) getOssClient(cred *ossCredentials) (*ossv2.Client, error) {
	credProvider := newCredentialsProvider(cred.accessKeyID, cred.accessKeySecret, cred.stsToken)
	cfg, err := buildOssConfig(credProvider, o.endpoint, o.region)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build OSS config")
	}

	client := ossv2.NewClient(cfg)
	return client, nil
}

// updateOssClient updates OSS client with new STS token if RAM role is provided
func (o *ObjectStore) updateOssClient() error {
	if len(o.ramRole) == 0 {
		return nil
	}

	accessKeyID, accessKeySecret, stsToken, err := getSTSAK(o.ramRole)
	if err != nil {
		return errors.Wrapf(err, "failed to get STS token for RAM role %s", o.ramRole)
	}
	cred := &ossCredentials{
		accessKeyID:     accessKeyID,
		accessKeySecret: accessKeySecret,
		stsToken:        stsToken,
	}
	client, err := o.getOssClient(cred)
	if err != nil {
		return errors.Wrapf(err, "failed to update OSS client")
	}

	o.rawClient = client
	o.client = &ossClientWrapper{client: o.rawClient}
	return nil
}
