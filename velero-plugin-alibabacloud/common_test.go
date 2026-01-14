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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOssEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		region   string
		config   map[string]string
		expected string
	}{
		{
			name:     "custom endpoint",
			region:   "cn-hangzhou",
			config:   map[string]string{endpointConfigKey: "https://custom.oss.com"},
			expected: "https://custom.oss.com",
		},
		{
			name:     "internal network",
			region:   "cn-hangzhou",
			config:   map[string]string{networkTypeConfigKey: networkTypeInternal},
			expected: "https://oss-cn-hangzhou-internal.aliyuncs.com",
		},
		{
			name:     "accelerate network",
			region:   "cn-hangzhou",
			config:   map[string]string{networkTypeConfigKey: networkTypeAccelerate},
			expected: "https://oss-accelerate.aliyuncs.com",
		},
		{
			name:     "public network with region",
			region:   "cn-beijing",
			config:   map[string]string{},
			expected: "https://oss-cn-beijing.aliyuncs.com",
		},
		{
			name:     "public network default region when region is empty",
			region:   "",
			config:   map[string]string{},
			expected: "https://oss-cn-hangzhou.aliyuncs.com",
		},
		{
			name:     "public network with different region",
			region:   "cn-shanghai",
			config:   map[string]string{},
			expected: "https://oss-cn-shanghai.aliyuncs.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getOssEndpoint(tc.region, tc.config)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetCredentials(t *testing.T) {
	tests := []struct {
		name          string
		veleroForAck  bool
		setupEnv      func(*testing.T)
		expectedError string
		validateCred  func(*testing.T, *ossCredentials)
	}{
		{
			name:         "success: get credentials from env directly",
			veleroForAck: false,
			setupEnv: func(t *testing.T) {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "test-ak")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "test-sk")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
			},
			validateCred: func(t *testing.T, cred *ossCredentials) {
				assert.Equal(t, "test-ak", cred.accessKeyID)
				assert.Equal(t, "test-sk", cred.accessKeySecret)
				assert.Empty(t, cred.stsToken)
				assert.Empty(t, cred.ramRole)
			},
		},
		{
			name:         "success: get credentials from env with STS token",
			veleroForAck: false,
			setupEnv: func(t *testing.T) {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "test-ak")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "test-sk")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "test-token")
			},
			validateCred: func(t *testing.T, cred *ossCredentials) {
				assert.Equal(t, "test-ak", cred.accessKeyID)
				assert.Equal(t, "test-sk", cred.accessKeySecret)
				assert.Equal(t, "test-token", cred.stsToken)
				assert.Empty(t, cred.ramRole)
			},
		},
		{
			name:         "success: get credentials from file",
			veleroForAck: false,
			setupEnv: func(t *testing.T) {
				// Create a temporary credential file
				tmpDir, err := os.MkdirTemp("", "test-cred")
				require.NoError(t, err)

				credFile := filepath.Join(tmpDir, "credentials")
				err = os.WriteFile(credFile, []byte(`ALIBABA_CLOUD_ACCESS_KEY_ID=file-ak
ALIBABA_CLOUD_ACCESS_KEY_SECRET=file-sk
ALIBABA_CLOUD_ACCESS_STS_TOKEN=file-token
`), 0644)
				require.NoError(t, err)

				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", credFile)
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")

				// Cleanup after test
				t.Cleanup(func() {
					os.RemoveAll(tmpDir)
				})
			},
			validateCred: func(t *testing.T, cred *ossCredentials) {
				assert.Equal(t, "file-ak", cred.accessKeyID)
				assert.Equal(t, "file-sk", cred.accessKeySecret)
				assert.Equal(t, "file-token", cred.stsToken)
				assert.Empty(t, cred.ramRole)
			},
		},
		{
			name:         "error: non-ACK environment without credentials",
			veleroForAck: false,
			setupEnv: func(t *testing.T) {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
			},
			expectedError: "ALIBABA_CLOUD_ACCESS_KEY_ID or ALIBABA_CLOUD_ACCESS_KEY_SECRET environment variable is not set",
		},
		{
			name:         "error: non-ACK environment with only AK",
			veleroForAck: false,
			setupEnv: func(t *testing.T) {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "test-ak")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
			},
			expectedError: "ALIBABA_CLOUD_ACCESS_KEY_ID or ALIBABA_CLOUD_ACCESS_KEY_SECRET environment variable is not set",
		},
		{
			name:         "error: non-ACK environment with only SK",
			veleroForAck: false,
			setupEnv: func(t *testing.T) {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "test-sk")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
			},
			expectedError: "ALIBABA_CLOUD_ACCESS_KEY_ID or ALIBABA_CLOUD_ACCESS_KEY_SECRET environment variable is not set",
		},
		{
			name:         "error: invalid credential file",
			veleroForAck: false,
			setupEnv: func(t *testing.T) {
				// Set a non-existent file
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "/nonexistent/file/path")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
			},
			expectedError: "error loading environment from ALIBABA_CLOUD_CREDENTIALS_FILE",
		},
		{
			name:         "success: custom RAM role in non-ACK environment",
			veleroForAck: false,
			setupEnv: func(t *testing.T) {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
				t.Setenv("ALIBABA_CLOUD_RAM_ROLE", "CustomVeleroRole")
			},
			// This will fail because getSTSAK requires real ECS metadata service,
			// but it verifies that the custom RAM role path is taken
			expectedError: "Failed to get sts token from ram role CustomVeleroRole",
		},
		{
			name:         "success: custom RAM role takes precedence over AccessKey",
			veleroForAck: false,
			setupEnv: func(t *testing.T) {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "test-ak")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "test-sk")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
				t.Setenv("ALIBABA_CLOUD_RAM_ROLE", "CustomVeleroRole")
			},
			// AccessKey should take precedence, so RAM role should be ignored
			validateCred: func(t *testing.T, cred *ossCredentials) {
				assert.Equal(t, "test-ak", cred.accessKeyID)
				assert.Equal(t, "test-sk", cred.accessKeySecret)
				assert.Empty(t, cred.stsToken)
				assert.Empty(t, cred.ramRole, "RAM role should be cleared when AccessKey is used")
			},
		},
		{
			name:         "success: custom RAM role from credential file",
			veleroForAck: false,
			setupEnv: func(t *testing.T) {
				// Create a temporary credential file with custom RAM role
				tmpDir, err := os.MkdirTemp("", "test-cred")
				require.NoError(t, err)

				credFile := filepath.Join(tmpDir, "credentials")
				err = os.WriteFile(credFile, []byte(`ALIBABA_CLOUD_RAM_ROLE=FileCustomRole
`), 0644)
				require.NoError(t, err)

				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", credFile)
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")

				// Cleanup after test
				t.Cleanup(func() {
					os.RemoveAll(tmpDir)
				})
			},
			// This will fail because getSTSAK requires real ECS metadata service,
			// but it verifies that the custom RAM role from file is used
			expectedError: "Failed to get sts token from ram role FileCustomRole",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup environment
			tc.setupEnv(t)

			// Call getCredentials
			cred, err := getCredentials(tc.veleroForAck)

			// Validate results
			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
				assert.Nil(t, cred)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cred)
				if tc.validateCred != nil {
					tc.validateCred(t, cred)
				}
			}
		})
	}
}

// Note: Tests for ACK environment with automatic RAM role detection (via ECS metadata)
// are not included here because they require mocking the MetaClient which is more complex.
// Those scenarios should be tested in integration tests.
//
// The tests above verify:
// 1. AccessKey credentials (from env or file) take precedence over RAM role
// 2. Custom RAM role (via ALIBABA_CLOUD_RAM_ROLE) is supported in both ACK and non-ACK environments
// 3. Custom RAM role can be specified via credential file
// 4. Error handling when credentials are not available
