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
	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
	"github.com/sirupsen/logrus"
)

func main() {
	veleroplugin.NewServer().
		RegisterObjectStore("velero.io/alibabacloud", newAlibabaCloudObjectStore).
		RegisterVolumeSnapshotter("velero.io/alibabacloud", newAlibabaCloudVolumeSnapshotter).
		RegisterRestoreItemAction("velero.io/alibabacloud", newAlibabaCloudRestoreItemAction).
		Serve()
}

func newAlibabaCloudObjectStore(logger logrus.FieldLogger) (interface{}, error) {
	return newObjectStore(logger), nil
}

func newAlibabaCloudVolumeSnapshotter(logger logrus.FieldLogger) (interface{}, error) {
	return newVolumeSnapshotter(logger), nil
}

func newAlibabaCloudRestoreItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return newRestoreItemAction(logger), nil
}
