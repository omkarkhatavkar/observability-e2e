/*
Copyright © 2024 - 2025 SUSE LLC

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

package migration

import (
	"os"
	"testing"

	localTerraform "github.com/rancher/observability-e2e/tests/helper/terraform"

	terraform "github.com/gruntwork-io/terratest/modules/terraform"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var tfCtx *localTerraform.TerraformContext

func FailWithReport(message string, callerSkip ...int) {
	// Ensures the correct line numbers are reported
	Fail(message, callerSkip[0]+1)
}

// Run individual or group of tests with labels using CLI
// TEST_LABEL_FILTER=backup-restore-migration  /usr/local/go/bin/go test -timeout 60m github.com/rancher/observability-e2e/tests/backuprestore/migration -v -count=1 -ginkgo.v
func TestE2E(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	suiteConfig, reporterConfig := GinkgoConfiguration()

	// Set the label filter to "LEVEL0" (or any other test with custom tag)
	if envLabelFilter := os.Getenv("TEST_LABEL_FILTER"); envLabelFilter != "" {
		suiteConfig.LabelFilter = envLabelFilter
	} else {
		suiteConfig.LabelFilter = "LEVEL0"
	}
	e2e.Logf("Executing tests with label '%v'", suiteConfig.LabelFilter)
	RunSpecs(t, "Rancher Migration Test Suite", suiteConfig, reporterConfig)
}

var _ = BeforeSuite(func() {
	var err error
	// Set up the Terraform context pointing to your configuration
	tfCtx, err = localTerraform.NewTerraformContext(localTerraform.TerraformOptions{
		// Relative path from the test file location to the Terraform config folder.
		TerraformDir: "../../../resources/terraform/config/",
	})
	Expect(err).ToNot(HaveOccurred(), "Failed to create Terraform context")

	// Initialize and apply the Terraform configuration.
	terraform.InitAndApply(GinkgoT(), tfCtx.Options)
})

var _ = AfterSuite(func() {
	// Tear down the infrastructure after all tests finish.
	if tfCtx != nil {
		terraform.Destroy(GinkgoT(), tfCtx.Options)
	}
})
