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

	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
	"io/ioutil"
	"math/rand"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	OriginStr = "volumeId"
	TargetStr = "VolumeId"
	Workspace = "/tmp/velero-restore/"
)

type bucketGetter interface {
	Bucket(bucket string) (ossBucket, error)
}

type ossBucket interface {
	IsObjectExist(key string, options ...oss.Option) (bool, error)
	ListObjects(options ...oss.Option) (oss.ListObjectsResult, error)
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

// ObjectStore represents an object storage entity
type ObjectStore struct {
	log             logrus.FieldLogger
	client          bucketGetter
	encryptionKeyID string
	privateKey      []byte
}

// newObjectStore init ObjectStore
func newObjectStore(logger logrus.FieldLogger) *ObjectStore {
	return &ObjectStore{log: logger}
}

func (o *ObjectStore) getBucket(bucket string) (ossBucket, error) {
	bucketObj, err := o.client.Bucket(bucket)
	if err != nil {
		o.log.Errorf("failed to get OSS bucket: %v", err)
	}
	return bucketObj, err
}

// Init init oss client with os envs
func (o *ObjectStore) Init(config map[string]string) error {
	if err := veleroplugin.ValidateObjectStoreConfigKeys(config, regionConfigKey); err != nil {
		return err
	}

	if err := loadEnv(); err != nil {
		return err
	}

	accessKeyID := os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")
	accessKeySecret := os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")
	stsToken := os.Getenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN")
	encryptionKeyID := os.Getenv("ALIBABA_CLOUD_ENCRYPTION_KEY_ID")

	if len(accessKeyID) == 0 {
		return errors.Errorf("ALIBABA_CLOUD_ACCESS_KEY_ID environment variable is not set")
	}

	if len(accessKeySecret) == 0 {
		return errors.Errorf("ALIBABA_CLOUD_ACCESS_KEY_SECRET environment variable is not set")
	}

	endpoint := getOssEndpoint(config)
	var client *oss.Client
	var err error

	if len(stsToken) == 0 {
		client, err = oss.New(endpoint, accessKeyID, accessKeySecret)
	} else {
		client, err = oss.New(endpoint, accessKeyID, accessKeySecret, oss.SecurityToken(stsToken))
	}

	if err != nil {
		return errors.Errorf("failed to create OSS client: %v", err.Error())
	}

	o.client = &ossBucketGetter{
		client,
	}

	o.encryptionKeyID = encryptionKeyID

	return nil
}

// PutObject put objects to oss bucket
func (o *ObjectStore) PutObject(bucket, key string, body io.Reader) error {
	bucketObj, err := o.getBucket(bucket)
	if err != nil {
		return err
	}
	if o.encryptionKeyID != "" {
		err = bucketObj.PutObject(key, body,
			oss.ServerSideEncryption("KMS"),
			oss.ServerSideEncryptionKeyID(o.encryptionKeyID))
	} else {

		err = bucketObj.PutObject(key, body)
	}

	return err
}

// ObjectExists check if object exist in a oss bucket
func (o *ObjectStore) ObjectExists(bucket, key string) (bool, error) {
	bucketObj, err := o.getBucket(bucket)
	if err != nil {
		return false, err
	}
	return bucketObj.IsObjectExist(key)
}

// GetObject get objects from a bucket
func (o *ObjectStore) GetObject(bucket, key string) (io.ReadCloser, error) {
	bucketObj, err := o.getBucket(bucket)
	if err != nil {
		return nil, err
	}

	if strings.HasSuffix(key, ".tar.gz") {
		body, err := bucketObj.GetObject(key)
		if err != nil {
			return nil, err
		}

		return CheckAndConvertVolumeId(body)
	}

	return bucketObj.GetObject(key)
}

// ListCommonPrefixes interface
func (o *ObjectStore) ListCommonPrefixes(bucket, prefix, delimiter string) ([]string, error) {

	bucketObj, err := o.getBucket(bucket)
	if err != nil {
		return nil, err
	}
	var res []string
	marker := oss.Marker("")
	for {
		lor, err := bucketObj.ListObjects(oss.Prefix(prefix), oss.Delimiter(delimiter), oss.MaxKeys(50), marker)
		if err != nil {
			o.log.Errorf("failed to list objects: %v", err)
			return res, err
		}
		res = append(res, lor.CommonPrefixes...)
		if lor.IsTruncated {
			marker = oss.Marker(lor.NextMarker)
		} else {
			break
		}
	}

	return res, nil
}

// ListObjects list objects of a bucket
func (o *ObjectStore) ListObjects(bucket, prefix string) ([]string, error) {
	bucketObj, err := o.getBucket(bucket)
	if err != nil {
		return nil, err
	}

	var res []string
	marker := oss.Marker("")
	for {
		lor, err := bucketObj.ListObjects(oss.Prefix(prefix), oss.MaxKeys(50), marker)
		if err != nil {
			o.log.Errorf("failed to list objects: %v", err)
		}
		for _, obj := range lor.Objects {
			res = append(res, obj.Key)
		}
		if lor.IsTruncated {
			marker = oss.Marker(lor.NextMarker)
		} else {
			break
		}
	}

	return res, nil
}

// DeleteObject delete objects from oss bucket
func (o *ObjectStore) DeleteObject(bucket, key string) error {
	bucketObj, err := o.getBucket(bucket)
	if err != nil {
		return err
	}
	return bucketObj.DeleteObject(key)
}

// CreateSignedURL create a signed URL
func (o *ObjectStore) CreateSignedURL(bucket, key string, ttl time.Duration) (string, error) {
	bucketObj, err := o.getBucket(bucket)
	if err != nil {
		return "", err
	}

	return bucketObj.SignURL(key, oss.HTTPGet, int64(ttl.Seconds()))

}

// CheckAndConvertVolumeId convert volumeId to VolumeId in persistentvolumes json files
func CheckAndConvertVolumeId(body io.ReadCloser) (io.ReadCloser, error) {
	randStr := CreateCaptcha()
	tmpWorkspace := filepath.Join(Workspace, randStr)
	tmpFileName := fmt.Sprintf("%s.tar.gz", randStr)
	if _, err := CheckPathExistsAndCreate(tmpWorkspace); err != nil {
		return nil, err
	}
	if err := os.Chdir(tmpWorkspace); err != nil {
		return nil, err
	}
	fd, err := os.OpenFile(tmpFileName, os.O_WRONLY|os.O_CREATE, 0660)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	if _, err := io.Copy(fd, body); err != nil {
		return nil, err
	}

	if err := DeCompress(tmpFileName, ""); err != nil {
		return nil, err
	}

	if err := os.Remove(tmpFileName); err != nil {
		return nil, err
	}

	tmpFiles := make([]string, 0)
	err = filepath.Walk(tmpWorkspace,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if f, _ := os.Stat(path); !f.IsDir() {
				if strings.Index(path, "resources/persistentvolumes/cluster") > 0 {
					tmpFiles = append(tmpFiles, path)
				}
			}
			return nil
		})
	if err != nil {
		return nil, err
	}

	for _, f := range tmpFiles {
		fmt.Println(f)
		if err := ReplaceVolumeId(f); err != nil {
			return nil, err
		}
	}

	if err := Compress(".", tmpFileName); err != nil {
		return nil, err
	}

	f1, err := ioutil.ReadFile(tmpFileName)
	if err != nil {
		return nil, err
	}
	f2 := ioutil.NopCloser(bytes.NewReader(f1))

	if err := os.RemoveAll(tmpWorkspace); err != nil {
		return nil, err
	}

	return f2, nil
}

// CheckPathExistsAndCreate
func CheckPathExistsAndCreate(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		err = os.MkdirAll(path, os.ModePerm)
		if err != nil {
			return false, err
		} else {
			return true, nil
		}
	}
	return false, nil
}

// CreateCaptcha
func CreateCaptcha() string {
	return fmt.Sprintf("%08v", rand.New(rand.NewSource(time.Now().UnixNano())).Int31n(1000000))
}

// DeCompress
func DeCompress(tarFile, dest string) error {
	srcFile, err := os.Open(tarFile)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	gr, err := gzip.NewReader(srcFile)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return err
			}
		}
		filename := dest + hdr.Name
		file, err := CreateFile(filename)
		if err != nil {
			return err
		}
		io.Copy(file, tr)
	}
	return nil
}

// CreateFile
func CreateFile(name string) (*os.File, error) {
	err := os.MkdirAll(string([]rune(name)[0:strings.LastIndex(name, "/")]), 0755)
	if err != nil {
		return nil, err
	}
	return os.Create(name)
}

// ReplaceVolumeId
func ReplaceVolumeId(filePath string) error {
	f, err := os.OpenFile(filePath, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	output := make([]byte, 0)
	for {
		line, _, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if ok, _ := regexp.Match(OriginStr, line); ok {
			reg := regexp.MustCompile(OriginStr)
			newByte := reg.ReplaceAll(line, []byte(TargetStr))
			output = append(output, newByte...)
			output = append(output, []byte("\n")...)
		} else {
			output = append(output, line...)
			output = append(output, []byte("\n")...)
		}
	}

	if err := writeToFile(filePath, output); err != nil {
		return err
	}
	return nil
}

// writeToFile
func writeToFile(filePath string, outPut []byte) error {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_TRUNC, 0600)
	defer f.Close()
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(f)
	_, err = writer.Write(outPut)
	if err != nil {
		return err
	}
	writer.Flush()
	return nil
}

// Compress
func Compress(src, dst string) error {
	fw, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer fw.Close()

	gw := gzip.NewWriter(fw)
	defer gw.Close()

	tw := tar.NewWriter(gw)

	defer tw.Close()

	return filepath.Walk(src, func(fileName string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.Index(fileName, dst) > -1 {
			return nil
		}

		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}

		hdr.Name = strings.TrimPrefix(fileName, string(filepath.Separator))

		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		if !fi.Mode().IsRegular() {
			return nil
		}

		fr, err := os.Open(fileName)
		defer fr.Close()
		if err != nil {
			return err
		}

		if _, err := io.Copy(tw, fr); err != nil {
			return err
		}

		return nil
	})
}
