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
		config        map[string]string
		setupEnv      func(*testing.T) map[string]string
		expectedError string
		validateCred  func(*testing.T, *ossCredentials)
	}{
		{
			name:   "success: get credentials from env directly",
			config: nil,
			setupEnv: func(t *testing.T) map[string]string {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "test-ak")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "test-sk")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
				return nil
			},
			validateCred: func(t *testing.T, cred *ossCredentials) {
				assert.Equal(t, "test-ak", cred.accessKeyID)
				assert.Equal(t, "test-sk", cred.accessKeySecret)
				assert.Empty(t, cred.stsToken)
				assert.Empty(t, cred.ramRole)
			},
		},
		{
			name:   "success: get credentials from env with STS token",
			config: nil,
			setupEnv: func(t *testing.T) map[string]string {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "test-ak")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "test-sk")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "test-token")
				return nil
			},
			validateCred: func(t *testing.T, cred *ossCredentials) {
				assert.Equal(t, "test-ak", cred.accessKeyID)
				assert.Equal(t, "test-sk", cred.accessKeySecret)
				assert.Equal(t, "test-token", cred.stsToken)
				assert.Empty(t, cred.ramRole)
			},
		},
		{
			name:   "success: get credentials from file",
			config: nil,
			setupEnv: func(t *testing.T) map[string]string {
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
				// Cleanup after test
				t.Cleanup(func() {
					os.RemoveAll(tmpDir)
				})
				return nil
			},
			validateCred: func(t *testing.T, cred *ossCredentials) {
				assert.Equal(t, "file-ak", cred.accessKeyID)
				assert.Equal(t, "file-sk", cred.accessKeySecret)
				assert.Equal(t, "file-token", cred.stsToken)
				assert.Empty(t, cred.ramRole)
			},
		},
		{
			name:   "success: get credentials from file via config",
			config: nil, // Will be set in setupEnv
			setupEnv: func(t *testing.T) map[string]string {
				// Create a temporary credential file
				tmpDir, err := os.MkdirTemp("", "test-cred-config")
				require.NoError(t, err)

				credFile := filepath.Join(tmpDir, "credentials")
				err = os.WriteFile(credFile, []byte(`ALIBABA_CLOUD_ACCESS_KEY_ID=config-file-ak
ALIBABA_CLOUD_ACCESS_KEY_SECRET=config-file-sk
ALIBABA_CLOUD_ACCESS_STS_TOKEN=config-file-token
`), 0644)
				require.NoError(t, err)

				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")

				// Cleanup after test
				t.Cleanup(func() {
					os.RemoveAll(tmpDir)
				})

				return map[string]string{"credentialsFile": credFile}
			},
			validateCred: func(t *testing.T, cred *ossCredentials) {
				assert.Equal(t, "config-file-ak", cred.accessKeyID)
				assert.Equal(t, "config-file-sk", cred.accessKeySecret)
				assert.Equal(t, "config-file-token", cred.stsToken)
				assert.Empty(t, cred.ramRole)
			},
		},
		{
			name:   "error: non-ACK environment without credentials",
			config: map[string]string{"notOnECS": "true"},
			setupEnv: func(t *testing.T) map[string]string {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
				return nil
			},
			expectedError: "ALIBABA_CLOUD_ACCESS_KEY_ID or ALIBABA_CLOUD_ACCESS_KEY_SECRET environment variable is not set",
		},
		{
			name:   "error: non-ACK environment with only AK",
			config: map[string]string{"notOnECS": "true"},
			setupEnv: func(t *testing.T) map[string]string {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "test-ak")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
				return nil
			},
			expectedError: "ALIBABA_CLOUD_ACCESS_KEY_ID or ALIBABA_CLOUD_ACCESS_KEY_SECRET environment variable is not set",
		},
		{
			name:   "error: non-ACK environment with only SK",
			config: map[string]string{"notOnECS": "true"},
			setupEnv: func(t *testing.T) map[string]string {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "test-sk")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
				return nil
			},
			expectedError: "ALIBABA_CLOUD_ACCESS_KEY_ID or ALIBABA_CLOUD_ACCESS_KEY_SECRET environment variable is not set",
		},
		{
			name:   "error: invalid credential file",
			config: nil,
			setupEnv: func(t *testing.T) map[string]string {
				// Set a non-existent file
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "/nonexistent/file/path")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
				return nil
			},
			expectedError: "error loading credentials file",
		},
		{
			name:   "error: invalid credential file from config",
			config: map[string]string{"credentialsFile": "/nonexistent/file/path"},
			setupEnv: func(t *testing.T) map[string]string {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
				return nil
			},
			expectedError: "error loading credentials file",
		},
		{
			name:   "success: custom RAM role in non-ACK environment",
			config: map[string]string{"notOnECS": "true"},
			setupEnv: func(t *testing.T) map[string]string {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
				t.Setenv("ALIBABA_CLOUD_RAM_ROLE", "CustomVeleroRole")
				return nil
			},
			// This will fail because getSTSAK requires real ECS metadata service,
			// but it verifies that the custom RAM role path is taken
			expectedError: "Failed to get sts token from ram role CustomVeleroRole",
		},
		{
			name:   "success: custom RAM role takes precedence over AccessKey",
			config: nil,
			setupEnv: func(t *testing.T) map[string]string {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "test-ak")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "test-sk")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
				t.Setenv("ALIBABA_CLOUD_RAM_ROLE", "CustomVeleroRole")
				return nil
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
			name:   "success: custom RAM role from credential file",
			config: nil,
			setupEnv: func(t *testing.T) map[string]string {
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
				return nil
			},
			// This will fail because getSTSAK requires real ECS metadata service,
			// but it verifies that the custom RAM role from file is used
			expectedError: "Failed to get sts token from ram role FileCustomRole",
		},
		// ============================================================
		// Isolation tests: verify per-location credential separation
		// ============================================================
		{
			name:   "isolation: credentialsFile does not pollute process env vars",
			config: nil,
			setupEnv: func(t *testing.T) map[string]string {
				// Create a credential file with specific AK/SK
				tmpDir, err := os.MkdirTemp("", "test-cred-isolation")
				require.NoError(t, err)

				credFile := filepath.Join(tmpDir, "credentials")
				err = os.WriteFile(credFile, []byte(`ALIBABA_CLOUD_ACCESS_KEY_ID=file-isolated-ak
ALIBABA_CLOUD_ACCESS_KEY_SECRET=file-isolated-sk
`), 0644)
				require.NoError(t, err)

				// Set process env vars to different values
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "process-env-ak")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "process-env-sk")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")

				t.Cleanup(func() {
					os.RemoveAll(tmpDir)
				})

				return map[string]string{"credentialsFile": credFile}
			},
			validateCred: func(t *testing.T, cred *ossCredentials) {
				// Should use file credentials, not process env
				assert.Equal(t, "file-isolated-ak", cred.accessKeyID)
				assert.Equal(t, "file-isolated-sk", cred.accessKeySecret)
				// Verify process env vars were NOT overwritten
				assert.Equal(t, "process-env-ak", os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID"),
					"process env vars must not be polluted by credentialsFile loading")
				assert.Equal(t, "process-env-sk", os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET"),
					"process env vars must not be polluted by credentialsFile loading")
			},
		},
		{
			name:   "isolation: no credentialsFile falls back to process env vars",
			config: map[string]string{"notOnECS": "true"},
			setupEnv: func(t *testing.T) map[string]string {
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "fallback-ak")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "fallback-sk")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
				return nil
			},
			validateCred: func(t *testing.T, cred *ossCredentials) {
				assert.Equal(t, "fallback-ak", cred.accessKeyID)
				assert.Equal(t, "fallback-sk", cred.accessKeySecret)
			},
		},
		{
			name:   "isolation: BSL with file-A then VSL with file-B get different credentials",
			config: nil, // will be set per-call below
			setupEnv: func(t *testing.T) map[string]string {
				// This test simulates sequential BSL and VSL Init calls.
				// We call getCredentials twice with different files and verify isolation.
				tmpDir, err := os.MkdirTemp("", "test-bsl-vsl-isolation")
				require.NoError(t, err)

				bslFile := filepath.Join(tmpDir, "bsl-credentials")
				err = os.WriteFile(bslFile, []byte(`ALIBABA_CLOUD_ACCESS_KEY_ID=bsl-ak
ALIBABA_CLOUD_ACCESS_KEY_SECRET=bsl-sk
`), 0644)
				require.NoError(t, err)

				vslFile := filepath.Join(tmpDir, "vsl-credentials")
				err = os.WriteFile(vslFile, []byte(`ALIBABA_CLOUD_ACCESS_KEY_ID=vsl-ak
ALIBABA_CLOUD_ACCESS_KEY_SECRET=vsl-sk
`), 0644)
				require.NoError(t, err)

				// Clear env to ensure no interference
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")

				t.Cleanup(func() { os.RemoveAll(tmpDir) })

				// Call BSL
				bslCred, err := getCredentials(map[string]string{"credentialsFile": bslFile})
				require.NoError(t, err)
				assert.Equal(t, "bsl-ak", bslCred.accessKeyID)
				assert.Equal(t, "bsl-sk", bslCred.accessKeySecret)

				// Call VSL — must get different credentials
				vslCred, err := getCredentials(map[string]string{"credentialsFile": vslFile})
				require.NoError(t, err)
				assert.Equal(t, "vsl-ak", vslCred.accessKeyID)
				assert.Equal(t, "vsl-sk", vslCred.accessKeySecret)

				// Verify env vars not polluted by either call
				assert.Empty(t, os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID"),
					"env must remain empty after file-based credential loading")

				// Return nil config — the actual test case assertion is done above
				// We use validateCred=nil and no expectedError to just pass
				return map[string]string{"notOnECS": "true", "credentialsFile": bslFile}
			},
			validateCred: func(t *testing.T, cred *ossCredentials) {
				// The main assertions are in setupEnv above; this is just the final call
				assert.Equal(t, "bsl-ak", cred.accessKeyID)
			},
		},
		// ============================================================
		// Breaking change tests: spec.credential does NOT leak to other locations
		// ============================================================
		{
			name:   "breaking: BSL credentialsFile does not leak AK to VSL without file (non-ACK)",
			config: nil,
			setupEnv: func(t *testing.T) map[string]string {
				// Simulate: BSL has a credentialsFile with AK/SK
				tmpDir, err := os.MkdirTemp("", "test-no-leak")
				require.NoError(t, err)

				bslFile := filepath.Join(tmpDir, "bsl-credentials")
				err = os.WriteFile(bslFile, []byte(`ALIBABA_CLOUD_ACCESS_KEY_ID=bsl-ak
ALIBABA_CLOUD_ACCESS_KEY_SECRET=bsl-sk
`), 0644)
				require.NoError(t, err)

				// Clear all env vars
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
				t.Setenv("ALIBABA_CLOUD_RAM_ROLE", "")

				t.Cleanup(func() { os.RemoveAll(tmpDir) })

				// BSL Init: loads file credentials successfully
				bslCred, err := getCredentials(map[string]string{"credentialsFile": bslFile})
				require.NoError(t, err)
				assert.Equal(t, "bsl-ak", bslCred.accessKeyID)

				// VSL Init: no credentialsFile, no env vars, non-ACK → must FAIL
				// (previously this would succeed by picking up BSL's leaked env vars)
				return map[string]string{"notOnECS": "true"}
			},
			expectedError: "ALIBABA_CLOUD_ACCESS_KEY_ID or ALIBABA_CLOUD_ACCESS_KEY_SECRET environment variable is not set",
		},
		{
			name:   "breaking: credentialsFile with partial AK only does not combine with env SK",
			config: nil,
			setupEnv: func(t *testing.T) map[string]string {
				// File has only AK, env has SK — these should NOT be combined.
				// The file path takes precedence: credentials come entirely from file.
				tmpDir, err := os.MkdirTemp("", "test-partial-file")
				require.NoError(t, err)

				credFile := filepath.Join(tmpDir, "credentials")
				err = os.WriteFile(credFile, []byte(`ALIBABA_CLOUD_ACCESS_KEY_ID=file-ak-only
`), 0644)
				require.NoError(t, err)

				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "env-sk-should-not-combine")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")
				t.Setenv("ALIBABA_CLOUD_RAM_ROLE", "")

				t.Cleanup(func() { os.RemoveAll(tmpDir) })

				// When credentialsFile is used, ONLY file contents matter.
				// Incomplete credentials (AK without SK) from file should not be
				// supplemented by env vars — the file is authoritative for that location.
				return map[string]string{"credentialsFile": credFile, "notOnECS": "true"}
			},
			// AK from file but no SK → incomplete → falls through to RAM role check → fails (non-ACK)
			expectedError: "ALIBABA_CLOUD_ACCESS_KEY_ID or ALIBABA_CLOUD_ACCESS_KEY_SECRET environment variable is not set",
		},
		// ============================================================
		// Backward compatibility: shared env var path still works
		// ============================================================
		{
			name:   "compat: shared env vars work for both BSL and VSL (no credentialsFile)",
			config: nil,
			setupEnv: func(t *testing.T) map[string]string {
				// Shared credential via env vars — the common case
				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "shared-ak")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "shared-sk")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "shared-token")

				// Simulate BSL and VSL both calling getCredentials with no file
				bslCred, err := getCredentials(map[string]string{"notOnECS": "true"})
				require.NoError(t, err)
				assert.Equal(t, "shared-ak", bslCred.accessKeyID)
				assert.Equal(t, "shared-sk", bslCred.accessKeySecret)
				assert.Equal(t, "shared-token", bslCred.stsToken)

				vslCred, err := getCredentials(map[string]string{"notOnECS": "true"})
				require.NoError(t, err)
				assert.Equal(t, "shared-ak", vslCred.accessKeyID)
				assert.Equal(t, "shared-sk", vslCred.accessKeySecret)
				assert.Equal(t, "shared-token", vslCred.stsToken)

				return map[string]string{"notOnECS": "true"}
			},
			validateCred: func(t *testing.T, cred *ossCredentials) {
				assert.Equal(t, "shared-ak", cred.accessKeyID)
				assert.Equal(t, "shared-sk", cred.accessKeySecret)
				assert.Equal(t, "shared-token", cred.stsToken)
			},
		},
		{
			name:   "compat: legacy ALIBABA_CLOUD_CREDENTIALS_FILE env var still works",
			config: nil,
			setupEnv: func(t *testing.T) map[string]string {
				// Legacy path: file path via env var (no config key)
				tmpDir, err := os.MkdirTemp("", "test-legacy-env-file")
				require.NoError(t, err)

				credFile := filepath.Join(tmpDir, "credentials")
				err = os.WriteFile(credFile, []byte(`ALIBABA_CLOUD_ACCESS_KEY_ID=legacy-file-ak
ALIBABA_CLOUD_ACCESS_KEY_SECRET=legacy-file-sk
`), 0644)
				require.NoError(t, err)

				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", credFile)
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")

				t.Cleanup(func() { os.RemoveAll(tmpDir) })
				return nil
			},
			validateCred: func(t *testing.T, cred *ossCredentials) {
				assert.Equal(t, "legacy-file-ak", cred.accessKeyID)
				assert.Equal(t, "legacy-file-sk", cred.accessKeySecret)
			},
		},
		{
			name:   "compat: config credentialsFile takes precedence over env CREDENTIALS_FILE",
			config: nil,
			setupEnv: func(t *testing.T) map[string]string {
				tmpDir, err := os.MkdirTemp("", "test-config-over-env")
				require.NoError(t, err)

				// File referenced by env var
				envFile := filepath.Join(tmpDir, "env-credentials")
				err = os.WriteFile(envFile, []byte(`ALIBABA_CLOUD_ACCESS_KEY_ID=env-file-ak
ALIBABA_CLOUD_ACCESS_KEY_SECRET=env-file-sk
`), 0644)
				require.NoError(t, err)

				// File referenced by config key (should win)
				configFile := filepath.Join(tmpDir, "config-credentials")
				err = os.WriteFile(configFile, []byte(`ALIBABA_CLOUD_ACCESS_KEY_ID=config-file-ak
ALIBABA_CLOUD_ACCESS_KEY_SECRET=config-file-sk
`), 0644)
				require.NoError(t, err)

				t.Setenv("ALIBABA_CLOUD_CREDENTIALS_FILE", envFile)
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
				t.Setenv("ALIBABA_CLOUD_ACCESS_STS_TOKEN", "")

				t.Cleanup(func() { os.RemoveAll(tmpDir) })
				return map[string]string{"credentialsFile": configFile}
			},
			validateCred: func(t *testing.T, cred *ossCredentials) {
				assert.Equal(t, "config-file-ak", cred.accessKeyID)
				assert.Equal(t, "config-file-sk", cred.accessKeySecret)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup environment and get config
			config := tc.config
			if tc.setupEnv != nil {
				setupConfig := tc.setupEnv(t)
				if setupConfig != nil {
					config = setupConfig
				}
			}

			// Call getCredentials
			cred, err := getCredentials(config)

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

func TestVeleroForAck(t *testing.T) {
	tests := []struct {
		name           string
		config         map[string]string
		veleroForAck   string
		expectedResult bool
	}{
		{
			name:           "config is nil, environment variable not set",
			config:         nil,
			veleroForAck:   "",
			expectedResult: true,
		},
		{
			name:           "config is nil, environment variable set to false",
			config:         nil,
			veleroForAck:   "false",
			expectedResult: false,
		},
		{
			name:           "config is nil, environment variable set to true",
			config:         nil,
			veleroForAck:   "true",
			expectedResult: true,
		},
		{
			name: "config with notOnECS=true (lowercase), should return false regardless of env",
			config: map[string]string{
				"notOnECS": "true",
			},
			veleroForAck:   "true",
			expectedResult: false,
		},
		{
			name: "config with notOnECS=True (mixed case), should return false regardless of env",
			config: map[string]string{
				"notOnECS": "True",
			},
			veleroForAck:   "true",
			expectedResult: false,
		},
		{
			name: "config with notOnECS=TRUE (uppercase), should return false regardless of env",
			config: map[string]string{
				"notOnECS": "TRUE",
			},
			veleroForAck:   "true",
			expectedResult: false,
		},
		{
			name: "config with notOnECS=true, environment variable set to false, should return false",
			config: map[string]string{
				"notOnECS": "true",
			},
			veleroForAck:   "false",
			expectedResult: false,
		},
		{
			name: "config with notOnECS=false, should check environment variable",
			config: map[string]string{
				"notOnECS": "false",
			},
			veleroForAck:   "false",
			expectedResult: false,
		},
		{
			name: "config with notOnECS=false, environment variable not set, should return true",
			config: map[string]string{
				"notOnECS": "false",
			},
			veleroForAck:   "",
			expectedResult: true,
		},
		{
			name: "config with notOnECS=yes, should check environment variable",
			config: map[string]string{
				"notOnECS": "yes",
			},
			veleroForAck:   "false",
			expectedResult: false,
		},
		{
			name: "config without not-on-ecs key, should check environment variable",
			config: map[string]string{
				"other-key": "value",
			},
			veleroForAck:   "false",
			expectedResult: false,
		},
		{
			name:           "config is empty map, should check environment variable",
			config:         map[string]string{},
			veleroForAck:   "false",
			expectedResult: false,
		},
		{
			name:           "config is empty map, environment variable not set, should return true",
			config:         map[string]string{},
			veleroForAck:   "",
			expectedResult: true,
		},
		{
			name:           "config is nil, environment variable set to False (mixed case)",
			config:         nil,
			veleroForAck:   "False",
			expectedResult: false,
		},
		{
			name:           "config is nil, environment variable set to FALSE (uppercase)",
			config:         nil,
			veleroForAck:   "FALSE",
			expectedResult: false,
		},
		{
			name:           "config is nil, environment variable set to True (mixed case)",
			config:         nil,
			veleroForAck:   "True",
			expectedResult: true,
		},
		{
			name:           "config is nil, environment variable set to TRUE (uppercase)",
			config:         nil,
			veleroForAck:   "TRUE",
			expectedResult: true,
		},
		{
			name:           "config is nil, environment variable set to other value",
			config:         nil,
			veleroForAck:   "yes",
			expectedResult: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Save original value
			originalValue := os.Getenv("VELERO_FOR_ACK")
			defer func() {
				// Restore original value
				if originalValue == "" {
					os.Unsetenv("VELERO_FOR_ACK")
				} else {
					os.Setenv("VELERO_FOR_ACK", originalValue)
				}
			}()

			// Set test value
			if tc.veleroForAck == "" {
				os.Unsetenv("VELERO_FOR_ACK")
			} else {
				os.Setenv("VELERO_FOR_ACK", tc.veleroForAck)
			}

			// Call function and verify result
			result := veleroForAck(tc.config)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}
