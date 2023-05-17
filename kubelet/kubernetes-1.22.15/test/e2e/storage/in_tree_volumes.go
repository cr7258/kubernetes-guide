/*
Copyright 2018 The Kubernetes Authors.

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

package storage

import (
	"github.com/onsi/ginkgo"
	"k8s.io/kubernetes/test/e2e/storage/drivers"
	storageframework "k8s.io/kubernetes/test/e2e/storage/framework"
	"k8s.io/kubernetes/test/e2e/storage/testsuites"
	"k8s.io/kubernetes/test/e2e/storage/utils"
)

// List of testDrivers to be executed in below loop
var testDrivers = []func() storageframework.TestDriver{
	drivers.InitNFSDriver,
	drivers.InitGlusterFSDriver,
	drivers.InitISCSIDriver,
	drivers.InitRbdDriver,
	drivers.InitCephFSDriver,
	drivers.InitHostPathDriver,
	drivers.InitHostPathSymlinkDriver,
	drivers.InitEmptydirDriver,
	drivers.InitCinderDriver,
	drivers.InitGcePdDriver,
	drivers.InitWindowsGcePdDriver,
	drivers.InitVSphereDriver,
	drivers.InitAzureDiskDriver,
	drivers.InitAwsDriver,
	drivers.InitLocalDriverWithVolumeType(utils.LocalVolumeDirectory),
	drivers.InitLocalDriverWithVolumeType(utils.LocalVolumeDirectoryLink),
	drivers.InitLocalDriverWithVolumeType(utils.LocalVolumeDirectoryBindMounted),
	drivers.InitLocalDriverWithVolumeType(utils.LocalVolumeDirectoryLinkBindMounted),
	drivers.InitLocalDriverWithVolumeType(utils.LocalVolumeTmpfs),
	drivers.InitLocalDriverWithVolumeType(utils.LocalVolumeBlock),
	drivers.InitLocalDriverWithVolumeType(utils.LocalVolumeBlockFS),
	drivers.InitLocalDriverWithVolumeType(utils.LocalVolumeGCELocalSSD),
}

// This executes testSuites for in-tree volumes.
var _ = utils.SIGDescribe("In-tree Volumes", func() {
	for _, initDriver := range testDrivers {
		curDriver := initDriver()

		ginkgo.Context(storageframework.GetDriverNameWithFeatureTags(curDriver), func() {
			storageframework.DefineTestSuites(curDriver, testsuites.BaseSuites)
		})
	}
})
