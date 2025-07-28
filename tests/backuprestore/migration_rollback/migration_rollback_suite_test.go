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

package migration_rollaback

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rancher/norman/types"
	"github.com/rancher/observability-e2e/resources"
	"github.com/rancher/observability-e2e/tests/helper/charts"
	localConfig "github.com/rancher/observability-e2e/tests/helper/config"
	localTerraform "github.com/rancher/observability-e2e/tests/helper/terraform"
	"github.com/rancher/observability-e2e/tests/helper/utils"
	"github.com/rancher/rancher/tests/v2/actions/pipeline"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/cloudcredentials"
	"github.com/rancher/shepherd/extensions/cloudcredentials/aws"
	"github.com/rancher/shepherd/extensions/clusters"
	"github.com/rancher/shepherd/pkg/config"
	session "github.com/rancher/shepherd/pkg/session"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var tfCtx *localTerraform.TerraformContext

// Sensitive secrets passed via env vars
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

var (
	client              *rancher.Client
	sess                *session.Session
	project             *management.Project
	cluster             *clusters.ClusterMeta
	registrySetting     *management.Setting
	s3Client            *resources.S3Client
	BackupRestoreConfig *localConfig.BackupRestoreConfig
	skipS3Tests         bool
	CloudCredentialName string
	CredentialConfig    *cloudcredentials.AmazonEC2CredentialConfig
)

const (
	exampleAppProjectName = "System"
	providerName          = "aws"
)

func FailWithReport(message string, callerSkip ...int) {
	// Ensures the correct line numbers are reported
	Fail(message, callerSkip[0]+1)
}

// Run individual or group of tests with labels using CLI
// TEST_LABEL_FILTER=backup-restore-migration-rollback  /usr/local/go/bin/go test -timeout 60m github.com/rancher/observability-e2e/tests/backuprestore/migration-rollback  -v -count=1 -ginkgo.v
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
	RunSpecs(t, "Rancher Migration/Rollback Test Suite", suiteConfig, reporterConfig)
}

var _ = BeforeEach(func() {
	By("Loading Terraform variables from environment")
	terraformVars := localTerraform.LoadVarsFromEnv(envTerraformVarMap)

	err := localTerraform.SetTerraformEnvVarsFromMap(envSecretsTerraformVarMap)
	if err != nil {
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

	testSession := session.NewSession()
	sess = testSession

	By("Creating Rancher client")
	client, err = rancher.NewClient("", testSession)
	Expect(err).NotTo(HaveOccurred())

	By("Retrieving cluster metadata")
	clusterName := client.RancherConfig.ClusterName
	Expect(clusterName).NotTo(BeEmpty(), "Cluster name to install is not set")
	cluster, err = clusters.NewClusterMeta(client, clusterName)
	Expect(err).NotTo(HaveOccurred())

	By("Retrieving system-default-registry setting")
	registrySetting, err = client.Management.Setting.ByID("system-default-registry")
	Expect(err).NotTo(HaveOccurred())

	By("Locating or creating system project")
	projectsList, err := client.Management.Project.List(&types.ListOpts{
		Filters: map[string]interface{}{
			"clusterId": cluster.ID,
		},
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

var _ = AfterSuite(func() {
	By("Destroying Terraform infrastructure")
	if tfCtx != nil {
		_, err := tfCtx.DestroyTarget("module.ec2.aws_instance.rke2_node")
		Expect(err).ToNot(HaveOccurred(), "Failed to Destroy Terraform Resource")
	}
})
