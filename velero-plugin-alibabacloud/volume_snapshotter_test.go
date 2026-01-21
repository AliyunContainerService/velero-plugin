/*
Copyright 2017 the Velero contributors.
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
	"sort"
	"testing"

	ecs20140526 "github.com/alibabacloud-go/ecs-20140526/v4/client"
	"github.com/alibabacloud-go/tea/tea"
	alicloudErr "github.com/aliyun/alibaba-cloud-sdk-go/sdk/errors"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func newTestLogger() logrus.FieldLogger {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	return logger
}

func TestGetVolumeIDFlexVolume(t *testing.T) {
	b := newVolumeSnapshotter(newTestLogger())

	pv := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}

	// missing spec.CSI and spec.FlexVolume -> no error
	volumeID, err := b.GetVolumeID(pv)
	require.NoError(t, err)
	assert.Equal(t, "", volumeID)

	// missing spec.flexvolume.options.volumeID -> error
	options := map[string]interface{}{}

	flexVolume := map[string]interface{}{
		"driver":  "alicloud/disk",
		"options": options,
	}
	pv.Object["spec"] = map[string]interface{}{
		"flexVolume": flexVolume,
	}
	volumeID, err = b.GetVolumeID(pv)
	assert.Error(t, err)
	assert.Equal(t, "", volumeID)

	// regex miss
	options["volumeId"] = "foo"
	volumeID, err = b.GetVolumeID(pv)
	assert.NoError(t, err)
	assert.Equal(t, "foo", volumeID)

	// regex match 1
	options["volumeId"] = "vol-abc123"
	volumeID, err = b.GetVolumeID(pv)
	assert.NoError(t, err)
	assert.Equal(t, "vol-abc123", volumeID)

	// regex match 2
	options["volumeId"] = "vol-abc123"
	volumeID, err = b.GetVolumeID(pv)
	assert.NoError(t, err)
	assert.Equal(t, "vol-abc123", volumeID)
}

func TestSetVolumeIDFlexVolume(t *testing.T) {
	b := &VolumeSnapshotter{}

	pv := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}

	// missing spec.FlexVolume -> no error
	_, err := b.SetVolumeID(pv, "vol-updated")
	require.Error(t, err)

	// happy path
	flexVolume := map[string]interface{}{
		"driver": "alicloud/disk",
	}

	pv.Object["spec"] = map[string]interface{}{
		"flexVolume": flexVolume,
	}

	labels := map[string]interface{}{
		"failure-domain.beta.kubernetes.io/zone": "cn-hangzhou-c",
	}

	pv.Object["metadata"] = map[string]interface{}{
		"labels": labels,
	}

	updatedPV, err := b.SetVolumeID(pv, "vol-updated")

	require.NoError(t, err)

	res := new(v1.PersistentVolume)
	require.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(updatedPV.UnstructuredContent(), res))
	require.NotNil(t, res.Spec.FlexVolume)
	diskID, err := getEBSDiskID(res)
	require.NoError(t, err)
	assert.Equal(t, "vol-updated", diskID)
}

func TestGetVolumeID(t *testing.T) {
	b := newVolumeSnapshotter(newTestLogger())

	pv := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}

	// missing spec.CSI and spec.FlexVolume -> no error
	volumeID, err := b.GetVolumeID(pv)
	require.NoError(t, err)
	assert.Equal(t, "", volumeID)

	// missing spec.csi.volumeAttributes.volumeID -> error
	csi := map[string]interface{}{
		"driver": "diskplugin.csi.alibabacloud.com",
	}
	pv.Object["spec"] = map[string]interface{}{
		"csi": csi,
	}
	volumeID, err = b.GetVolumeID(pv)
	assert.Error(t, err)
	assert.Equal(t, "", volumeID)

	// regex miss
	csi["volumeHandle"] = "foo"
	volumeID, err = b.GetVolumeID(pv)
	assert.NoError(t, err)
	assert.Equal(t, "foo", volumeID)

	// regex match 1
	csi["volumeHandle"] = "vol-abc123"
	volumeID, err = b.GetVolumeID(pv)
	assert.NoError(t, err)
	assert.Equal(t, "vol-abc123", volumeID)

	// regex match 2
	csi["volumeHandle"] = "vol-abc123"
	volumeID, err = b.GetVolumeID(pv)
	assert.NoError(t, err)
	assert.Equal(t, "vol-abc123", volumeID)
}

func TestSetVolumeID(t *testing.T) {
	b := &VolumeSnapshotter{}

	pv := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}

	// missing spec.CSI -> error
	_, err := b.SetVolumeID(pv, "vol-updated")
	require.Error(t, err)

	// happy path
	csi := map[string]interface{}{
		"driver": "diskplugin.csi.alibabacloud.com",
	}
	pv.Object["spec"] = map[string]interface{}{
		"csi": csi,
	}

	labels := map[string]interface{}{
		"failure-domain.beta.kubernetes.io/zone": "cn-hangzhou-c",
	}

	pv.Object["metadata"] = map[string]interface{}{
		"labels": labels,
	}

	updatedPV, err := b.SetVolumeID(pv, "vol-updated")

	require.NoError(t, err)

	res := new(v1.PersistentVolume)
	require.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(updatedPV.UnstructuredContent(), res))
	require.NotNil(t, res.Spec.CSI)
	diskID, err := getEBSDiskID(res)
	require.NoError(t, err)
	assert.Equal(t, "vol-updated", diskID)
}

func TestSetVolumeIDNoZone(t *testing.T) {
	b := &VolumeSnapshotter{}

	pv := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}

	// missing spec.CSI -> error
	_, err := b.SetVolumeID(pv, "vol-updated")
	require.Error(t, err)

	// happy path
	csi := map[string]interface{}{
		"driver": "diskplugin.csi.alibabacloud.com",
	}
	pv.Object["spec"] = map[string]interface{}{
		"csi": csi,
	}

	updatedPV, err := b.SetVolumeID(pv, "vol-updated")

	require.NoError(t, err)

	res := new(v1.PersistentVolume)
	require.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(updatedPV.UnstructuredContent(), res))
	require.NotNil(t, res.Spec.CSI)
	diskID, err := getEBSDiskID(res)
	require.NoError(t, err)
	assert.Equal(t, "vol-updated", diskID)
}

func TestGetTagsForCluster(t *testing.T) {
	tests := []struct {
		name         string
		isNameSet    bool
		snapshotTags []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag
		expected     []*ecs20140526.CreateDiskRequestTag
	}{
		{
			name:         "degenerate case (no tags)",
			isNameSet:    false,
			snapshotTags: nil,
			expected:     nil,
		},
		{
			name:      "cluster tags exist and remain set",
			isNameSet: false,
			snapshotTags: []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag{
				{TagKey: tea.String("KubernetesCluster"), TagValue: tea.String("old-cluster")},
				{TagKey: tea.String("kubernetes.io/cluster/old-cluster"), TagValue: tea.String("owned")},
				{TagKey: tea.String("alibaba-cloud-key"), TagValue: tea.String("alibaba-cloud-val")},
			},
			expected: []*ecs20140526.CreateDiskRequestTag{
				{Key: tea.String("KubernetesCluster"), Value: tea.String("old-cluster")},
				{Key: tea.String("kubernetes.io/cluster/old-cluster"), Value: tea.String("owned")},
				{Key: tea.String("alibaba-cloud-key"), Value: tea.String("alibaba-cloud-val")},
			},
		},
		{
			name:         "cluster tags only get applied",
			isNameSet:    true,
			snapshotTags: nil,
			expected: []*ecs20140526.CreateDiskRequestTag{
				{Key: tea.String("KubernetesCluster"), Value: tea.String("current-cluster")},
				{Key: tea.String("kubernetes.io/cluster/current-cluster"), Value: tea.String("owned")},
			},
		},
		{
			name:      "non-overlapping cluster and snapshot tags both get applied",
			isNameSet: true,
			snapshotTags: []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag{
				{TagKey: tea.String("alibaba-cloud-key"), TagValue: tea.String("alibaba-cloud-val")},
			},
			expected: []*ecs20140526.CreateDiskRequestTag{
				{Key: tea.String("KubernetesCluster"), Value: tea.String("current-cluster")},
				{Key: tea.String("kubernetes.io/cluster/current-cluster"), Value: tea.String("owned")},
				{Key: tea.String("alibaba-cloud-key"), Value: tea.String("alibaba-cloud-val")},
			},
		},
		{
			name:      "overlapping cluster tags, current cluster tags take precedence",
			isNameSet: true,
			snapshotTags: []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag{
				{TagKey: tea.String("KubernetesCluster"), TagValue: tea.String("old-name")},
				{TagKey: tea.String("kubernetes.io/cluster/old-name"), TagValue: tea.String("owned")},
				{TagKey: tea.String("alibaba-cloud-key"), TagValue: tea.String("alibaba-cloud-val")},
			},
			expected: []*ecs20140526.CreateDiskRequestTag{
				{Key: tea.String("KubernetesCluster"), Value: tea.String("current-cluster")},
				{Key: tea.String("kubernetes.io/cluster/current-cluster"), Value: tea.String("owned")},
				{Key: tea.String("alibaba-cloud-key"), Value: tea.String("alibaba-cloud-val")},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := newVolumeSnapshotter(newTestLogger())
			if test.isNameSet {
				t.Setenv(ackClusterNameKey, "current-cluster")
			}
			res := b.getTagsForCluster(test.snapshotTags)

			sort.Slice(res, func(i, j int) bool {
				return tea.StringValue(res[i].Key) < tea.StringValue(res[j].Key)
			})

			sort.Slice(test.expected, func(i, j int) bool {
				return tea.StringValue(test.expected[i].Key) < tea.StringValue(test.expected[j].Key)
			})

			assert.Equal(t, len(test.expected), len(res))
			for i := range test.expected {
				assert.Equal(t, tea.StringValue(test.expected[i].Key), tea.StringValue(res[i].Key))
				assert.Equal(t, tea.StringValue(test.expected[i].Value), tea.StringValue(res[i].Value))
			}
		})
	}
}

func TestGetPerformanceLevelFromIOPS(t *testing.T) {
	tests := []struct {
		name     string
		iops     int64
		expected string
	}{
		{
			name:     "IOPS less than 10k - PL0",
			iops:     5000,
			expected: "PL0",
		},
		{
			name:     "IOPS exactly 10k - PL0",
			iops:     10000,
			expected: "PL0",
		},
		{
			name:     "IOPS between 10k and 50k - PL1",
			iops:     30000,
			expected: "PL1",
		},
		{
			name:     "IOPS exactly 50k - PL1",
			iops:     50000,
			expected: "PL1",
		},
		{
			name:     "IOPS between 50k and 100k - PL2",
			iops:     80000,
			expected: "PL2",
		},
		{
			name:     "IOPS exactly 100k - PL2",
			iops:     100000,
			expected: "PL2",
		},
		{
			name:     "IOPS between 100k and 1M - PL3",
			iops:     500000,
			expected: "PL3",
		},
		{
			name:     "IOPS exactly 1M - PL3",
			iops:     1000000,
			expected: "PL3",
		},
		{
			name:     "IOPS greater than 1M - PL3",
			iops:     2000000,
			expected: "PL3",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := getPerformanceLevelFromIOPS(test.iops)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestGetTags(t *testing.T) {
	tests := []struct {
		name       string
		veleroTags map[string]string
		volumeTags []*ecs20140526.DescribeDisksResponseBodyDisksDiskTagsTag
		expected   []*ecs20140526.CreateSnapshotRequestTag
	}{
		{
			name:       "degenerate case (no tags)",
			veleroTags: nil,
			volumeTags: nil,
			expected:   nil,
		},
		{
			name: "velero tags only get applied",
			veleroTags: map[string]string{
				"velero-key1": "velero-val1",
				"velero-key2": "velero-val2",
			},
			volumeTags: nil,
			expected: []*ecs20140526.CreateSnapshotRequestTag{
				{Key: tea.String("velero-key1"), Value: tea.String("velero-val1")},
				{Key: tea.String("velero-key2"), Value: tea.String("velero-val2")},
			},
		},
		{
			name:       "volume tags only get applied",
			veleroTags: nil,
			volumeTags: []*ecs20140526.DescribeDisksResponseBodyDisksDiskTagsTag{
				{TagKey: tea.String("alibaba-cloud-key1"), TagValue: tea.String("alibaba-cloud-val1")},
				{TagKey: tea.String("alibaba-cloud-key2"), TagValue: tea.String("alibaba-cloud-val2")},
			},
			expected: []*ecs20140526.CreateSnapshotRequestTag{
				{Key: tea.String("alibaba-cloud-key1"), Value: tea.String("alibaba-cloud-val1")},
				{Key: tea.String("alibaba-cloud-key2"), Value: tea.String("alibaba-cloud-val2")},
			},
		},
		{
			name:       "non-overlapping velero and volume tags both get applied",
			veleroTags: map[string]string{"velero-key": "velero-val"},
			volumeTags: []*ecs20140526.DescribeDisksResponseBodyDisksDiskTagsTag{
				{TagKey: tea.String("alibaba-cloud-key"), TagValue: tea.String("alibaba-cloud-val")},
			},
			expected: []*ecs20140526.CreateSnapshotRequestTag{
				{Key: tea.String("velero-key"), Value: tea.String("velero-val")},
				{Key: tea.String("alibaba-cloud-key"), Value: tea.String("alibaba-cloud-val")},
			},
		},
		{
			name: "when tags overlap, velero tags take precedence",
			veleroTags: map[string]string{
				"velero-key":      "velero-val",
				"overlapping-key": "velero-val",
			},
			volumeTags: []*ecs20140526.DescribeDisksResponseBodyDisksDiskTagsTag{
				{TagKey: tea.String("alibaba-cloud-key"), TagValue: tea.String("alibaba-cloud-val")},
				{TagKey: tea.String("overlapping-key"), TagValue: tea.String("alibaba-cloud-val")},
			},
			expected: []*ecs20140526.CreateSnapshotRequestTag{
				{Key: tea.String("velero-key"), Value: tea.String("velero-val")},
				{Key: tea.String("overlapping-key"), Value: tea.String("velero-val")},
				{Key: tea.String("alibaba-cloud-key"), Value: tea.String("alibaba-cloud-val")},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := newVolumeSnapshotter(newTestLogger())
			res := b.getTags(test.veleroTags, test.volumeTags)

			sort.Slice(res, func(i, j int) bool {
				return tea.StringValue(res[i].Key) < tea.StringValue(res[j].Key)
			})

			sort.Slice(test.expected, func(i, j int) bool {
				return tea.StringValue(test.expected[i].Key) < tea.StringValue(test.expected[j].Key)
			})

			assert.Equal(t, len(test.expected), len(res))
			for i := range test.expected {
				assert.Equal(t, tea.StringValue(test.expected[i].Key), tea.StringValue(res[i].Key))
				assert.Equal(t, tea.StringValue(test.expected[i].Value), tea.StringValue(res[i].Value))
			}
		})
	}
}

// mockECSClient is a mock implementation of ecsClientInterface for testing
type mockECSClient struct {
	mock.Mock
}

func (m *mockECSClient) CreateDisk(request *ecs20140526.CreateDiskRequest) (*ecs20140526.CreateDiskResponse, error) {
	args := m.Called(request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ecs20140526.CreateDiskResponse), args.Error(1)
}

func (m *mockECSClient) CreateSnapshot(request *ecs20140526.CreateSnapshotRequest) (*ecs20140526.CreateSnapshotResponse, error) {
	args := m.Called(request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ecs20140526.CreateSnapshotResponse), args.Error(1)
}

func (m *mockECSClient) DeleteSnapshot(request *ecs20140526.DeleteSnapshotRequest) (*ecs20140526.DeleteSnapshotResponse, error) {
	args := m.Called(request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ecs20140526.DeleteSnapshotResponse), args.Error(1)
}

func (m *mockECSClient) DescribeSnapshots(request *ecs20140526.DescribeSnapshotsRequest) (*ecs20140526.DescribeSnapshotsResponse, error) {
	args := m.Called(request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ecs20140526.DescribeSnapshotsResponse), args.Error(1)
}

func (m *mockECSClient) DescribeDisks(request *ecs20140526.DescribeDisksRequest) (*ecs20140526.DescribeDisksResponse, error) {
	args := m.Called(request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ecs20140526.DescribeDisksResponse), args.Error(1)
}

func TestCreateSnapshot(t *testing.T) {
	tests := []struct {
		name          string
		volumeID      string
		volumeAZ      string
		tags          map[string]string
		mockSetup     func(*mockECSClient)
		expectedID    string
		expectedError string
	}{
		{
			name:     "success - create snapshot with tags",
			volumeID: "d-123456",
			volumeAZ: "cn-hangzhou-h",
			tags: map[string]string{
				"velero-backup": "backup-123",
			},
			mockSetup: func(m *mockECSClient) {
				diskResponse := &ecs20140526.DescribeDisksResponse{
					Body: &ecs20140526.DescribeDisksResponseBody{
						Disks: &ecs20140526.DescribeDisksResponseBodyDisks{
							Disk: []*ecs20140526.DescribeDisksResponseBodyDisksDisk{
								{
									DiskId: tea.String("d-123456"),
									Tags: &ecs20140526.DescribeDisksResponseBodyDisksDiskTags{
										Tag: []*ecs20140526.DescribeDisksResponseBodyDisksDiskTagsTag{
											{TagKey: tea.String("existing-tag"), TagValue: tea.String("existing-value")},
										},
									},
								},
							},
						},
					},
				}
				m.On("DescribeDisks", mock.Anything).Return(diskResponse, nil)

				snapshotResponse := &ecs20140526.CreateSnapshotResponse{
					Body: &ecs20140526.CreateSnapshotResponseBody{
						SnapshotId: tea.String("s-123456"),
					},
				}
				m.On("CreateSnapshot", mock.Anything).Return(snapshotResponse, nil)
			},
			expectedID: "s-123456",
		},
		{
			name:     "error - describe disk fails",
			volumeID: "d-123456",
			volumeAZ: "cn-hangzhou-h",
			tags:     map[string]string{},
			mockSetup: func(m *mockECSClient) {
				m.On("DescribeDisks", mock.Anything).Return(nil, errors.New("describe disk failed"))
			},
			expectedError: "failed to describe volume d-123456 for creating snapshot",
		},
		{
			name:     "error - create snapshot fails",
			volumeID: "d-123456",
			volumeAZ: "cn-hangzhou-h",
			tags:     map[string]string{},
			mockSetup: func(m *mockECSClient) {
				diskResponse := &ecs20140526.DescribeDisksResponse{
					Body: &ecs20140526.DescribeDisksResponseBody{
						Disks: &ecs20140526.DescribeDisksResponseBodyDisks{
							Disk: []*ecs20140526.DescribeDisksResponseBodyDisksDisk{
								{
									DiskId: tea.String("d-123456"),
									Tags: &ecs20140526.DescribeDisksResponseBodyDisksDiskTags{
										Tag: []*ecs20140526.DescribeDisksResponseBodyDisksDiskTagsTag{},
									},
								},
							},
						},
					},
				}
				m.On("DescribeDisks", mock.Anything).Return(diskResponse, nil)
				m.On("CreateSnapshot", mock.Anything).Return(nil, errors.New("create snapshot failed"))
			},
			expectedError: "failed to create snapshot for volume d-123456",
		},
		{
			name:     "error - missing snapshot ID in response",
			volumeID: "d-123456",
			volumeAZ: "cn-hangzhou-h",
			tags:     map[string]string{},
			mockSetup: func(m *mockECSClient) {
				diskResponse := &ecs20140526.DescribeDisksResponse{
					Body: &ecs20140526.DescribeDisksResponseBody{
						Disks: &ecs20140526.DescribeDisksResponseBodyDisks{
							Disk: []*ecs20140526.DescribeDisksResponseBodyDisksDisk{
								{
									DiskId: tea.String("d-123456"),
									Tags: &ecs20140526.DescribeDisksResponseBodyDisksDiskTags{
										Tag: []*ecs20140526.DescribeDisksResponseBodyDisksDiskTagsTag{},
									},
								},
							},
						},
					},
				}
				m.On("DescribeDisks", mock.Anything).Return(diskResponse, nil)

				snapshotResponse := &ecs20140526.CreateSnapshotResponse{
					Body: &ecs20140526.CreateSnapshotResponseBody{
						SnapshotId: nil,
					},
				}
				m.On("CreateSnapshot", mock.Anything).Return(snapshotResponse, nil)
			},
			expectedError: "create snapshot response missing snapshot ID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := new(mockECSClient)
			defer client.AssertExpectations(t)

			test.mockSetup(client)

			b := &VolumeSnapshotter{
				log:    newTestLogger(),
				client: client,
				region: "cn-hangzhou",
			}

			snapshotID, err := b.CreateSnapshot(test.volumeID, test.volumeAZ, test.tags)

			if test.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
				assert.Empty(t, snapshotID)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, test.expectedID, snapshotID)
		})
	}
}

func TestDeleteSnapshot(t *testing.T) {
	client := new(mockECSClient)
	defer client.AssertExpectations(t)

	response := &ecs20140526.DeleteSnapshotResponse{}
	client.On("DeleteSnapshot", mock.Anything).Return(response, nil)

	b := &VolumeSnapshotter{
		log:    newTestLogger(),
		client: client,
		region: "cn-hangzhou",
	}

	err := b.DeleteSnapshot("s-123456")
	assert.NoError(t, err)
}

func TestDeleteSnapshot_NotFound(t *testing.T) {
	client := new(mockECSClient)
	defer client.AssertExpectations(t)

	serverErr := alicloudErr.NewServerError(404, `{"Code":"InvalidSnapshotId.NotFound","Message":"The specified snapshot does not exist."}`, "")
	client.On("DeleteSnapshot", mock.Anything).Return(nil, serverErr)

	b := &VolumeSnapshotter{
		log:    newTestLogger(),
		client: client,
		region: "cn-hangzhou",
	}

	err := b.DeleteSnapshot("s-123456")
	assert.NoError(t, err)
}

func TestDeleteSnapshot_Error(t *testing.T) {
	client := new(mockECSClient)
	defer client.AssertExpectations(t)

	client.On("DeleteSnapshot", mock.Anything).Return(nil, errors.New("delete failed"))

	b := &VolumeSnapshotter{
		log:    newTestLogger(),
		client: client,
		region: "cn-hangzhou",
	}

	err := b.DeleteSnapshot("s-123456")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete snapshot s-123456")
}

func TestGetVolumeInfo(t *testing.T) {
	tests := []struct {
		name          string
		volumeID      string
		volumeAZ      string
		mockSetup     func(*mockECSClient)
		expectedType  string
		expectedIOPS  *int64
		expectedError string
	}{
		{
			name:     "success - get volume info with IOPS",
			volumeID: "d-123456",
			volumeAZ: "cn-hangzhou-h",
			mockSetup: func(m *mockECSClient) {
				iops := int32(3000)
				response := &ecs20140526.DescribeDisksResponse{
					Body: &ecs20140526.DescribeDisksResponseBody{
						Disks: &ecs20140526.DescribeDisksResponseBodyDisks{
							Disk: []*ecs20140526.DescribeDisksResponseBodyDisksDisk{
								{
									DiskId:   tea.String("d-123456"),
									Category: tea.String("cloud_essd"),
									IOPS:     &iops,
								},
							},
						},
					},
				}
				m.On("DescribeDisks", mock.Anything).Return(response, nil)
			},
			expectedType: "cloud_essd",
			expectedIOPS: func() *int64 { i := int64(3000); return &i }(),
		},
		{
			name:     "success - get volume info without IOPS",
			volumeID: "d-123456",
			volumeAZ: "cn-hangzhou-h",
			mockSetup: func(m *mockECSClient) {
				response := &ecs20140526.DescribeDisksResponse{
					Body: &ecs20140526.DescribeDisksResponseBody{
						Disks: &ecs20140526.DescribeDisksResponseBodyDisks{
							Disk: []*ecs20140526.DescribeDisksResponseBodyDisksDisk{
								{
									DiskId:   tea.String("d-123456"),
									Category: tea.String("cloud_ssd"),
									IOPS:     nil,
								},
							},
						},
					},
				}
				m.On("DescribeDisks", mock.Anything).Return(response, nil)
			},
			expectedType: "cloud_ssd",
			expectedIOPS: nil,
		},
		{
			name:     "error - describe disk fails",
			volumeID: "d-123456",
			volumeAZ: "cn-hangzhou-h",
			mockSetup: func(m *mockECSClient) {
				m.On("DescribeDisks", mock.Anything).Return(nil, errors.New("describe failed"))
			},
			expectedError: "failed to describe volume d-123456",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := new(mockECSClient)
			defer client.AssertExpectations(t)

			test.mockSetup(client)

			b := &VolumeSnapshotter{
				log:    newTestLogger(),
				client: client,
				region: "cn-hangzhou",
			}

			volumeType, iops, err := b.GetVolumeInfo(test.volumeID, test.volumeAZ)

			if test.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, test.expectedType, volumeType)
			if test.expectedIOPS == nil {
				assert.Nil(t, iops)
			} else {
				assert.NotNil(t, iops)
				assert.Equal(t, *test.expectedIOPS, *iops)
			}
		})
	}
}

func TestDescribeSnapshot(t *testing.T) {
	tests := []struct {
		name          string
		snapshotID    string
		mockSetup     func(*mockECSClient)
		expectedError string
	}{
		{
			name:       "success - describe snapshot",
			snapshotID: "s-123456",
			mockSetup: func(m *mockECSClient) {
				response := &ecs20140526.DescribeSnapshotsResponse{
					Body: &ecs20140526.DescribeSnapshotsResponseBody{
						Snapshots: &ecs20140526.DescribeSnapshotsResponseBodySnapshots{
							Snapshot: []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshot{
								{
									SnapshotId: tea.String("s-123456"),
									Tags: &ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTags{
										Tag: []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag{
											{TagKey: tea.String("test-key"), TagValue: tea.String("test-value")},
										},
									},
								},
							},
						},
					},
				}
				m.On("DescribeSnapshots", mock.Anything).Return(response, nil)
			},
		},
		{
			name:       "error - describe snapshot fails",
			snapshotID: "s-123456",
			mockSetup: func(m *mockECSClient) {
				m.On("DescribeSnapshots", mock.Anything).Return(nil, errors.New("describe failed"))
			},
			expectedError: "failed to describe snapshot s-123456",
		},
		{
			name:       "error - invalid response (nil body)",
			snapshotID: "s-123456",
			mockSetup: func(m *mockECSClient) {
				response := &ecs20140526.DescribeSnapshotsResponse{
					Body: nil,
				}
				m.On("DescribeSnapshots", mock.Anything).Return(response, nil)
			},
			expectedError: "invalid response from DescribeSnapshots",
		},
		{
			name:       "error - wrong number of snapshots",
			snapshotID: "s-123456",
			mockSetup: func(m *mockECSClient) {
				response := &ecs20140526.DescribeSnapshotsResponse{
					Body: &ecs20140526.DescribeSnapshotsResponseBody{
						Snapshots: &ecs20140526.DescribeSnapshotsResponseBodySnapshots{
							Snapshot: []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshot{
								{SnapshotId: tea.String("s-123456")},
								{SnapshotId: tea.String("s-789012")},
							},
						},
					},
				}
				m.On("DescribeSnapshots", mock.Anything).Return(response, nil)
			},
			expectedError: "expected 1 snapshot from DescribeSnapshots",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := new(mockECSClient)
			defer client.AssertExpectations(t)

			test.mockSetup(client)

			b := &VolumeSnapshotter{
				log:    newTestLogger(),
				client: client,
				region: "cn-hangzhou",
			}

			snapshot, err := b.describeSnapshot(test.snapshotID)

			if test.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
				assert.Nil(t, snapshot)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, snapshot)
			assert.Equal(t, "s-123456", tea.StringValue(snapshot.SnapshotId))
		})
	}
}

func TestDescribeVolume(t *testing.T) {
	tests := []struct {
		name          string
		volumeID      string
		volumeAZ      string
		mockSetup     func(*mockECSClient)
		expectedError string
	}{
		{
			name:     "success - describe volume with zone",
			volumeID: "d-123456",
			volumeAZ: "cn-hangzhou-h",
			mockSetup: func(m *mockECSClient) {
				response := &ecs20140526.DescribeDisksResponse{
					Body: &ecs20140526.DescribeDisksResponseBody{
						Disks: &ecs20140526.DescribeDisksResponseBodyDisks{
							Disk: []*ecs20140526.DescribeDisksResponseBodyDisksDisk{
								{
									DiskId:   tea.String("d-123456"),
									Category: tea.String("cloud_ssd"),
								},
							},
						},
					},
				}
				m.On("DescribeDisks", mock.Anything).Return(response, nil)
			},
		},
		{
			name:     "success - describe volume without zone",
			volumeID: "d-123456",
			volumeAZ: "",
			mockSetup: func(m *mockECSClient) {
				response := &ecs20140526.DescribeDisksResponse{
					Body: &ecs20140526.DescribeDisksResponseBody{
						Disks: &ecs20140526.DescribeDisksResponseBodyDisks{
							Disk: []*ecs20140526.DescribeDisksResponseBodyDisksDisk{
								{
									DiskId:   tea.String("d-123456"),
									Category: tea.String("cloud_ssd"),
								},
							},
						},
					},
				}
				m.On("DescribeDisks", mock.Anything).Return(response, nil)
			},
		},
		{
			name:     "error - describe disk fails",
			volumeID: "d-123456",
			volumeAZ: "cn-hangzhou-h",
			mockSetup: func(m *mockECSClient) {
				m.On("DescribeDisks", mock.Anything).Return(nil, errors.New("describe failed"))
			},
			expectedError: "failed to describe disk d-123456",
		},
		{
			name:     "error - invalid response (nil body)",
			volumeID: "d-123456",
			volumeAZ: "cn-hangzhou-h",
			mockSetup: func(m *mockECSClient) {
				response := &ecs20140526.DescribeDisksResponse{
					Body: nil,
				}
				m.On("DescribeDisks", mock.Anything).Return(response, nil)
			},
			expectedError: "invalid response from DescribeDisks",
		},
		{
			name:     "error - wrong number of disks",
			volumeID: "d-123456",
			volumeAZ: "cn-hangzhou-h",
			mockSetup: func(m *mockECSClient) {
				response := &ecs20140526.DescribeDisksResponse{
					Body: &ecs20140526.DescribeDisksResponseBody{
						Disks: &ecs20140526.DescribeDisksResponseBodyDisks{
							Disk: []*ecs20140526.DescribeDisksResponseBodyDisksDisk{
								{DiskId: tea.String("d-123456")},
								{DiskId: tea.String("d-789012")},
							},
						},
					},
				}
				m.On("DescribeDisks", mock.Anything).Return(response, nil)
			},
			expectedError: "expected 1 disk from DescribeDisks",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := new(mockECSClient)
			defer client.AssertExpectations(t)

			test.mockSetup(client)

			b := &VolumeSnapshotter{
				log:    newTestLogger(),
				client: client,
				region: "cn-hangzhou",
			}

			disk, err := b.describeVolume(test.volumeID, test.volumeAZ)

			if test.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
				assert.Nil(t, disk)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, disk)
			assert.Equal(t, "d-123456", tea.StringValue(disk.DiskId))
		})
	}
}

// TestGetTagsWithVolumeZone tests the getTagsWithVolumeZone function
func TestGetTagsWithVolumeZone(t *testing.T) {
	tests := []struct {
		name          string
		veleroTags    map[string]string
		volumeTags    []*ecs20140526.DescribeDisksResponseBodyDisksDiskTagsTag
		volumeZoneID  string
		expectedCount int
		hasOriginalAZ bool
		originalAZVal string
	}{
		{
			name:          "add original AZ tag when not present",
			veleroTags:    map[string]string{"velero.io/backup": "backup-1"},
			volumeTags:    []*ecs20140526.DescribeDisksResponseBodyDisksDiskTagsTag{},
			volumeZoneID:  "cn-hangzhou-k",
			expectedCount: 2, // velero tag + original AZ tag
			hasOriginalAZ: true,
			originalAZVal: "cn-hangzhou-k",
		},
		{
			name:         "do not add original AZ tag when already present in volume tags",
			veleroTags:   map[string]string{"velero.io/backup": "backup-1"},
			volumeZoneID: "cn-hangzhou-k",
			volumeTags: []*ecs20140526.DescribeDisksResponseBodyDisksDiskTagsTag{
				{TagKey: tea.String(originalVolumeAZTagKey), TagValue: tea.String("cn-hangzhou-l")},
			},
			expectedCount: 2, // velero tag + existing original AZ tag
			hasOriginalAZ: true,
			originalAZVal: "cn-hangzhou-l", // Should keep existing value
		},
		{
			name: "do not add original AZ tag when already present in velero tags",
			veleroTags: map[string]string{
				"velero.io/backup":     "backup-1",
				originalVolumeAZTagKey: "cn-hangzhou-l",
			},
			volumeTags:    []*ecs20140526.DescribeDisksResponseBodyDisksDiskTagsTag{},
			volumeZoneID:  "cn-hangzhou-k",
			expectedCount: 2, // velero tags (including original AZ)
			hasOriginalAZ: true,
			originalAZVal: "cn-hangzhou-l", // Should keep velero tag value
		},
		{
			name:          "do not add original AZ tag when volumeZoneID is empty",
			veleroTags:    map[string]string{"velero.io/backup": "backup-1"},
			volumeTags:    []*ecs20140526.DescribeDisksResponseBodyDisksDiskTagsTag{},
			volumeZoneID:  "",
			expectedCount: 1, // only velero tag
			hasOriginalAZ: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := &VolumeSnapshotter{
				log: newTestLogger(),
			}

			result := b.getTagsWithVolumeZone(test.veleroTags, test.volumeTags, test.volumeZoneID)

			assert.Equal(t, test.expectedCount, len(result))

			// Check if original AZ tag exists
			found := false
			for _, tag := range result {
				if tag != nil && tea.StringValue(tag.Key) == originalVolumeAZTagKey {
					found = true
					if test.hasOriginalAZ {
						assert.Equal(t, test.originalAZVal, tea.StringValue(tag.Value))
					}
					break
				}
			}
			assert.Equal(t, test.hasOriginalAZ, found)
		})
	}
}

// For testing loadSupportedZones, we'll test the parsing logic separately
// and use a simpler approach that doesn't require full kubernetes.Interface implementation

// TestLoadSupportedZones tests the loadSupportedZones function
// Note: This test is simplified to test the parsing logic
// Full integration with kubernetes.Interface would require a more complex mock setup
// TestLoadSupportedZones tests the loadSupportedZones function
// It only tests Kubernetes client related errors, parsing logic is tested separately in TestParseClusterConfig
func TestLoadSupportedZones(t *testing.T) {
	t.Run("kubeClient is nil", func(t *testing.T) {
		b := &VolumeSnapshotter{
			log:            newTestLogger(),
			kubeClient:     nil,
			supportedZones: make(map[string]bool),
		}

		err := b.loadSupportedZones()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Kubernetes client not available")
	})
}

// TestParseClusterConfig tests the parseClusterConfig function
func TestParseClusterConfig(t *testing.T) {
	tests := []struct {
		name          string
		data          map[string]string
		expectedZones map[string]bool
		expectedError string
	}{
		{
			name: "successfully parse supported zones",
			data: map[string]string{
				"vsw-zone": "vsw-2zegxoti9moxn5797q0wc:cn-beijing-l,vsw-2zelzo4uk3hip3o7nzkua:cn-beijing-k,vsw-2zegiyiy0jeh7imiquqlc:cn-beijing-j",
			},
			expectedZones: map[string]bool{
				"cn-beijing-l": true,
				"cn-beijing-k": true,
				"cn-beijing-j": true,
			},
		},
		{
			name: "handle empty pairs",
			data: map[string]string{
				"vsw-zone": "vsw-xxx:cn-hangzhou-k,,vsw-yyy:cn-hangzhou-l",
			},
			expectedZones: map[string]bool{
				"cn-hangzhou-k": true,
				"cn-hangzhou-l": true,
			},
		},
		{
			name: "handle spaces in zone names",
			data: map[string]string{
				"vsw-zone": "vsw-xxx: cn-hangzhou-k ,vsw-yyy:cn-hangzhou-l",
			},
			expectedZones: map[string]bool{
				"cn-hangzhou-k": true,
				"cn-hangzhou-l": true,
			},
		},
		{
			name: "handle empty zone value",
			data: map[string]string{
				"vsw-zone": "vsw-xxx:cn-hangzhou-k,vsw-yyy:",
			},
			expectedZones: map[string]bool{
				"cn-hangzhou-k": true,
			},
		},
		{
			name: "handle pair without colon",
			data: map[string]string{
				"vsw-zone": "vsw-xxx:cn-hangzhou-k,invalid-pair",
			},
			expectedZones: map[string]bool{
				"cn-hangzhou-k": true,
			},
		},
		{
			name:          "data is nil",
			data:          nil,
			expectedError: "data is nil",
		},
		{
			name:          "vsw-zone field not found",
			data:          map[string]string{},
			expectedError: "vsw-zone field not found",
		},
		{
			name: "handle empty vsw-zone value",
			data: map[string]string{
				"vsw-zone": "",
			},
			expectedZones: map[string]bool{},
		},
		{
			name: "handle whitespace-only pairs",
			data: map[string]string{
				"vsw-zone": "  ,  ,vsw-xxx:cn-hangzhou-k,  ",
			},
			expectedZones: map[string]bool{
				"cn-hangzhou-k": true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			zones, err := parseClusterConfig(test.data)

			if test.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
				assert.Nil(t, zones)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedZones, zones)
			}
		})
	}
}

// TestDetermineVolumeAZ tests the determineVolumeAZ function
func TestDetermineVolumeAZ(t *testing.T) {
	tests := []struct {
		name           string
		currentZone    string
		snapshotTags   []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag
		supportedZones map[string]bool
		expectedZone   string
	}{
		{
			name:        "originalZone matches currentZone",
			currentZone: "cn-hangzhou-k",
			snapshotTags: []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag{
				{TagKey: tea.String(originalVolumeAZTagKey), TagValue: tea.String("cn-hangzhou-k")},
			},
			supportedZones: map[string]bool{},
			expectedZone:   "cn-hangzhou-k",
		},
		{
			name:        "originalZone in supported zones",
			currentZone: "cn-beijing-k",
			snapshotTags: []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag{
				{TagKey: tea.String(originalVolumeAZTagKey), TagValue: tea.String("cn-beijing-l")},
			},
			supportedZones: map[string]bool{
				"cn-beijing-l": true,
				"cn-beijing-k": true,
			},
			expectedZone: "cn-beijing-l",
		},
		{
			name:        "originalZone not in supported zones",
			currentZone: "cn-beijing-k",
			snapshotTags: []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag{
				{TagKey: tea.String(originalVolumeAZTagKey), TagValue: tea.String("cn-beijing-m")},
			},
			supportedZones: map[string]bool{
				"cn-beijing-l": true,
				"cn-beijing-k": true,
			},
			expectedZone: "cn-beijing-k", // Should fall back to currentZone
		},
		{
			name:         "no originalZone in snapshot tags",
			currentZone:  "cn-beijing-k",
			snapshotTags: []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag{},
			supportedZones: map[string]bool{
				"cn-beijing-l": true,
			},
			expectedZone: "cn-beijing-k", // Should use currentZone
		},
		{
			name:        "originalZone exists but empty currentZone",
			currentZone: "",
			snapshotTags: []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag{
				{TagKey: tea.String(originalVolumeAZTagKey), TagValue: tea.String("cn-beijing-l")},
			},
			supportedZones: map[string]bool{
				"cn-beijing-l": true,
			},
			expectedZone: "cn-beijing-l", // Should use originalZone if in supported zones
		},
		{
			name:        "originalZone exists but empty currentZone and not in supported zones",
			currentZone: "",
			snapshotTags: []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag{
				{TagKey: tea.String(originalVolumeAZTagKey), TagValue: tea.String("cn-beijing-m")},
			},
			supportedZones: map[string]bool{
				"cn-beijing-l": true,
			},
			expectedZone: "", // Should use empty currentZone
		},
		{
			name:        "nil tag in snapshot tags",
			currentZone: "cn-hangzhou-k",
			snapshotTags: []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag{
				nil,
				{TagKey: tea.String(originalVolumeAZTagKey), TagValue: tea.String("cn-hangzhou-l")},
			},
			supportedZones: map[string]bool{
				"cn-hangzhou-l": true,
			},
			expectedZone: "cn-hangzhou-l",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := &VolumeSnapshotter{
				log:            newTestLogger(),
				zone:           test.currentZone,
				supportedZones: test.supportedZones,
			}

			zone, err := b.determineVolumeAZ(test.snapshotTags)

			assert.NoError(t, err)
			assert.Equal(t, test.expectedZone, zone)
		})
	}
}
