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
	"io"
	"os"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
)

// ObjectStore represents an object storage entity
type ObjectStore struct {
	log             logrus.FieldLogger
	client          bucketGetter
	encryptionKeyID string
	privateKey      []byte
	ramRole         string
	endpoint        string
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

	o.endpoint = getOssEndpoint(config)
	o.encryptionKeyID = os.Getenv("ALIBABA_CLOUD_ENCRYPTION_KEY_ID")

	veleroForAck := os.Getenv("VELERO_FOR_ACK") == "true"
	cred, err := getCredentials(veleroForAck)
	if err != nil {
		return errors.Wrapf(err, "failed to get credentials")
	}

	o.ramRole = cred.ramRole
	client, err := o.getOssClient(cred)
	if err != nil {
		return errors.Wrapf(err, "failed to create OSS client")
	}

	o.client = &ossBucketGetter{
		client,
	}
	return nil
}

// PutObject creates a new object using the data in body within the specified
// object storage bucket with the given key.
func (o *ObjectStore) PutObject(bucket, key string, body io.Reader) error {
	bucketObj, err := o.getBucket(bucket)
	if err != nil {
		return errors.Wrapf(err, "failed to get bucket %s for putting object %s", bucket, key)
	}

	if o.encryptionKeyID == "" {
		if err := bucketObj.PutObject(key, body); err != nil {
			return errors.Wrapf(err, "failed to put object %s to bucket %s", key, bucket)
		}
		return nil
	}

	if err := bucketObj.PutObject(key, body,
		oss.ServerSideEncryption("KMS"),
		oss.ServerSideEncryptionKeyID(o.encryptionKeyID)); err != nil {
		return errors.Wrapf(err, "failed to put object %s to bucket %s with encryption", key, bucket)
	}
	return nil
}

// ObjectExists checks if there is an object with the given key in the object storage bucket.
func (o *ObjectStore) ObjectExists(bucket, key string) (bool, error) {
	bucketObj, err := o.getBucket(bucket)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get bucket %s for checking object %s", bucket, key)
	}

	var exists bool
	if o.encryptionKeyID == "" {
		var err error
		exists, err = bucketObj.IsObjectExist(key)
		if err != nil {
			return false, errors.Wrapf(err, "failed to check if object %s exists in bucket %s", key, bucket)
		}
		return exists, nil
	}

	exists, err = bucketObj.IsObjectExist(key,
		oss.ServerSideEncryption("KMS"),
		oss.ServerSideEncryptionKeyID(o.encryptionKeyID))
	if err != nil {
		return false, errors.Wrapf(err, "failed to check if object %s exists in bucket %s with encryption", key, bucket)
	}
	return exists, nil
}

// GetObject retrieves the object with the given key from the specified
// bucket in object storage.
func (o *ObjectStore) GetObject(bucket, key string) (io.ReadCloser, error) {
	bucketObj, err := o.getBucket(bucket)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket %s for getting object %s", bucket, key)
	}

	var body io.ReadCloser
	if o.encryptionKeyID == "" {
		body, err = bucketObj.GetObject(key)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get object %s from bucket %s", key, bucket)
		}
		return body, nil
	}

	body, err = bucketObj.GetObject(key,
		oss.ServerSideEncryption("KMS"),
		oss.ServerSideEncryptionKeyID(o.encryptionKeyID))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get object %s from bucket %s with encryption", key, bucket)
	}
	return body, nil
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
	bucketObj, err := o.getBucket(bucket)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket %s", bucket)
	}
	var res []string
	continueToken := ""
	for {
		lor, err := bucketObj.ListObjectsV2(oss.Prefix(prefix), oss.Delimiter(delimiter), oss.MaxKeys(50), oss.ContinuationToken(continueToken))
		if err != nil {
			return res, errors.Wrapf(err, "failed to list objects with prefix %s in bucket %s", prefix, bucket)
		}
		res = append(res, lor.CommonPrefixes...)
		if lor.IsTruncated {
			continueToken = lor.NextContinuationToken
		} else {
			break
		}
	}

	return res, nil
}

// ListObjects gets a list of all keys in the specified bucket
// that have the given prefix.
func (o *ObjectStore) ListObjects(bucket, prefix string) ([]string, error) {
	bucketObj, err := o.getBucket(bucket)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket %s", bucket)
	}

	var res []string
	continueToken := ""
	for {
		lor, err := bucketObj.ListObjectsV2(oss.Prefix(prefix), oss.MaxKeys(50), oss.ContinuationToken(continueToken))
		if err != nil {
			return res, errors.Wrapf(err, "failed to list objects with prefix %s in bucket %s", prefix, bucket)
		}
		for _, obj := range lor.Objects {
			res = append(res, obj.Key)
		}
		if lor.IsTruncated {
			continueToken = lor.NextContinuationToken
		} else {
			break
		}
	}

	return res, nil
}

// DeleteObject removes the object with the specified key from the given
// bucket.
func (o *ObjectStore) DeleteObject(bucket, key string) error {
	bucketObj, err := o.getBucket(bucket)
	if err != nil {
		return errors.Wrapf(err, "failed to get bucket %s", bucket)
	}
	if err := bucketObj.DeleteObject(key); err != nil {
		return errors.Wrapf(err, "failed to delete object %s from bucket %s", key, bucket)
	}
	return nil
}

// CreateSignedURL creates a pre-signed URL for the given bucket and key that expires after ttl.
func (o *ObjectStore) CreateSignedURL(bucket, key string, ttl time.Duration) (string, error) {
	bucketObj, err := o.getBucket(bucket)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get bucket %s for creating signed URL for object %s", bucket, key)
	}

	url, err := bucketObj.SignURL(key, oss.HTTPGet, int64(ttl.Seconds()))
	if err != nil {
		return "", errors.Wrapf(err, "failed to create signed URL for object %s in bucket %s", key, bucket)
	}
	return url, nil
}

// ============================================================================
// Internal interfaces and types (not part of Velero plugin interface)
// ============================================================================

type bucketGetter interface {
	Bucket(bucket string) (ossBucket, error)
}

type ossBucket interface {
	IsObjectExist(key string, options ...oss.Option) (bool, error)
	ListObjectsV2(options ...oss.Option) (oss.ListObjectsResultV2, error)
	PutObject(objectKey string, reader io.Reader, options ...oss.Option) error
	GetObject(key string, options ...oss.Option) (io.ReadCloser, error)
	DeleteObject(key string, options ...oss.Option) error
	SignURL(objectKey string, method oss.HTTPMethod, expiredInSec int64, options ...oss.Option) (string, error)
}

type ossBucketGetter struct {
	client *oss.Client
}

func (getter *ossBucketGetter) Bucket(bucket string) (ossBucket, error) {
	return getter.client.Bucket(bucket)
}

// ============================================================================
// ObjectStore internal utility functions (not part of Velero plugin interface)
// ============================================================================

// getBucket gets a bucket object, updating OSS client if needed
func (o *ObjectStore) getBucket(bucket string) (ossBucket, error) {
	var err error
	// TODO: Review the updateOssClient logic - consider caching and refresh strategy for STS tokens
	// AWS plugin uses credential chain and automatic refresh, we may need similar logic
	o.client, err = updateOssClient(o.ramRole, o.endpoint, o.client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to update OSS Client")
	}
	bucketObj, err := o.client.Bucket(bucket)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get OSS bucket %s", bucket)
	}
	return bucketObj, nil
}

// getOssClient creates a new OSS client using the provided credentials
// This function only handles OSS client initialization, credentials should be obtained separately
func (o *ObjectStore) getOssClient(cred *credentials) (*oss.Client, error) {
	var client *oss.Client
	var err error
	if len(cred.stsToken) == 0 {
		client, err = oss.New(o.endpoint, cred.accessKeyID, cred.accessKeySecret)
	} else {
		client, err = oss.New(o.endpoint, cred.accessKeyID, cred.accessKeySecret, oss.SecurityToken(cred.stsToken))
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create OSS client with endpoint %s", o.endpoint)
	}
	return client, nil
}
