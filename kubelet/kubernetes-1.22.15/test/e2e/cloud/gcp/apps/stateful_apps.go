/*
Copyright 2021 The Kubernetes Authors.

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

package apps

import (
	"k8s.io/kubernetes/test/e2e/cloud/gcp/common"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/upgrades"
	"k8s.io/kubernetes/test/e2e/upgrades/apps"
	"k8s.io/kubernetes/test/utils/junit"

	"github.com/onsi/ginkgo"
)

var upgradeTests = []upgrades.Test{
	&apps.MySQLUpgradeTest{},
	&apps.EtcdUpgradeTest{},
	&apps.CassandraUpgradeTest{},
}

var _ = SIGDescribe("stateful Upgrade [Feature:StatefulUpgrade]", func() {
	f := framework.NewDefaultFramework("stateful-upgrade")
	testFrameworks := upgrades.CreateUpgradeFrameworks(upgradeTests)

	ginkgo.Describe("stateful upgrade", func() {
		ginkgo.It("should maintain a functioning cluster", func() {
			upgCtx, err := common.GetUpgradeContext(f.ClientSet.Discovery())
			framework.ExpectNoError(err)

			testSuite := &junit.TestSuite{Name: "Stateful upgrade"}
			statefulUpgradeTest := &junit.TestCase{Name: "[sig-apps] stateful-upgrade", Classname: "upgrade_tests"}
			testSuite.TestCases = append(testSuite.TestCases, statefulUpgradeTest)

			upgradeFunc := common.ClusterUpgradeFunc(f, upgCtx, statefulUpgradeTest, nil, nil)
			upgrades.RunUpgradeSuite(upgCtx, upgradeTests, testFrameworks, testSuite, upgrades.ClusterUpgrade, upgradeFunc)
		})
	})
})
