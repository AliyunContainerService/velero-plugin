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
	case persistentVolumeKey:
		var pv corev1api.PersistentVolume
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &pv); err != nil {
			return nil, errors.WithStack(err)
		}

		if pv.Spec.FlexVolume != nil && pv.Spec.FlexVolume.Options != nil && pv.Spec.FlexVolume.Options[OriginStr] != "" {
			pv.Spec.FlexVolume.Options[TargetStr] = pv.Spec.FlexVolume.Options[OriginStr]
			p.log.Info("Modify FlexVolume options for PV")
		}

		inputMap, err = runtime.DefaultUnstructuredConverter.ToUnstructured(&pv)
		if err != nil {
			return nil, errors.WithStack(err)
		}

	default:
		// do nothing
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
