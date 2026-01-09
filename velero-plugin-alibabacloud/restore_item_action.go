package main

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type RestoreItemAction struct {
	log logrus.FieldLogger
}

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
// in this case, modifying PersistentVolume FlexVolume options.
func (p *RestoreItemAction) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	var kind string
	var ok bool
	inputMap := input.Item.UnstructuredContent()

	if kind, ok = inputMap[kindKey].(string); !ok {
		return nil, errors.New("failed to get kind from input item")
	}

	// Only process PersistentVolume resources, return nil UpdatedItem for others (no modifications)
	if kind != persistentVolumeKey {
		return &velero.RestoreItemActionExecuteOutput{}, nil
	}

	p.log.Info("Alibaba Cloud RestorePlugin processing PersistentVolume")

	var pv corev1api.PersistentVolume
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &pv); err != nil {
		return nil, errors.WithStack(err)
	}

	// Modify FlexVolume options: copy volumeId to VolumeId
	if pv.Spec.FlexVolume != nil && pv.Spec.FlexVolume.Options != nil && pv.Spec.FlexVolume.Options[OriginStr] != "" {
		pv.Spec.FlexVolume.Options[TargetStr] = pv.Spec.FlexVolume.Options[OriginStr]
		p.log.Infof("Modify FlexVolume PersistentVolume %s Options", pv.Name)
	}

	inputMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pv)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return velero.NewRestoreItemActionExecuteOutput(&unstructured.Unstructured{Object: inputMap}), nil
}
