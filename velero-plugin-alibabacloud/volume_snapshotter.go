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
	"fmt"
	"os"
	"strings"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecs20140526 "github.com/alibabacloud-go/ecs-20140526/v4/client"
	"github.com/alibabacloud-go/tea/tea"
	alicloudErr "github.com/aliyun/alibaba-cloud-sdk-go/sdk/errors"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
)

const (
	ackClusterNameKey      = "ACK_CLUSTER_NAME"
	originalVolumeAZTagKey = "alibabacloud.velero-plugin/orginal-volume-az"
)

// DiskPerformanceLevels maps performance levels to their max IOPS values
// refers to: https://www.alibabacloud.com/help/en/ecs/developer-reference/api-ecs-2014-05-26-createdisk
var DiskPerformanceLevels = map[string]int64{
	"PL0": 10000,   // Up to 10,000 random read/write IOPS
	"PL1": 50000,   // Up to 50,000 random read/write IOPS
	"PL2": 100000,  // Up to 100,000 random read/write IOPS
	"PL3": 1000000, // Up to 1,000,000 random read/write IOPS
}

// ecsClientInterface defines the interface for ECS client operations
// This allows for easier testing with mocks
type ecsClientInterface interface {
	CreateDisk(request *ecs20140526.CreateDiskRequest) (*ecs20140526.CreateDiskResponse, error)
	CreateSnapshot(request *ecs20140526.CreateSnapshotRequest) (*ecs20140526.CreateSnapshotResponse, error)
	DeleteSnapshot(request *ecs20140526.DeleteSnapshotRequest) (*ecs20140526.DeleteSnapshotResponse, error)
	DescribeSnapshots(request *ecs20140526.DescribeSnapshotsRequest) (*ecs20140526.DescribeSnapshotsResponse, error)
	DescribeDisks(request *ecs20140526.DescribeDisksRequest) (*ecs20140526.DescribeDisksResponse, error)
}

// ecsClientWrapper wraps ecs20140526.Client to implement ecsClientInterface
type ecsClientWrapper struct {
	client *ecs20140526.Client
}

func (w *ecsClientWrapper) CreateDisk(request *ecs20140526.CreateDiskRequest) (*ecs20140526.CreateDiskResponse, error) {
	return w.client.CreateDisk(request)
}

func (w *ecsClientWrapper) CreateSnapshot(request *ecs20140526.CreateSnapshotRequest) (*ecs20140526.CreateSnapshotResponse, error) {
	return w.client.CreateSnapshot(request)
}

func (w *ecsClientWrapper) DeleteSnapshot(request *ecs20140526.DeleteSnapshotRequest) (*ecs20140526.DeleteSnapshotResponse, error) {
	return w.client.DeleteSnapshot(request)
}

func (w *ecsClientWrapper) DescribeSnapshots(request *ecs20140526.DescribeSnapshotsRequest) (*ecs20140526.DescribeSnapshotsResponse, error) {
	return w.client.DescribeSnapshots(request)
}

func (w *ecsClientWrapper) DescribeDisks(request *ecs20140526.DescribeDisksRequest) (*ecs20140526.DescribeDisksResponse, error) {
	return w.client.DescribeDisks(request)
}

// VolumeSnapshotter struct
type VolumeSnapshotter struct {
	log            logrus.FieldLogger
	client         ecsClientInterface
	region         string
	zone           string
	ramRole        string
	rawClient      *ecs20140526.Client  // Keep raw client for updateEcsClient
	kubeClient     kubernetes.Interface // Kubernetes client for ConfigMap queries (optional)
	supportedZones map[string]bool      // Set of supported zones from ack-cluster-profile ConfigMap
}

// newVolumeSnapshotter init a VolumeSnapshotter
func newVolumeSnapshotter(logger logrus.FieldLogger) *VolumeSnapshotter {
	return &VolumeSnapshotter{log: logger}
}

// interfaces refers: https://github.com/vmware-tanzu/velero/blob/v1.17.1/pkg/plugin/velero/volumesnapshotter/v1/volume_snapshotter.go
// VolumeSnapshotter exposes volume snapshot operations required

// Init prepares the VolumeSnapshotter for usage using the provided map of
// configuration key-value pairs. It returns an error if the VolumeSnapshotter
// cannot be initialized from the provided config.
func (b *VolumeSnapshotter) Init(config map[string]string) error {
	if err := veleroplugin.ValidateVolumeSnapshotterConfigKeys(config, regionConfigKey); err != nil {
		return errors.Wrapf(err, "failed to validate volume snapshotter config keys")
	}

	regionID := getEcsRegionID(config)
	b.region = regionID

	zoneID := getEcsZoneID(config)
	b.zone = zoneID

	veleroForAck := veleroForAck(config)
	cred, err := getCredentials(veleroForAck)
	if err != nil {
		return errors.Wrapf(err, "failed to get credentials")
	}

	b.ramRole = cred.ramRole
	rawClient, err := b.getEcsClient(cred)
	if err != nil {
		return errors.Wrapf(err, "failed to create ECS client")
	}

	b.rawClient = rawClient
	b.client = &ecsClientWrapper{client: rawClient}
	b.supportedZones = make(map[string]bool)

	// Try to initialize Kubernetes client and load supported zones from ConfigMap (best-effort)
	// This is used to determine which zones are available in the cluster
	if kubeClient, err := b.initKubeClient(); err != nil {
		b.log.Warnf("failed to initialize Kubernetes client (this is optional): %v", err)
	} else {
		b.kubeClient = kubeClient
		// Try to load supported zones from ack-cluster-profile ConfigMap
		if veleroForAck {
			if err := b.loadSupportedZones(); err != nil {
				b.log.Warnf("failed to load supported zones from ConfigMap (this is optional): %v", err)
			}
		}
	}

	return nil
}

// CreateVolumeFromSnapshot creates a new volume in the specified
// availability zone, initialized from the provided snapshot,
// and with the specified type and IOPS (if using provisioned IOPS).
func (b *VolumeSnapshotter) CreateVolumeFromSnapshot(snapshotID, volumeType, volumeAZ string, iops *int64) (volumeID string, err error) {
	// Update ECS client if needed (for STS token refresh)
	if err := b.updateEcsClient(); err != nil {
		return "", errors.Wrapf(err, "failed to update ECS client for creating volume from snapshot %s", snapshotID)
	}

	// Describe the snapshot so we can apply its tags to the volume
	snapInfo, err := b.describeSnapshot(snapshotID)
	if err != nil {
		return "", errors.Wrapf(err, "failed to describe snapshot %s", snapshotID)
	}

	tags := b.getTagsForCluster(snapInfo.Tags.Tag)

	// Use volumeAZ from parameter if provided, otherwise determine from snapshot tags or metadata
	if volumeAZ == "" {
		var err error
		volumeAZ, err = b.determineVolumeAZ(snapInfo.Tags.Tag)
		if err != nil {
			return "", err
		}
	}

	// Create disk from snapshot with tags
	// Do not validate  disk category and performance level, return error from ECS API d irectly
	req := &ecs20140526.CreateDiskRequest{
		RegionId:     tea.String(b.region),
		SnapshotId:   tea.String(snapshotID),
		ZoneId:       tea.String(volumeAZ),
		DiskCategory: tea.String(volumeType),
	}
	if snapInfo.Encrypted != nil {
		req.Encrypted = snapInfo.Encrypted
	}
	if iops != nil {
		// Convert IOPS to PerformanceLevel for Alibaba Cloud ESSD disks
		performanceLevel := getPerformanceLevelFromIOPS(*iops)
		req.PerformanceLevel = tea.String(performanceLevel)

		// Log the conversion for debugging
		maxIOPS := DiskPerformanceLevels[performanceLevel]
		b.log.Warnf("Converting IOPS: %d to Performance Level: %s, Max supported random read/write IOPS: %d. Note: Only ESSD cloud disks support setting Performance Level.",
			*iops, performanceLevel, maxIOPS)
	}
	if len(tags) > 0 {
		req.Tag = tags
	}

	res, err := b.client.CreateDisk(req)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create disk from snapshot %s", snapshotID)
	}

	if res.Body == nil || res.Body.DiskId == nil {
		return "", errors.New("create disk response missing disk ID")
	}

	return tea.StringValue(res.Body.DiskId), nil
}

// GetVolumeID returns the cloud provider specific identifier for the PersistentVolume.
func (b *VolumeSnapshotter) GetVolumeID(unstructuredPV runtime.Unstructured) (string, error) {
	pv := new(v1.PersistentVolume)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPV.UnstructuredContent(), pv); err != nil {
		return "", errors.Wrapf(err, "failed to convert unstructured to PersistentVolume")
	}

	volumeID, err := getEBSDiskID(pv)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get disk ID from PersistentVolume")
	}

	return volumeID, nil
}

// SetVolumeID sets the cloud provider specific identifier for the PersistentVolume.
func (b *VolumeSnapshotter) SetVolumeID(unstructuredPV runtime.Unstructured, volumeID string) (runtime.Unstructured, error) {
	pv := new(v1.PersistentVolume)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPV.UnstructuredContent(), pv); err != nil {
		return nil, errors.Wrapf(err, "failed to convert unstructured to PersistentVolume")
	}

	err := setEBSDiskID(pv, volumeID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set disk ID %s in PersistentVolume", volumeID)
	}

	res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pv)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert PersistentVolume to unstructured")
	}

	return &unstructured.Unstructured{Object: res}, nil
}

// GetVolumeInfo returns the type and IOPS (if using provisioned IOPS) for
// the specified volume in the given availability zone.
func (b *VolumeSnapshotter) GetVolumeInfo(volumeID, volumeAZ string) (string, *int64, error) {
	// Update ECS client if needed (for STS token refresh)
	if err := b.updateEcsClient(); err != nil {
		return "", nil, errors.Wrapf(err, "failed to update ECS client for getting info of volume %s", volumeID)
	}

	volumeInfo, err := b.describeVolume(volumeID, volumeAZ)
	if err != nil {
		return "", nil, errors.Wrapf(err, "failed to describe volume %s", volumeID)
	}

	var iops *int64
	if volumeInfo.IOPS != nil {
		iopsVal := int64(*volumeInfo.IOPS)
		iops = &iopsVal
	}
	return tea.StringValue(volumeInfo.Category), iops, nil
}

// CreateSnapshot creates a snapshot of the specified volume, and applies the provided
// set of tags to the snapshot.
func (b *VolumeSnapshotter) CreateSnapshot(volumeID, volumeAZ string, tags map[string]string) (snapshotID string, err error) {
	// Update ECS client if needed (for STS token refresh)
	if err := b.updateEcsClient(); err != nil {
		return "", errors.Wrapf(err, "failed to update ECS client for creating snapshot of volume %s", volumeID)
	}

	// Describe the volume so we can copy its tags to the snapshot
	volumeInfo, err := b.describeVolume(volumeID, volumeAZ)
	if err != nil {
		return "", errors.Wrapf(err, "failed to describe volume %s for creating snapshot", volumeID)
	}

	req := &ecs20140526.CreateSnapshotRequest{
		DiskId: tea.String(volumeID),
	}

	// Get volume zone ID for tagging
	volumeZoneID := ""
	if volumeInfo.ZoneId != nil {
		volumeZoneID = tea.StringValue(volumeInfo.ZoneId)
	}

	newTags := b.getTagsWithVolumeZone(tags, volumeInfo.Tags.Tag, volumeZoneID)
	if len(newTags) > 0 {
		req.Tag = newTags
	}

	res, err := b.client.CreateSnapshot(req)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create snapshot for volume %s", volumeID)
	}

	if res.Body == nil || res.Body.SnapshotId == nil {
		return "", errors.New("create snapshot response missing snapshot ID")
	}

	return tea.StringValue(res.Body.SnapshotId), nil
}

// DeleteSnapshot deletes the specified volume snapshot.
func (b *VolumeSnapshotter) DeleteSnapshot(snapshotID string) error {
	// Update ECS client if needed (for STS token refresh)
	if err := b.updateEcsClient(); err != nil {
		return errors.Wrapf(err, "failed to update ECS client for deleting snapshot %s", snapshotID)
	}

	req := &ecs20140526.DeleteSnapshotRequest{
		SnapshotId: tea.String(snapshotID),
	}

	_, err := b.client.DeleteSnapshot(req)
	if err != nil {
		// If it's a NotFound error, we don't need to return an error
		// since the snapshot is not there (similar to AWS plugin behavior)
		// Alibaba Cloud ECS returns error code "InvalidSnapshotId.NotFound" for non-existent snapshots
		var aliErr *alicloudErr.ServerError
		if errors.As(err, &aliErr) && aliErr.ErrorCode() == "InvalidSnapshotId.NotFound" {
			b.log.Warnf("snapshot %s is not found, skip deleting")
			return nil
		}
		return errors.Wrapf(err, "failed to delete snapshot %s", snapshotID)
	}

	return nil
}

// ============================================================================
// VolumeSnapshotter internal utility functions (not part of Velero plugin interface)
// ============================================================================

// updateEcsClient updates the ECS client with fresh credentials if using RAM role
func (b *VolumeSnapshotter) updateEcsClient() error {
	// Only update if we have RAM role
	if len(b.ramRole) == 0 {
		return nil
	}

	// Get new STS credentials
	accessKeyID, accessKeySecret, stsToken, err := getSTSAK(b.ramRole)
	if err != nil {
		return errors.Wrapf(err, "failed to get STS token for RAM role %s", b.ramRole)
	}

	cred := &ossCredentials{
		accessKeyID:     accessKeyID,
		accessKeySecret: accessKeySecret,
		stsToken:        stsToken,
	}

	// Create new ECS client with updated credentials
	rawClient, err := b.getEcsClient(cred)
	if err != nil {
		return errors.Wrapf(err, "failed to create new ECS client with updated credentials")
	}

	b.rawClient = rawClient
	b.client = &ecsClientWrapper{client: rawClient}
	return nil
}

// getEcsClient creates a new ECS client using the provided credentials
// This function only handles ECS client initialization, credentials should be obtained separately
func (b *VolumeSnapshotter) getEcsClient(cred *ossCredentials) (*ecs20140526.Client, error) {
	config := &openapi.Config{
		AccessKeyId:     tea.String(cred.accessKeyID),
		AccessKeySecret: tea.String(cred.accessKeySecret),
		RegionId:        tea.String(b.region),
	}

	if len(cred.stsToken) > 0 {
		config.SecurityToken = tea.String(cred.stsToken)
	}

	client, err := ecs20140526.NewClient(config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create ECS client with region %s", b.region)
	}

	return client, nil
}

// describeSnapshot describes a snapshot by ID
func (b *VolumeSnapshotter) describeSnapshot(snapshotID string) (*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshot, error) {
	req := &ecs20140526.DescribeSnapshotsRequest{
		RegionId:    tea.String(b.region),
		SnapshotIds: tea.String(fmt.Sprintf("[\"%s\"]", snapshotID)),
	}

	res, err := b.client.DescribeSnapshots(req)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to describe snapshot %s", snapshotID)
	}

	if res.Body == nil || res.Body.Snapshots == nil || res.Body.Snapshots.Snapshot == nil {
		return nil, errors.Errorf("invalid response from DescribeSnapshots for snapshot %s", snapshotID)
	}

	if count := len(res.Body.Snapshots.Snapshot); count != 1 {
		return nil, errors.Errorf("expected 1 snapshot from DescribeSnapshots for %s, got %d", snapshotID, count)
	}

	return res.Body.Snapshots.Snapshot[0], nil
}

// describeVolume describes a volume by ID
func (b *VolumeSnapshotter) describeVolume(volumeID string, volumeAZ string) (*ecs20140526.DescribeDisksResponseBodyDisksDisk, error) {
	req := &ecs20140526.DescribeDisksRequest{
		RegionId: tea.String(b.region),
		DiskIds:  tea.String(fmt.Sprintf("[\"%s\"]", volumeID)),
	}
	if volumeAZ != "" {
		req.ZoneId = tea.String(volumeAZ)
	}

	res, err := b.client.DescribeDisks(req)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to describe disk %s", volumeID)
	}

	if res.Body == nil || res.Body.Disks == nil || res.Body.Disks.Disk == nil {
		return nil, errors.Errorf("invalid response from DescribeDisks for disk %s", volumeID)
	}

	if count := len(res.Body.Disks.Disk); count != 1 {
		return nil, errors.Errorf("expected 1 disk from DescribeDisks for volume ID %s, got %d", volumeID, count)
	}

	return res.Body.Disks.Disk[0], nil
}

// getTagsForCluster processes snapshot tags and adds cluster-specific tags for restored volumes
func (b *VolumeSnapshotter) getTagsForCluster(snapshotTags []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag) []*ecs20140526.CreateDiskRequestTag {
	var result []*ecs20140526.CreateDiskRequestTag

	clusterName, haveACKClusterNameEnvVar := os.LookupEnv(ackClusterNameKey)

	if haveACKClusterNameEnvVar {
		result = append(result, &ecs20140526.CreateDiskRequestTag{
			Key:   tea.String("kubernetes.io/cluster/" + clusterName),
			Value: tea.String("owned"),
		})

		result = append(result, &ecs20140526.CreateDiskRequestTag{
			Key:   tea.String("KubernetesCluster"),
			Value: tea.String(clusterName),
		})
	}

	for _, tag := range snapshotTags {
		if tag == nil {
			continue
		}
		tagKey := tea.StringValue(tag.TagKey)
		if haveACKClusterNameEnvVar && (strings.HasPrefix(tagKey, "kubernetes.io/cluster/") || tagKey == "KubernetesCluster") {
			// if the ACK_CLUSTER_NAME variable is found we want current cluster
			// to overwrite the old ownership on volumes
			continue
		}

		result = append(result, &ecs20140526.CreateDiskRequestTag{
			Key:   tag.TagKey,
			Value: tag.TagValue,
		})
	}

	return result
}

// getTags processes Velero tags and volume tags to create snapshot tags
func (b *VolumeSnapshotter) getTags(veleroTags map[string]string, volumeTags []*ecs20140526.DescribeDisksResponseBodyDisksDiskTagsTag) []*ecs20140526.CreateSnapshotRequestTag {
	var result []*ecs20140526.CreateSnapshotRequestTag

	// set Velero-assigned tags
	for k, v := range veleroTags {
		result = append(result, &ecs20140526.CreateSnapshotRequestTag{
			Key:   tea.String(k),
			Value: tea.String(v),
		})
	}

	// copy tags from volume to snapshot
	for _, tag := range volumeTags {
		if tag == nil {
			continue
		}
		tagKey := tea.StringValue(tag.TagKey)
		// we want current Velero-assigned tags to overwrite any older versions
		// of them that may exist due to prior snapshots/restores
		if _, found := veleroTags[tagKey]; found {
			continue
		}

		result = append(result, &ecs20140526.CreateSnapshotRequestTag{
			Key:   tag.TagKey,
			Value: tag.TagValue,
		})
	}

	return result
}

// getTagsWithVolumeZone processes Velero tags and volume tags to create snapshot tags,
// and adds original volume AZ tag if not present
func (b *VolumeSnapshotter) getTagsWithVolumeZone(veleroTags map[string]string, volumeTags []*ecs20140526.DescribeDisksResponseBodyDisksDiskTagsTag, volumeZoneID string) []*ecs20140526.CreateSnapshotRequestTag {
	result := b.getTags(veleroTags, volumeTags)

	// Check if original volume AZ tag already exists in result
	hasOriginalVolumeAZTag := false
	for _, tag := range result {
		if tag != nil && tea.StringValue(tag.Key) == originalVolumeAZTagKey {
			hasOriginalVolumeAZTag = true
			break
		}
	}

	// If not present and volumeZoneID is available, add it
	if !hasOriginalVolumeAZTag && volumeZoneID != "" {
		result = append(result, &ecs20140526.CreateSnapshotRequestTag{
			Key:   tea.String(originalVolumeAZTagKey),
			Value: tea.String(volumeZoneID),
		})
	}

	return result
}

// determineVolumeAZ determines the availability zone for creating a volume from snapshot
// It tries to use the original zone from snapshot tags, falling back to current node zone
func (b *VolumeSnapshotter) determineVolumeAZ(snapshotTags []*ecs20140526.DescribeSnapshotsResponseBodySnapshotsSnapshotTagsTag) (string, error) {
	var originalZone, currentZone string

	// Try to get originalZone from snapshot tags (the zone where the original volume was created)
	for _, tag := range snapshotTags {
		if tag != nil && tea.StringValue(tag.TagKey) == originalVolumeAZTagKey {
			originalZone = tea.StringValue(tag.TagValue)
			break
		}
	}

	// Get currentZone from current node metadata (the zone where Velero is running) or config
	currentZone = b.zone

	// If originalZone matches currentZone, use originalZone
	if originalZone != "" && originalZone == currentZone {
		return originalZone, nil
	} else if originalZone != "" {
		// If originalZone exists but differs from currentZone, check if originalZone is in supported zones
		if b.supportedZones[originalZone] {
			return originalZone, nil
		} else {
			return currentZone, nil
		}
	} else {
		// No originalZone from snapshot tags, use currentZone
		return currentZone, nil
	}
}

// initKubeClient initializes a Kubernetes client using in-cluster config
// Returns nil if not running in a Kubernetes cluster
func (b *VolumeSnapshotter) initKubeClient() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrapf(err, "not running in cluster or failed to get in-cluster config")
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create Kubernetes client")
	}

	return client, nil
}

// loadSupportedZones loads supported zones from ack-cluster-profile ConfigMap in kube-system namespace.
// This is a best-effort operation - if it fails, we just continue without supported zones info.
// It handles Kubernetes client operations and delegates the actual parsing to parseClusterConfig.
func (b *VolumeSnapshotter) loadSupportedZones() error {
	if b.kubeClient == nil {
		return errors.New("Kubernetes client not available")
	}

	// Get ack-cluster-profile ConfigMap from kube-system namespace
	cm, err := b.kubeClient.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "ack-cluster-profile", metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get ack-cluster-profile ConfigMap")
	}

	zones, err := parseClusterConfig(cm.Data)
	if err != nil {
		return errors.Wrapf(err, "failed to parse cluster config")
	}

	b.supportedZones = zones
	b.log.Infof("loaded %d supported zones from ack-cluster-profile ConfigMap: %v", len(zones), zones)
	return nil
}

// ============================================================================
//  Utility functions (not part of Velero plugin interface)
// ============================================================================

// parseClusterConfig parses the vsw-zone field from ConfigMap data and extracts supported zones.
// The vsw-zone format is: vsw-xxx:zone1,vsw-yyy:zone2,...
// It returns a map of zone names (values after the colon) to true.
func parseClusterConfig(data map[string]string) (map[string]bool, error) {
	if data == nil {
		return nil, errors.New("data is nil")
	}

	// Extract vsw-zone field from ConfigMap data
	vswZoneData, ok := data["vsw-zone"]
	if !ok {
		return nil, errors.New("vsw-zone field not found in ack-cluster-profile ConfigMap")
	}

	// Parse vsw-zone format: vsw-xxx:zone1,vsw-yyy:zone2,...
	// Extract zones (values after the colon)
	zones := make(map[string]bool)
	pairs := strings.Split(vswZoneData, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		// Split by colon to get vsw-id:zone
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) == 2 {
			zone := strings.TrimSpace(parts[1])
			if zone != "" {
				zones[zone] = true
			}
		}
	}
	return zones, nil
}

// checkCSIVolumeDriver validates CSI volume driver
func checkCSIVolumeDriver(driver string) error {
	if driver != "diskplugin.csi.alibabacloud.com" {
		return errors.Errorf("unsupported CSI driver: %s", driver)
	}
	return nil
}

// checkFlexVolumeDriver validates FlexVolume driver
func checkFlexVolumeDriver(driver string) error {
	if driver != "alicloud/disk" {
		return errors.Errorf("unsupported FlexVolume driver: %s", driver)
	}
	return nil
}

// getEBSDiskID extracts disk ID from PersistentVolume
// Returns empty string and nil error if neither CSI nor FlexVolume is found (for compatibility)
func getEBSDiskID(pv *v1.PersistentVolume) (string, error) {
	if pv.Spec.CSI != nil {
		if err := checkCSIVolumeDriver(pv.Spec.CSI.Driver); err != nil {
			return "", err
		}
		handle := pv.Spec.CSI.VolumeHandle
		if handle == "" {
			return "", errors.New("spec.CSI.VolumeHandle not found")
		}
		return handle, nil
	}
	if pv.Spec.FlexVolume != nil {
		if err := checkFlexVolumeDriver(pv.Spec.FlexVolume.Driver); err != nil {
			return "", err
		}
		options := pv.Spec.FlexVolume.Options
		if options == nil || (options["VolumeId"] == "" && options["volumeId"] == "") {
			return "", errors.New("spec.FlexVolume.Options['VolumeId'] or spec.FlexVolume.Options['volumeId'] not found")
		} else if options["VolumeId"] != "" {
			return options["VolumeId"], nil
		} else {
			return options["volumeId"], nil
		}
	}
	return "", nil
}

// setEBSDiskID sets disk ID in PersistentVolume
func setEBSDiskID(pv *v1.PersistentVolume, diskID string) error {
	if pv.Spec.CSI != nil {
		if err := checkCSIVolumeDriver(pv.Spec.CSI.Driver); err != nil {
			return err
		}
		pv.Spec.CSI.VolumeHandle = diskID
		return nil
	}
	if pv.Spec.FlexVolume != nil {
		if err := checkFlexVolumeDriver(pv.Spec.FlexVolume.Driver); err != nil {
			return err
		}
		options := pv.Spec.FlexVolume.Options
		if options == nil {
			options = map[string]string{}
			pv.Spec.FlexVolume.Options = options
		}
		options["VolumeId"] = diskID
		return nil
	}
	return errors.New("spec.CSI or spec.FlexVolume not found")
}

// getPerformanceLevelFromIOPS converts IOPS value to Alibaba Cloud Performance Level
// PL0: up to 10,000 random read/write IOPS
// PL1: up to 50,000 random read/write IOPS
// PL2: up to 100,000 random read/write IOPS
// PL3: up to 1,000,000 random read/write IOPS
func getPerformanceLevelFromIOPS(iops int64) string {
	if iops <= DiskPerformanceLevels["PL0"] {
		return "PL0"
	}
	if iops <= DiskPerformanceLevels["PL1"] {
		return "PL1"
	}
	if iops <= DiskPerformanceLevels["PL2"] {
		return "PL2"
	}
	// For IOPS greater than 100,000, use PL3 (or cap at PL3)
	return "PL3"
}
