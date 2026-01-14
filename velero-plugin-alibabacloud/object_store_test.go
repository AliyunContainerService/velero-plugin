/*
Copyright 2018, 2019 the Velero contributors.
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
	"strings"
	"testing"

	ossv2 "github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockOSSClient is a mock implementation of ossClientInterface for testing
type mockOSSClient struct {
	mock.Mock
}

func (m *mockOSSClient) PutObject(ctx context.Context, request *ossv2.PutObjectRequest, optFns ...func(*ossv2.Options)) (*ossv2.PutObjectResult, error) {
	args := m.Called(ctx, request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ossv2.PutObjectResult), args.Error(1)
}

func (m *mockOSSClient) HeadObject(ctx context.Context, request *ossv2.HeadObjectRequest, optFns ...func(*ossv2.Options)) (*ossv2.HeadObjectResult, error) {
	args := m.Called(ctx, request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ossv2.HeadObjectResult), args.Error(1)
}

func (m *mockOSSClient) GetObject(ctx context.Context, request *ossv2.GetObjectRequest, optFns ...func(*ossv2.Options)) (*ossv2.GetObjectResult, error) {
	args := m.Called(ctx, request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ossv2.GetObjectResult), args.Error(1)
}

func (m *mockOSSClient) ListObjectsV2(ctx context.Context, request *ossv2.ListObjectsV2Request, optFns ...func(*ossv2.Options)) (*ossv2.ListObjectsV2Result, error) {
	args := m.Called(ctx, request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ossv2.ListObjectsV2Result), args.Error(1)
}

func (m *mockOSSClient) DeleteObject(ctx context.Context, request *ossv2.DeleteObjectRequest, optFns ...func(*ossv2.Options)) (*ossv2.DeleteObjectResult, error) {
	args := m.Called(ctx, request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ossv2.DeleteObjectResult), args.Error(1)
}

func (m *mockOSSClient) Presign(ctx context.Context, request any, optFns ...func(*ossv2.PresignOptions)) (*ossv2.PresignResult, error) {
	args := m.Called(ctx, request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ossv2.PresignResult), args.Error(1)
}

func TestNewCredentialsProvider(t *testing.T) {
	tests := []struct {
		name            string
		accessKeyID     string
		accessKeySecret string
		stsToken        string
		expectToken     bool
	}{
		{
			name:            "without STS token",
			accessKeyID:     "test-ak",
			accessKeySecret: "test-sk",
			stsToken:        "",
			expectToken:     false,
		},
		{
			name:            "with STS token",
			accessKeyID:     "test-ak",
			accessKeySecret: "test-sk",
			stsToken:        "test-token",
			expectToken:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := newCredentialsProvider(tc.accessKeyID, tc.accessKeySecret, tc.stsToken)
			assert.NotNil(t, provider)

			// Verify provider can get credentials
			ctx := context.Background()
			creds, err := provider.GetCredentials(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tc.accessKeyID, creds.AccessKeyID)
			assert.Equal(t, tc.accessKeySecret, creds.AccessKeySecret)
			if tc.expectToken {
				assert.Equal(t, tc.stsToken, creds.SecurityToken)
			} else {
				assert.Empty(t, creds.SecurityToken)
			}
		})
	}
}

func TestBuildOssConfig(t *testing.T) {
	// Create a test credentials provider
	credProvider := credentials.NewStaticCredentialsProvider("test-ak", "test-sk")

	tests := []struct {
		name          string
		endpoint      string
		region        string
		expectedError string
		validate      func(t *testing.T, cfg *ossv2.Config)
	}{
		{
			name:     "with endpoint and region",
			endpoint: "https://oss-cn-hangzhou.aliyuncs.com",
			region:   "cn-hangzhou",
			validate: func(t *testing.T, cfg *ossv2.Config) {
				assert.NotNil(t, cfg)
			},
		},
		{
			name:     "with endpoint only",
			endpoint: "https://oss-cn-hangzhou.aliyuncs.com",
			region:   "",
			validate: func(t *testing.T, cfg *ossv2.Config) {
				assert.NotNil(t, cfg)
			},
		},
		{
			name:     "with region only",
			endpoint: "",
			region:   "cn-beijing",
			validate: func(t *testing.T, cfg *ossv2.Config) {
				assert.NotNil(t, cfg)
			},
		},
		{
			name:          "without endpoint and region",
			endpoint:      "",
			region:        "",
			expectedError: "either endpoint or region must be specified",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := buildOssConfig(credProvider, tc.endpoint, tc.region)

			if tc.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
				assert.Nil(t, cfg)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, cfg)
			if tc.validate != nil {
				tc.validate(t, cfg)
			}
		})
	}
}

func TestObjectExists(t *testing.T) {
	tests := []struct {
		name           string
		errorResponse  error
		expectedExists bool
		expectedError  string
	}{
		{
			name:           "exists",
			errorResponse:  nil,
			expectedExists: true,
		},
		{
			name:           "doesn't exist",
			errorResponse:  errors.New("404"),
			expectedExists: false,
		},
		{
			name:           "error checking for existence",
			errorResponse:  errors.New("bad"),
			expectedExists: false,
			expectedError:  "failed to check if object key exists in bucket bucket: bad",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := new(mockOSSClient)
			defer client.AssertExpectations(t)

			o := &ObjectStore{
				client: client,
			}

			// Mock HeadObject call
			if tc.errorResponse == nil {
				client.On("HeadObject", mock.Anything, mock.Anything).Return(&ossv2.HeadObjectResult{}, nil)
			} else if strings.Contains(tc.errorResponse.Error(), "404") {
				client.On("HeadObject", mock.Anything, mock.Anything).Return(nil, tc.errorResponse)
			} else {
				client.On("HeadObject", mock.Anything, mock.Anything).Return(nil, tc.errorResponse)
			}

			exists, err := o.ObjectExists("bucket", "key")

			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
				return
			}
			assert.Equal(t, tc.expectedExists, exists)
		})
	}
}
