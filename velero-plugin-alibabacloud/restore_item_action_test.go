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
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestRestoreItemAction_AppliesTo(t *testing.T) {
	action := newRestoreItemAction(logrus.New())
	selector, err := action.AppliesTo()

	require.NoError(t, err)
	assert.Equal(t, []string{"pvc", "persistentvolume"}, selector.IncludedResources)
}

func TestRestoreItemAction_Execute_PVC(t *testing.T) {
	action := newRestoreItemAction(logrus.New())

	// Create a PVC unstructured object
	pvc := &corev1api.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind: "PersistentVolumeClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pvc",
		},
	}

	unstructuredPVC, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pvc)
	require.NoError(t, err)

	input := &velero.RestoreItemActionExecuteInput{
		Item: &unstructured.Unstructured{Object: unstructuredPVC},
	}

	output, err := action.Execute(input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify no modifications were made (UpdatedItem is nil for non-PV resources)
	assert.Nil(t, output.UpdatedItem, "PVC should not be modified, UpdatedItem should be nil")
}

func TestRestoreItemAction_Execute_PV_WithoutFlexVolume(t *testing.T) {
	action := newRestoreItemAction(logrus.New())

	// Create a PV without FlexVolume
	pv := &corev1api.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind: "PersistentVolume",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
		Spec: corev1api.PersistentVolumeSpec{
			Capacity: corev1api.ResourceList{
				corev1api.ResourceStorage: resource.MustParse("10Gi"),
			},
		},
	}

	unstructuredPV, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pv)
	require.NoError(t, err)

	input := &velero.RestoreItemActionExecuteInput{
		Item: &unstructured.Unstructured{Object: unstructuredPV},
	}

	output, err := action.Execute(input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotNil(t, output.UpdatedItem, "PV should be processed and UpdatedItem should not be nil")

	// Verify the item was returned (even if no FlexVolume modifications were made)
	resultItem := output.UpdatedItem.UnstructuredContent()
	kind, ok := resultItem["kind"].(string)
	require.True(t, ok)
	assert.Equal(t, "PersistentVolume", kind)
}

func TestRestoreItemAction_Execute_PV_WithFlexVolume(t *testing.T) {
	action := newRestoreItemAction(logrus.New())

	// Create a PV with FlexVolume containing volumeId
	pv := &corev1api.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind: "PersistentVolume",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
		Spec: corev1api.PersistentVolumeSpec{
			PersistentVolumeSource: corev1api.PersistentVolumeSource{
				FlexVolume: &corev1api.FlexPersistentVolumeSource{
					Driver: "alicloud/disk",
					Options: map[string]string{
						OriginStr: "d-test123",
					},
				},
			},
		},
	}

	unstructuredPV, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pv)
	require.NoError(t, err)

	input := &velero.RestoreItemActionExecuteInput{
		Item: &unstructured.Unstructured{Object: unstructuredPV},
	}

	output, err := action.Execute(input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotNil(t, output.UpdatedItem, "PV should be processed and UpdatedItem should not be nil")

	// Verify FlexVolume options were modified
	resultItem := output.UpdatedItem.UnstructuredContent()
	spec, ok := resultItem["spec"].(map[string]interface{})
	require.True(t, ok)
	flexVolume, ok := spec["flexVolume"].(map[string]interface{})
	require.True(t, ok)
	options, ok := flexVolume["options"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "d-test123", options[OriginStr])
	assert.Equal(t, "d-test123", options[TargetStr])
}

func TestRestoreItemAction_Execute_PV_WithFlexVolume_NoVolumeId(t *testing.T) {
	action := newRestoreItemAction(logrus.New())

	// Create a PV with FlexVolume but without volumeId
	pv := &corev1api.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind: "PersistentVolume",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
		Spec: corev1api.PersistentVolumeSpec{
			PersistentVolumeSource: corev1api.PersistentVolumeSource{
				FlexVolume: &corev1api.FlexPersistentVolumeSource{
					Driver: "alicloud/disk",
					Options: map[string]string{
						"otherKey": "otherValue",
					},
				},
			},
		},
	}

	unstructuredPV, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pv)
	require.NoError(t, err)

	input := &velero.RestoreItemActionExecuteInput{
		Item: &unstructured.Unstructured{Object: unstructuredPV},
	}

	output, err := action.Execute(input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotNil(t, output.UpdatedItem, "PV should be processed and UpdatedItem should not be nil")

	// Verify FlexVolume options were not modified (no volumeId)
	resultItem := output.UpdatedItem.UnstructuredContent()
	spec, ok := resultItem["spec"].(map[string]interface{})
	require.True(t, ok)
	flexVolume, ok := spec["flexVolume"].(map[string]interface{})
	require.True(t, ok)
	options, ok := flexVolume["options"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "otherValue", options["otherKey"])
	_, exists := options[TargetStr]
	assert.False(t, exists, "TargetStr should not be added when OriginStr is empty")
}

func TestRestoreItemAction_Execute_InvalidKind(t *testing.T) {
	action := newRestoreItemAction(logrus.New())

	// Create an object without kind field
	invalidItem := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test",
			},
		},
	}

	input := &velero.RestoreItemActionExecuteInput{
		Item: invalidItem,
	}

	output, err := action.Execute(input)
	assert.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "failed to get kind from input item")
}
