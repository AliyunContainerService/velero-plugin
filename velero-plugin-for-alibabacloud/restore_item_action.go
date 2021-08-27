package main

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type RestoreItemAction struct {
	log logrus.FieldLogger
}

//
func newRestoreItemAction(logger logrus.FieldLogger) *RestoreItemAction {
	return &RestoreItemAction{log: logger}
}

// AppliesTo returns information about which resources this action should be invoked for.
// A RestoreItemAction's Execute function will only be invoked on items that match the returned
// selector. A zero-valued ResourceSelector matches all resources.g
func (p *RestoreItemAction) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"pvc", "persistentvolume"},
	}, nil
}

// Execute allows the RestorePlugin to perform arbitrary logic with the item being restored,
// in this case, setting a custom annotation on the item being restored.
func (p *RestoreItemAction) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	p.log.Info("Alibaba Cloud RestorePlugin!")

	var kind string
	var err error
	var ok bool
	inputMap := input.Item.UnstructuredContent()

	if kind, ok = inputMap[kindKey].(string); !ok {
		return nil, errors.WithStack(err)
	}

	metadata, err := meta.Accessor(input.Item)
	if err != nil {
		return &velero.RestoreItemActionExecuteOutput{}, err
	}

	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations["velero.io/alibabacloud-restore-plugin"] = "1"

	metadata.SetAnnotations(annotations)

	switch kind {
	case persistentVolumeClaimKey:
		var pvc corev1api.PersistentVolumeClaim
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &pvc); err != nil {
			return nil, errors.WithStack(err)
		}
		capacity := pvc.Spec.Resources.Requests[corev1api.ResourceName(corev1api.ResourceStorage)]
		volSizeBytes := capacity.Value()
		if int64(volSizeBytes) <= int64(minReqVolSizeBytes) {
			p.log.Warnf("Alibaba disk volume request at least 20Gi, auto resize persistentVolumeClaim to 20Gi.")
			pvc.Spec.Resources = corev1api.ResourceRequirements{
				Requests: getResourceList(minReqVolSizeString),
			}
			pvc.Status = corev1api.PersistentVolumeClaimStatus{}
			inputMap, err = runtime.DefaultUnstructuredConverter.ToUnstructured(&pvc)
			if err != nil {
				return nil, errors.WithStack(err)
			}
		}
	case persistentVolumeKey:
		var pv corev1api.PersistentVolume
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &pv); err != nil {
			return nil, errors.WithStack(err)
		}
		capacity := pv.Spec.Capacity[corev1api.ResourceName(corev1api.ResourceStorage)]
		volSizeBytes := capacity.Value()
		if int64(volSizeBytes) <= int64(minReqVolSizeBytes) {
			p.log.Warnf("Alibaba disk volume request at least 20Gi, auto resize persistentVolume to 20Gi.")
			persistentVolumeSource := pv.Spec.PersistentVolumeSource
			accessModes := pv.Spec.AccessModes
			claimRef := pv.Spec.ClaimRef
			persistentVolumeReclaimPolicy := pv.Spec.PersistentVolumeReclaimPolicy
			storageClassName := pv.Spec.StorageClassName
			mountOptions := pv.Spec.MountOptions
			volumeMode := pv.Spec.VolumeMode
			nodeAffinity := pv.Spec.NodeAffinity
			if len(nodeAffinity.Required.NodeSelectorTerms) > 0 && len(nodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions) > 0 {
				if nodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Key == "topology.diskplugin.csi.alibabacloud.com/zone" {
					volumeAZ, err := getMetaData(metadataZoneKey)
					if err != nil {
						return nil, errors.Errorf("Set NodeAffinity failed to get zone-id, got %v", err)
					}
					nodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Values = []string{volumeAZ}
				}
			}

			pv.Spec = corev1api.PersistentVolumeSpec{
				Capacity:                      getResourceList(minReqVolSizeString),
				PersistentVolumeSource:        persistentVolumeSource,
				AccessModes:                   accessModes,
				ClaimRef:                      claimRef,
				PersistentVolumeReclaimPolicy: persistentVolumeReclaimPolicy,
				StorageClassName:              storageClassName,
				MountOptions:                  mountOptions,
				VolumeMode:                    volumeMode,
				NodeAffinity:                  nodeAffinity,
			}
			pv.Status = corev1api.PersistentVolumeStatus{}
			inputMap, err = runtime.DefaultUnstructuredConverter.ToUnstructured(&pv)
			if err != nil {
				return nil, errors.WithStack(err)
			}
		}
	default:
		p.log.Info("Nothing need to do, skip")
	}
	return velero.NewRestoreItemActionExecuteOutput(&unstructured.Unstructured{Object: inputMap}), nil
}

func getResourceList(storage string) corev1api.ResourceList {
	res := corev1api.ResourceList{}
	if storage != "" {
		res[corev1api.ResourceStorage] = resource.MustParse(storage)
	}
	return res
}
