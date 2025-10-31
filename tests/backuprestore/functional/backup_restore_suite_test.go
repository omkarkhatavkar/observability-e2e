/*
Copyright © 2024 - 2025 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at:

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package backuprestore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	//. "github.com/rancher-sandbox/qase-ginkgo"
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/norman/types"
	"github.com/rancher/observability-e2e/resources"
	"github.com/rancher/observability-e2e/tests/helper/charts"
	localConfig "github.com/rancher/observability-e2e/tests/helper/config"
	localTerraform "github.com/rancher/observability-e2e/tests/helper/terraform"
	"github.com/rancher/observability-e2e/tests/helper/utils"
	"github.com/rancher/rancher/tests/v2/actions/pipeline"
	rancher "github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/cloudcredentials"
	"github.com/rancher/shepherd/extensions/cloudcredentials/aws"
	clusters "github.com/rancher/shepherd/extensions/clusters"
	"github.com/rancher/shepherd/pkg/config"
	session "github.com/rancher/shepherd/pkg/session"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var (
	client              *rancher.Client
	sess                *session.Session
	project             *management.Project
	cluster             *clusters.ClusterMeta
	registrySetting     *management.Setting
	s3Client            *resources.S3Client
	BackupRestoreConfig *localConfig.BackupRestoreConfig
	CredentialConfig    *cloudcredentials.AmazonEC2CredentialConfig
	tfCtx               *localTerraform.TerraformContext

	CloudCredentialName string
	labelFilter         string
	skipS3Tests         bool
	err                 error
)

const (
	exampleAppProjectName = "System"
	providerName          = "aws"
)

// Secrets passed via env vars
var envSecretsTerraformVarMap = map[string]string{
	"ENCRYPTION_SECRET_KEY": "encryption_secret_key",
	"RANCHER_PASSWORD":      "rancher_password",
	"KEY_NAME":              "key_name",
}

// Non-sensitive config passed directly
var envTerraformVarMap = map[string]string{
	"CERT_MANAGER_VERSION": "cert_manager_version",
	"RANCHER_VERSION":      "rancher_version",
	"RKE2_VERSION":         "rke2_version",
	"RANCHER_REPO_URL":     "rancher_repo_url",
}

var testCaseID int64

var _ = ReportAfterEach(func(report SpecReport) {
	if testCaseID == 0 {
		return
	}
	// Add result in Qase if asked
	Qase(testCaseID, report)

	// This prevents the ID from "leaking" into the next test's ReportAfterEach execution.
	testCaseID = 0
})

func FailWithReport(message string, callerSkip ...int) {
	// Ensures the correct line numbers are reported
	Fail(message, callerSkip[0]+1)
}

// Run tests with labels from CLI:
// TEST_LABEL_FILTER=backup-restore go test -timeout 60m github.com/rancher/observability-e2e/tests/backuprestore/functional -v -count=1 -ginkgo.v
func TestE2E(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	suiteConfig, reporterConfig := GinkgoConfiguration()

	// Set the label filter (default: LEVEL0)
	if envLabelFilter := os.Getenv("TEST_LABEL_FILTER"); envLabelFilter != "" {
		suiteConfig.LabelFilter = envLabelFilter
	} else {
		suiteConfig.LabelFilter = "LEVEL0"
	}

	e2e.Logf("Executing tests with label '%v'", suiteConfig.LabelFilter)
	RunSpecs(t, "Backup and Restore End-To-End Test Suite", suiteConfig, reporterConfig)
}

// -------------------------
// BeforeSuite: global setup
// -------------------------
var _ = BeforeSuite(func() {
	suiteConfig, _ := GinkgoConfiguration()
	labelFilter = suiteConfig.LabelFilter

	// Run Terraform unless we’re in "installation" tests
	if !strings.Contains(labelFilter, "installation") {
		By("Loading Terraform variables from environment")
		terraformVars := localTerraform.LoadVarsFromEnv(envTerraformVarMap)

		if err = localTerraform.SetTerraformEnvVarsFromMap(envSecretsTerraformVarMap); err != nil {
			e2e.Logf("Failed to set secret TF_VAR_*: %v", err)
		}

		By("Creating Terraform context")
		tfCtx, err = localTerraform.NewTerraformContext(localTerraform.TerraformOptions{
			TerraformDir: "../../../resources/terraform/config/",
			Vars:         terraformVars,
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create Terraform context")

		By("Initializing and applying Terraform configuration")
		_, err = tfCtx.InitAndApply()
		Expect(err).ToNot(HaveOccurred(), "Failed to init/apply Terraform context")
		time.Sleep(3 * time.Minute)

		By("Loading Rancher config and creating admin token")
		rancherConfig := new(rancher.Config)
		config.LoadConfig(rancher.ConfigurationFileKey, rancherConfig)

		token, err := pipeline.CreateAdminToken(os.Getenv("RANCHER_PASSWORD"), rancherConfig)
		Expect(err).To(BeNil())

		rancherConfig.AdminToken = token
		config.UpdateConfig(rancher.ConfigurationFileKey, rancherConfig)

		By("Loading AWS credential config")
		CredentialConfig = new(cloudcredentials.AmazonEC2CredentialConfig)
		config.LoadAndUpdateConfig("awsCredentials", CredentialConfig, func() {
			CredentialConfig.AccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
			CredentialConfig.SecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
			CredentialConfig.DefaultRegion = os.Getenv("DEFAULT_REGION")
		})

	} else {
		e2e.Logf("Skipping Terraform setup because label filter is '%s'", labelFilter)
	}

	testSession := session.NewSession()
	sess = testSession

	By("Creating Rancher client")
	client, err = rancher.NewClient("", testSession)
	Expect(err).NotTo(HaveOccurred())

	By("Retrieving cluster metadata")
	clusterName := client.RancherConfig.ClusterName
	Expect(clusterName).NotTo(BeEmpty(), "Cluster name is not set")
	cluster, err = clusters.NewClusterMeta(client, clusterName)
	Expect(err).NotTo(HaveOccurred())

	By("Retrieving system-default-registry setting")
	registrySetting, err = client.Management.Setting.ByID("system-default-registry")
	Expect(err).NotTo(HaveOccurred())

	By("Locating or creating system project")
	projectsList, err := client.Management.Project.List(&types.ListOpts{
		Filters: map[string]interface{}{"clusterId": cluster.ID},
	})
	Expect(err).NotTo(HaveOccurred())

	for i := range projectsList.Data {
		p := &projectsList.Data[i]
		if p.Name == exampleAppProjectName {
			project = p
			break
		}
	}

	if project == nil {
		projectConfig := &management.Project{
			ClusterID: cluster.ID,
			Name:      exampleAppProjectName,
		}
		project, err = client.Management.Project.Create(projectConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(project.Name).To(Equal(exampleAppProjectName))
	}

	By("Creating AWS cloud credentials")
	cloudCredentialConfig := cloudcredentials.LoadCloudCredential(providerName)
	cloudCredential, err := aws.CreateAWSCloudCredentials(client, cloudCredentialConfig)
	Expect(err).NotTo(HaveOccurred())

	CloudCredentialName = strings.Replace(cloudCredential.ID, "/", ":", 1)
	Expect(CloudCredentialName).To(ContainSubstring("cc"))

	By("Loading backup/restore config and setting dynamic S3 bucket name")
	BackupRestoreConfig = &localConfig.BackupRestoreConfig{}
	filePath, _ := filepath.Abs(charts.BackupRestoreConfigurationFileKey)

	err = utils.LoadConfigIntoStruct(filePath, BackupRestoreConfig)
	Expect(err).NotTo(HaveOccurred())

	BackupRestoreConfig.S3BucketName = fmt.Sprintf("backup-restore-automation-test-%d", time.Now().Unix())

	if BackupRestoreConfig.AccessKey != "" {
		By("Creating S3 client and S3 bucket")
		s3Client, err = resources.NewS3Client(BackupRestoreConfig)
		Expect(err).NotTo(HaveOccurred())

		err = s3Client.CreateBucket(BackupRestoreConfig.S3BucketName, BackupRestoreConfig.S3Region)
		Expect(err).NotTo(HaveOccurred())

		e2e.Logf("S3 bucket '%s' created successfully", BackupRestoreConfig.S3BucketName)
	} else {
		skipS3Tests = true
	}
})

// -------------------------
// AfterSuite: global teardown
// -------------------------
var _ = AfterSuite(func() {
	if BackupRestoreConfig.AccessKey != "" {
		By("Deleting the S3 bucket")
		err := s3Client.DeleteBucket(BackupRestoreConfig.S3BucketName)
		Expect(err).NotTo(HaveOccurred())
		e2e.Logf("S3 bucket '%s' deleted successfully", BackupRestoreConfig.S3BucketName)
	}
	if !strings.Contains(labelFilter, "installation") {
		By("Destroying Terraform infrastructure")
		if tfCtx != nil {
			_, err := tfCtx.Destroy()
			Expect(err).ToNot(HaveOccurred(), "Failed to destroy Terraform resources")
		}
	}

	By("Cleaning up test session")
	sess.Cleanup()
})
