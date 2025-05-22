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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	resources "github.com/rancher/observability-e2e/resources/rancher"
	"github.com/rancher/observability-e2e/tests/helper/charts"
	"github.com/rancher/observability-e2e/tests/helper/utils"
	rancher "github.com/rancher/shepherd/clients/rancher"
	catalog "github.com/rancher/shepherd/clients/rancher/catalog"
	extcharts "github.com/rancher/shepherd/extensions/charts"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = DescribeTable("BackupTests: ",
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
		initialBackupRestoreChart, err := extcharts.GetChartStatus(clientWithSession, project.ClusterID, charts.RancherBackupRestoreNamespace, charts.RancherBackupRestoreName)
		Expect(err).NotTo(HaveOccurred())

		e2e.Logf("Checking if the backup and restore chart is already installed")
		if initialBackupRestoreChart.IsAlreadyInstalled {
			e2e.Logf("Backup and Restore chart is already installed in project: %v", exampleAppProjectName)
		}

		By(fmt.Sprintf("Configuring/Creating required resources for the storage type: %s testing", params.StorageType))
		secretName, err := charts.CreateStorageResources(params.StorageType, clientWithSession, BackupRestoreConfig)
		Expect(err).NotTo(HaveOccurred())

		By("Creating two users, projects, and role templates...")
		userList, projList, roleList, err := resources.CreateRancherResources(clientWithSession, project.ClusterID, "cluster")
		e2e.Logf("%v, %v, %v", userList, projList, roleList)
		Expect(err).NotTo(HaveOccurred())

		// Ensure chart uninstall runs at the end of the test
		DeferCleanup(func() {
			By("Uninstalling the rancher backup-restore chart")
			err := charts.UninstallBackupRestoreChart(clientWithSession, project.ClusterID, charts.RancherBackupRestoreNamespace)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("Deleting required resources used for the storage type: %s testing", params.StorageType))
			err = charts.DeleteStorageResources(params.StorageType, clientWithSession, BackupRestoreConfig)
			Expect(err).NotTo(HaveOccurred())
		})

		// Get the latest version of the backup restore chart
		if !initialBackupRestoreChart.IsAlreadyInstalled {
			latestBackupRestoreVersion, err := clientWithSession.Catalog.GetLatestChartVersion(charts.RancherBackupRestoreName, catalog.RancherChartRepo)
			Expect(err).NotTo(HaveOccurred())
			e2e.Logf("Retrieved latest backup-restore chart version to install: %v", latestBackupRestoreVersion)
			latestBackupRestoreVersion = utils.GetEnvOrDefault("BACKUP_RESTORE_CHART_VERSION", latestBackupRestoreVersion)
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
				err = extcharts.WatchAndWaitDeployments(clientWithSession, project.ClusterID, charts.RancherBackupRestoreNamespace, metav1.ListOptions{})
				errDeployChan <- err
			}()

			select {
			case err := <-errDeployChan:
				Expect(err).NotTo(HaveOccurred())
			case <-time.After(2 * time.Minute):
				e2e.Failf("Timeout waiting for WatchAndWaitDeployments to complete")
			}
		}
		By("Check if the backup needs to be encrypted, if yes create the encryptionconfig secret")
		if params.BackupOptions.EncryptionConfigSecretName != "" {
			secretName, err = charts.CreateEncryptionConfigSecret(client.Steve, charts.EncryptionConfigFilePath,
				params.BackupOptions.EncryptionConfigSecretName, charts.RancherBackupRestoreNamespace)
			if err != nil {
				e2e.Logf("Error applying encryption config: %v", err)
			}
			e2e.Logf("Successfully created encryption config secret: %s", secretName)
		}
		By("Creating the rancher backup")
		backupObject, filename, err := charts.CreateRancherBackupAndVerifyCompleted(clientWithSession, params.BackupOptions)
		Expect(err).NotTo(HaveOccurred())
		Expect(filename).To(ContainSubstring(params.BackupOptions.Name))
		Expect(filename).To(ContainSubstring(params.BackupFileExtension))

		By("Validating backup file is present in AWS S3...")
		s3Location := BackupRestoreConfig.S3BucketName + "/" + BackupRestoreConfig.S3FolderName
		result, err := s3Client.FileExistsInBucket(s3Location, filename)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(true))

		By("Validate that there are 3 backups in the location after 5 mins")
		duration := 5 * time.Minute
		e2e.Logf("Waiting for 5 minutes to see backups appear...")
		time.Sleep(duration)

		resultList, err := s3Client.ListFilesAndTimeDifference(BackupRestoreConfig.S3BucketName, BackupRestoreConfig.S3FolderName)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(resultList)).To(Equal(3))
		client, err := client.ReLogin()
		Expect(err).NotTo(HaveOccurred())

		By("Deleting the Backup from the Rancher Manager")
		err = client.Steve.SteveType(charts.BackupSteveType).Delete(backupObject)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying the Backup entry has been deleted from Rancher Manager")
		_, err = client.Steve.SteveType(charts.BackupSteveType).ByID(backupObject.ID)
		Expect(err).To(HaveOccurred())
	},

	Entry("Test Rancher Backup retention with scheduled functionality", Label("LEVEL1", "only_backup", "s3"), charts.BackupParams{
		StorageType: "s3",
		BackupOptions: charts.BackupOptions{
			Name:           namegen.AppendRandomString("backup"),
			RetentionCount: 3,
			Schedule:       "* * * * *",
		},
		BackupFileExtension: ".tar.gz",
		Prune:               true,
	}),
)

var _ = DescribeTable("Backup Resource Set Tests : ",
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

		rancherVersion, err := utils.GetRancherVersion(clientWithSession)
		Expect(err).NotTo(HaveOccurred())
		ok, err := charts.IsVersionAtLeast(rancherVersion, 2, 11)
		Expect(err).NotTo(HaveOccurred())
		if !ok {
			Skip("Skipping test as this needs rancher 2.11 or above")
		}

		By(fmt.Sprintf("Installing Backup Restore Chart with %s", params.StorageType))

		// Check if the chart is already installed
		initialBackupRestoreChart, err := extcharts.GetChartStatus(clientWithSession, project.ClusterID, charts.RancherBackupRestoreNamespace, charts.RancherBackupRestoreName)
		Expect(err).NotTo(HaveOccurred())

		e2e.Logf("Checking if the backup and restore chart is already installed")
		if initialBackupRestoreChart.IsAlreadyInstalled {
			e2e.Logf("Backup and Restore chart is already installed in project: %v", exampleAppProjectName)
		}

		By(fmt.Sprintf("Configuring/Creating required resources for the storage type: %s testing", params.StorageType))
		secretName, err := charts.CreateStorageResources(params.StorageType, clientWithSession, BackupRestoreConfig)
		Expect(err).NotTo(HaveOccurred())

		By("Creating two users, projects, and role templates...")
		userList, projList, roleList, err := resources.CreateRancherResources(clientWithSession, project.ClusterID, "cluster")
		e2e.Logf("%v, %v, %v", userList, projList, roleList)
		Expect(err).NotTo(HaveOccurred())

		// Ensure chart uninstall runs at the end of the test
		DeferCleanup(func() {
			By("Uninstalling the rancher backup-restore chart")
			err := charts.UninstallBackupRestoreChart(clientWithSession, project.ClusterID, charts.RancherBackupRestoreNamespace)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("Deleting required resources used for the storage type: %s testing", params.StorageType))
			err = charts.DeleteStorageResources(params.StorageType, clientWithSession, BackupRestoreConfig)
			Expect(err).NotTo(HaveOccurred())
		})

		// Get the latest version of the backup restore chart
		if !initialBackupRestoreChart.IsAlreadyInstalled {
			latestBackupRestoreVersion, err := clientWithSession.Catalog.GetLatestChartVersion(charts.RancherBackupRestoreName, catalog.RancherChartRepo)
			Expect(err).NotTo(HaveOccurred())
			e2e.Logf("Retrieved latest backup-restore chart version to install: %v", latestBackupRestoreVersion)
			latestBackupRestoreVersion = utils.GetEnvOrDefault("BACKUP_RESTORE_CHART_VERSION", latestBackupRestoreVersion)
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
				err = extcharts.WatchAndWaitDeployments(clientWithSession, project.ClusterID, charts.RancherBackupRestoreNamespace, metav1.ListOptions{})
				errDeployChan <- err
			}()

			select {
			case err := <-errDeployChan:
				Expect(err).NotTo(HaveOccurred())
			case <-time.After(2 * time.Minute):
				e2e.Failf("Timeout waiting for WatchAndWaitDeployments to complete")
			}
		}
		By("Check if the backup needs to be encrypted, if yes create the encryptionconfig secret")
		if params.BackupOptions.EncryptionConfigSecretName != "" {
			secretName, err = charts.CreateEncryptionConfigSecret(client.Steve, charts.EncryptionConfigFilePath,
				params.BackupOptions.EncryptionConfigSecretName, charts.RancherBackupRestoreNamespace)
			if err != nil {
				e2e.Logf("Error applying encryption config: %v", err)
			}
			e2e.Logf("Successfully created encryption config secret: %s", secretName)
		}
		By("Creating the rancher backup")
		_, filename, err := charts.CreateRancherBackupAndVerifyCompleted(clientWithSession, params.BackupOptions)
		Expect(err).NotTo(HaveOccurred())
		Expect(filename).To(ContainSubstring(params.BackupOptions.Name))
		Expect(filename).To(ContainSubstring(params.BackupFileExtension))

		By("Validating backup file is present in AWS S3...")
		s3Location := BackupRestoreConfig.S3BucketName + "/" + BackupRestoreConfig.S3FolderName
		result, err := s3Client.FileExistsInBucket(s3Location, filename)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(true))

		By("Download the backup file to local machine")
		tmpPath := filepath.Join(os.TempDir(), filename)
		err = s3Client.DownloadFile(s3Location, filename, tmpPath)
		Expect(err).NotTo(HaveOccurred())

		By("Unzip the backup file on local machine in tmp directory")
		dir, err := utils.CreateTempDir(strings.TrimSuffix(filename, ".tar.gz"))
		if err != nil {
			panic(err)
		}
		Expect(err).NotTo(HaveOccurred())

		defer func() {
			if !CurrentSpecReport().Failed() {
				_ = os.RemoveAll(dir)
				_ = os.Remove(tmpPath)
			}
		}()
		err = utils.ExtractTarGz(tmpPath, dir)
		if err != nil {
			e2e.Logf("Failed to extract: %v", err)
		}
		Expect(err).NotTo(HaveOccurred())

		By("Validate does the backup have the secrets in case of full and not in basic resource-set")
		err = charts.ValidateBackupFile(dir)
		if err != nil {
			e2e.Logf("Assert Error: Failed to validate: %v", err)
		}
		if params.SecretsExists {
			Expect(err).NotTo(HaveOccurred())
		} else {
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("no secrets.#v1 directory found")))
		}
	},

	Entry("Test Rancher Backup with Basic Resource Set (should not backup secrets)", Label("LEVEL1", "resource-set", "basic"), charts.BackupParams{
		StorageType: "s3",
		BackupOptions: charts.BackupOptions{
			Name:            namegen.AppendRandomString("backup"),
			ResourceSetName: "rancher-resource-set-basic",
			RetentionCount:  3,
			Schedule:        "* * * * *",
		},
		BackupFileExtension: ".tar.gz",
		Prune:               true,
		SecretsExists:       false,
	}),

	Entry("Test Rancher Backup with Full Resource Set (should backup secrets)", Label("LEVEL1", "resource-set", "full"), charts.BackupParams{
		StorageType: "s3",
		BackupOptions: charts.BackupOptions{
			Name:            namegen.AppendRandomString("backup"),
			ResourceSetName: "rancher-resource-set-full",
			RetentionCount:  3,
			Schedule:        "* * * * *",
		},
		BackupFileExtension: ".tar.gz",
		Prune:               true,
		SecretsExists:       true,
	}),
)
