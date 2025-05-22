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
	"github.com/rancher/norman/types"
	"github.com/rancher/observability-e2e/resources"
	"github.com/rancher/observability-e2e/tests/helper/charts"
	localConfig "github.com/rancher/observability-e2e/tests/helper/config"
	"github.com/rancher/observability-e2e/tests/helper/utils"
	rancher "github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/cloudcredentials"
	"github.com/rancher/shepherd/extensions/cloudcredentials/aws"
	clusters "github.com/rancher/shepherd/extensions/clusters"
	session "github.com/rancher/shepherd/pkg/session"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var (
	client              *rancher.Client
	sess                *session.Session
	project             *management.Project
	cluster             *clusters.ClusterMeta
	registrySetting     *management.Setting
	err                 error
	s3Client            *resources.S3Client
	BackupRestoreConfig *localConfig.BackupRestoreConfig
	skipS3Tests         bool
	CloudCredentialName string
)

const (
	providerName = "aws"
)

func FailWithReport(message string, callerSkip ...int) {
	// Ensures the correct line numbers are reported
	Fail(message, callerSkip[0]+1)
}

// Run individual or group of tests with labels using CLI
// TEST_LABEL_FILTER=backup-restore  /usr/local/go/bin/go test -timeout 60m github.com/rancher/observability-e2e/tests/backuprestore/functional -v -count=1 -ginkgo.v
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
	RunSpecs(t, "Backup and Restore End-To-End Test Suite", suiteConfig, reporterConfig)
}

// This setup will run once for the entire test suite
var _ = BeforeSuite(func() {
	project = nil
	testSession := session.NewSession()
	sess = testSession
	client, err = rancher.NewClient("", testSession)
	Expect(err).NotTo(HaveOccurred(), "Failed to create Rancher client: %v", err)

	// Get clusterName from config yaml
	clusterName := client.RancherConfig.ClusterName
	Expect(clusterName).NotTo(BeEmpty(), "Cluster name to install is not set")

	// Get cluster meta
	cluster, err = clusters.NewClusterMeta(client, clusterName)
	Expect(err).NotTo(HaveOccurred())

	// Get Server and Registry Setting Values
	registrySetting, err = client.Management.Setting.ByID("system-default-registry")
	Expect(err).NotTo(HaveOccurred())

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

	cloudCredentialConfig := cloudcredentials.LoadCloudCredential(providerName)
	cloudCredential, err := aws.CreateAWSCloudCredentials(client, cloudCredentialConfig)
	Expect(err).NotTo(HaveOccurred())
	CloudCredentialName = strings.Replace(cloudCredential.ID, "/", ":", 1)
	Expect(CloudCredentialName).To(ContainSubstring("cc"))

	// Check if project was found
	if project == nil {
		projectConfig := &management.Project{
			ClusterID: cluster.ID,
			Name:      exampleAppProjectName,
		}

		project, err = client.Management.Project.Create(projectConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(project.Name).To(Equal(exampleAppProjectName))
	}

	// Load configuration from the default YAML file
	BackupRestoreConfig = &localConfig.BackupRestoreConfig{}
	filePath, _ := filepath.Abs(charts.BackupRestoreConfigurationFileKey)
	err = utils.LoadConfigIntoStruct(filePath, BackupRestoreConfig)
	Expect(err).NotTo(HaveOccurred())
	// Giving a dynamic and valid bucket name
	BackupRestoreConfig.S3BucketName = fmt.Sprintf("backup-restore-automation-test-%d", time.Now().Unix())

	// Initialize the S3 client
	if BackupRestoreConfig.AccessKey != "" {
		s3Client, err = resources.NewS3Client(BackupRestoreConfig)
		Expect(err).NotTo(HaveOccurred())

		// Create the S3 bucket from the config
		err = s3Client.CreateBucket(BackupRestoreConfig.S3BucketName, BackupRestoreConfig.S3Region)
		Expect(err).NotTo(HaveOccurred())
		e2e.Logf("S3 bucket '%s' created successfully", BackupRestoreConfig.S3BucketName)
	} else {
		skipS3Tests = true
	}
})

// This teardown will run once after all the tests in the suite are done
var _ = AfterSuite(func() {
	if BackupRestoreConfig.AccessKey != "" {
		// Delete the S3 bucket
		err := s3Client.DeleteBucket(BackupRestoreConfig.S3BucketName)
		Expect(err).NotTo(HaveOccurred())
		e2e.Logf("S3 bucket '%s' deleted successfully", BackupRestoreConfig.S3BucketName)
	}
	// Clean up session
	sess.Cleanup()
})
