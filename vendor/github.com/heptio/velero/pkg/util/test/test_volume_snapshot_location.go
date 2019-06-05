/*
Copyright 2018 the Velero contributors.

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

package test

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/heptio/velero/pkg/apis/velero/v1"
)

type TestVolumeSnapshotLocation struct {
	*v1.VolumeSnapshotLocation
}

func NewTestVolumeSnapshotLocation() *TestVolumeSnapshotLocation {
	return &TestVolumeSnapshotLocation{
		VolumeSnapshotLocation: &v1.VolumeSnapshotLocation{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: v1.DefaultNamespace,
			},
			Spec: v1.VolumeSnapshotLocationSpec{
				Provider: "aws",
				Config:   map[string]string{"region": "us-west-1"},
			},
		},
	}
}

func (location *TestVolumeSnapshotLocation) WithName(name string) *TestVolumeSnapshotLocation {
	location.Name = name
	return location
}

func (location *TestVolumeSnapshotLocation) WithProvider(name string) *TestVolumeSnapshotLocation {
	location.Spec.Provider = name
	return location
}

func (location *TestVolumeSnapshotLocation) WithProviderConfig(info []LocationInfo) []*TestVolumeSnapshotLocation {
	var locations []*TestVolumeSnapshotLocation

	for _, v := range info {
		location := &TestVolumeSnapshotLocation{
			VolumeSnapshotLocation: &v1.VolumeSnapshotLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v.Name,
					Namespace: v1.DefaultNamespace,
				},
				Spec: v1.VolumeSnapshotLocationSpec{
					Provider: v.Provider,
					Config:   v.Config,
				},
			},
		}
		locations = append(locations, location)
	}
	return locations
}

type LocationInfo struct {
	Name, Provider string
	Config         map[string]string
}
