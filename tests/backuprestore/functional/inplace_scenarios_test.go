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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	bv1 "github.com/rancher/backup-restore-operator/pkg/apis/resources.cattle.io/v1"
	resources "github.com/rancher/observability-e2e/resources/rancher"
	"github.com/rancher/observability-e2e/tests/helper/charts"
	"github.com/rancher/observability-e2e/tests/helper/utils"
	rancher "github.com/rancher/shepherd/clients/rancher"
	catalog "github.com/rancher/shepherd/clients/rancher/catalog"
	extencharts "github.com/rancher/shepherd/extensions/charts"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type InplaceParams struct {
	StorageType              string
	BackupOptions            charts.BackupOptions
	BackupFileExtension      string
	ProvisioningInput        charts.ProvisioningConfig
	Prune                    bool
	CreateCluster            bool
	EncryptionConfigFilePath string
}

var clusterName string

var _ = DescribeTable("Test: Rancher inplace backup and restore test.",
	func(params InplaceParams) {
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

		By("Creating two users, projects, and role templates...")
		userList, projList, roleList, err := resources.CreateRancherResources(clientWithSession, project.ClusterID, "cluster")
		e2e.Logf("%v, %v, %v", userList, projList, roleList)
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() {
			By("Delete the downstream clusters as part of cleanup")
			err = resources.DeleteCluster(client, clusterName)
			Expect(err).NotTo(HaveOccurred())
		})

		if params.CreateCluster == true {
			By("Provisioning a downstream RKE2 cluster...")
			clusterName, err = resources.CreateRKE2Cluster(clientWithSession, CloudCredentialName)
			Expect(err).NotTo(HaveOccurred())
		}
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
		By("Check if the backup needs to be encrypted, if yes create the encryptionconfig secret")
		if params.BackupOptions.EncryptionConfigSecretName != "" {
			By("Delete any existing encryption config")
			existingSecret, err := client.Steve.SteveType("secret").ByID("cattle-resources-system/encryptionconfig")
			if err == nil {
				err = client.Steve.SteveType("secret").Delete(existingSecret)
				Expect(err).NotTo(HaveOccurred())
			}
			secretName, err = charts.CreateEncryptionConfigSecret(client.Steve, params.EncryptionConfigFilePath,
				params.BackupOptions.EncryptionConfigSecretName, charts.RancherBackupRestoreNamespace)
			if err != nil {
				e2e.Logf("Error applying encryption config: %v", err)
			}
			e2e.Logf("Successfully created encryption config secret: %s", secretName)
		}

		_, filename, err := charts.CreateRancherBackupAndVerifyCompleted(clientWithSession, params.BackupOptions)
		Expect(err).NotTo(HaveOccurred())
		Expect(filename).To(ContainSubstring(params.BackupOptions.Name))
		Expect(filename).To(ContainSubstring(params.BackupFileExtension))

		By("Validating backup file is present in AWS S3...")
		s3Location := BackupRestoreConfig.S3BucketName + "/" + BackupRestoreConfig.S3FolderName
		result, err := s3Client.FileExistsInBucket(s3Location, filename)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(true))

		By("Creating two more users, projects, and role templates...")
		userListPostBackup, projListPostBackup, roleListPostBackup, err := resources.CreateRancherResources(clientWithSession, project.ClusterID, "cluster")
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("Creating a restore using backup file: %v", filename))
		restoreTemplate := bv1.NewRestore("", "", charts.SetRestoreObject(params.BackupOptions.Name, params.Prune, params.BackupOptions.EncryptionConfigSecretName))
		restoreTemplate.Spec.BackupFilename = filename
		client, err = client.ReLogin()
		Expect(err).NotTo(HaveOccurred())

		createdRestore, err := client.Steve.SteveType(charts.RestoreSteveType).Create(restoreTemplate)
		Expect(err).NotTo(HaveOccurred())

		restoreObj, err := client.Steve.SteveType(charts.RestoreSteveType).ByID(createdRestore.ID)
		Expect(err).NotTo(HaveOccurred())

		By("Verify that restore is completed and successfully.")
		status, err := charts.VerifyRestoreCompleted(client, charts.RestoreSteveType, restoreObj)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(true))

		By(("Validating Rancher resource still exists"))
		err = charts.VerifyRancherResources(client, userList, projList, roleList)
		Expect(err).NotTo(HaveOccurred())

		By("Validate that rancher resources create after the backup are also exists (works if Prune is false) ")
		err = charts.VerifyRancherResources(client, userListPostBackup, projListPostBackup, roleListPostBackup)
		if params.Prune == true {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).NotTo(HaveOccurred())
		}
		if params.CreateCluster == true {
			By("Validating downstream clusters are in an Active status...")
			err = resources.VerifyCluster(client, clusterName)
			Expect(err).NotTo(HaveOccurred())
		}
	},

	// **Test Case: Rancher inplace backup and restore test scenarios
	Entry("(without encryption)", Label("LEVEL0", "backup-restore", "s3", "inplace"), InplaceParams{
		StorageType: "s3",
		BackupOptions: charts.BackupOptions{
			Name:            namegen.AppendRandomString("backup"),
			ResourceSetName: "rancher-resource-set",
			RetentionCount:  10,
		},
		BackupFileExtension: ".tar.gz",
		ProvisioningInput: charts.ProvisioningConfig{
			RKE2KubernetesVersions: []string{utils.GetEnvOrDefault("RKE2_VERSION", "v1.31.5+rke2r1")},
			Providers:              []string{"aws"},
			NodeProviders:          []string{"ec2"},
			CNIs:                   []string{"calico"},
		},
		Prune:                    true,
		CreateCluster:            true,
		EncryptionConfigFilePath: charts.EncryptionConfigFilePath,
	}),

	Entry("(with encryption)", Label("LEVEL0", "backup-restore", "s3", "inplace"), InplaceParams{
		StorageType: "s3",
		BackupOptions: charts.BackupOptions{
			Name:                       namegen.AppendRandomString("backup"),
			RetentionCount:             10,
			EncryptionConfigSecretName: "encryptionconfig",
		},
		BackupFileExtension: ".tar.gz.enc",
		ProvisioningInput: charts.ProvisioningConfig{
			RKE2KubernetesVersions: []string{utils.GetEnvOrDefault("RKE2_VERSION", "v1.31.5+rke2r1")},
			Providers:              []string{"aws"},
			NodeProviders:          []string{"ec2"},
			CNIs:                   []string{"calico"},
		},
		Prune:                    true,
		CreateCluster:            true,
		EncryptionConfigFilePath: charts.EncryptionConfigFilePath,
	}),

	Entry("(with Prune is set as false)", Label("LEVEL1", "backup-restore", "s3", "prune"), InplaceParams{
		StorageType: "s3",
		BackupOptions: charts.BackupOptions{
			Name:                       namegen.AppendRandomString("backup"),
			RetentionCount:             10,
			EncryptionConfigSecretName: "encryptionconfig",
		},
		BackupFileExtension: ".tar.gz.enc",
		ProvisioningInput: charts.ProvisioningConfig{
			RKE2KubernetesVersions: []string{utils.GetEnvOrDefault("RKE2_VERSION", "v1.31.5+rke2r1")},
			Providers:              []string{"aws"},
			NodeProviders:          []string{"ec2"},
			CNIs:                   []string{"calico"},
		},
		Prune:                    false,
		CreateCluster:            false,
		EncryptionConfigFilePath: charts.EncryptionConfigFilePath,
	}),

	Entry("(with encryption config having asterisk *)", Label("LEVEL1", "backup-restore", "s3", "encryption-config-asterisk"), InplaceParams{
		StorageType: "s3",
		BackupOptions: charts.BackupOptions{
			Name:                       namegen.AppendRandomString("backup"),
			RetentionCount:             10,
			EncryptionConfigSecretName: "encryptionconfig",
		},
		BackupFileExtension: ".tar.gz.enc",
		ProvisioningInput: charts.ProvisioningConfig{
			RKE2KubernetesVersions: []string{utils.GetEnvOrDefault("RKE2_VERSION", "v1.31.5+rke2r1")},
			Providers:              []string{"aws"},
			NodeProviders:          []string{"ec2"},
			CNIs:                   []string{"calico"},
		},
		Prune:                    true,
		CreateCluster:            false,
		EncryptionConfigFilePath: charts.EncryptionConfigAsteriskFilePath,
	}),
)
