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
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/observability-e2e/tests/helper/charts"
	rancher "github.com/rancher/shepherd/clients/rancher"
	catalog "github.com/rancher/shepherd/clients/rancher/catalog"
	extencharts "github.com/rancher/shepherd/extensions/charts"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const exampleAppProjectName = "System"

var _ = Describe("Parameterized Backup and Restore Chart Installation Tests", func() {
	var _ = DescribeTable("Test: Backup and Restore Chart Installation with multiple storage",
		func(params charts.BackupParams) {
			if params.StorageType == "s3" && skipS3Tests {
				Skip("Skipping S3 tests as the access key is empty.")
			}
			var (
				clientWithSession *rancher.Client
				err               error
			)

			By("Creating a client session")
			clientWithSession, err = client.WithSession(sess)
			Expect(err).NotTo(HaveOccurred())

			err = charts.SelectResourceSetName(clientWithSession, &params.BackupOptions)
			Expect(err).NotTo(HaveOccurred())
			By(fmt.Sprintf("Installing Backup Restore Chart with %s", params.StorageType))

			// Check if the chart is already installed
			initialBackupRestoreChart, err := extencharts.GetChartStatus(clientWithSession, project.ClusterID, charts.RancherBackupRestoreNamespace, charts.RancherBackupRestoreName)
			Expect(err).NotTo(HaveOccurred())

			e2e.Logf("Checking if the backup and restore chart is already installed")
			if initialBackupRestoreChart.IsAlreadyInstalled {
				e2e.Logf("Backup and Restore chart is already installed in project: %v", exampleAppProjectName)
			}

			By(fmt.Sprintf("Configuring/Creating required resources for the storage type: %s testing", params.StorageType))
			secretName, err := charts.CreateStorageResources(params.StorageType, clientWithSession, BackupRestoreConfig)
			Expect(err).NotTo(HaveOccurred())

			// Get the latest version of the backup restore chart
			if !initialBackupRestoreChart.IsAlreadyInstalled {
				latestBackupRestoreVersion, err := clientWithSession.Catalog.GetLatestChartVersion(charts.RancherBackupRestoreName, catalog.RancherChartRepo)
				Expect(err).NotTo(HaveOccurred())
				e2e.Logf("Retrieved latest backup-restore chart version to install: %v", latestBackupRestoreVersion)

				backuprestoreInstOpts := &charts.InstallOptions{
					Cluster:   cluster,
					Version:   latestBackupRestoreVersion,
					ProjectID: project.ID,
				}

				backuprestoreOpts := &charts.RancherBackupRestoreOpts{
					VolumeName:                BackupRestoreConfig.VolumeName,
					StorageClassName:          BackupRestoreConfig.StorageClassName,
					BucketName:                BackupRestoreConfig.S3BucketName,
					CredentialSecretName:      secretName,
					CredentialSecretNamespace: BackupRestoreConfig.CredentialSecretNamespace,
					Enabled:                   true,
					Endpoint:                  BackupRestoreConfig.S3Endpoint,
					Folder:                    BackupRestoreConfig.S3FolderName,
					Region:                    BackupRestoreConfig.S3Region,
				}

				By(fmt.Sprintf("Installing the version %s for the backup restore", latestBackupRestoreVersion))
				err = charts.InstallRancherBackupRestoreChart(clientWithSession, backuprestoreInstOpts, backuprestoreOpts, true, params.StorageType)
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for backup-restore chart deployments to have expected replicas")
				errDeployChan := make(chan error, 1)
				go func() {
					err = extencharts.WatchAndWaitDeployments(clientWithSession, project.ClusterID, charts.RancherBackupRestoreNamespace, metav1.ListOptions{})
					errDeployChan <- err
				}()

				select {
				case err := <-errDeployChan:
					Expect(err).NotTo(HaveOccurred())
				case <-time.After(2 * time.Minute):
					e2e.Failf("Timeout waiting for WatchAndWaitDeployments to complete")
				}
			}

			_, filename, err := charts.CreateRancherBackupAndVerifyCompleted(clientWithSession, params.BackupOptions)
			Expect(err).NotTo(HaveOccurred())
			Expect(filename).To(ContainSubstring(params.BackupOptions.Name))
			Expect(filename).To(ContainSubstring(".tar.gz"))

			By("Uninstalling the rancher backup-restore chart")
			err = charts.UninstallBackupRestoreChart(clientWithSession, project.ClusterID, charts.RancherBackupRestoreNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Verify the uninstalling charts removed the backup and restore objects")
			err = charts.WaitForDeploymentsCleanup(client, project.ClusterID, charts.RancherBackupRestoreNamespace)
			if err != nil {
				log.Fatalf("Cleanup check failed: %v", err)
			}

			By(fmt.Sprintf("Deleting required resources used for the storage type: %s testing", params.StorageType))
			err = charts.DeleteStorageResources(params.StorageType, clientWithSession, BackupRestoreConfig)
			Expect(err).NotTo(HaveOccurred())

		},

		// **Test Case: Install with S3 Storage**
		Entry("Install and Uninstall Backup Restore Chart with S3 Storage", Label("LEVEL0", "backup-restore", "s3", "installation"), charts.BackupParams{
			StorageType: "s3",
			BackupOptions: charts.BackupOptions{
				Name:           namegen.AppendRandomString("backup"),
				RetentionCount: 10,
			},
		}),

		// **Test Case: Install with local Storage**
		Entry("Install and Uninstall Backup Restore Chart with Local Storage Class", Label("LEVEL0", "backup-restore", "local", "installation"), charts.BackupParams{
			StorageType: "storageClass",
			BackupOptions: charts.BackupOptions{
				Name:           namegen.AppendRandomString("backup"),
				RetentionCount: 10,
			},
		}),
	)

})
