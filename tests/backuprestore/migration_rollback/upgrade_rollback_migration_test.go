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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	resources "github.com/rancher/observability-e2e/resources/rancher"
	"github.com/rancher/observability-e2e/tests/helper/charts"
	"github.com/rancher/observability-e2e/tests/helper/helm"
	localkubectl "github.com/rancher/observability-e2e/tests/helper/kubectl"
	"github.com/rancher/observability-e2e/tests/helper/utils"
	"github.com/rancher/rancher/tests/v2/actions/pipeline"
	"github.com/rancher/shepherd/clients/rancher"
	extencharts "github.com/rancher/shepherd/extensions/charts"
	"github.com/rancher/shepherd/pkg/config"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type UpgradeRollbackMigrationParams struct {
	StorageType              string
	BackupOptions            charts.BackupOptions
	BackupFileExtension      string
	ProvisioningInput        charts.ProvisioningConfig
	Prune                    bool
	CreateCluster            bool
	EncryptionConfigFilePath string
}

var clusterNameRollbackMigration string
var rollbackChartVersion string

var _ = DescribeTable("Test: Validate the Backup and Restore Upgrade and Rollback Scenario from RKE2 to RKE2",
	func(params UpgradeRollbackMigrationParams) {
		By("Checking that the Terraform context is valid")
		Expect(tfCtx).ToNot(BeNil())
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

		utils.SafeCleanup("Deleting the downstream clusters as part of cleanup", func() {
			err := resources.DeleteCluster(client, clusterNameMigration)
			Expect(err).NotTo(HaveOccurred())
		})
		if params.CreateCluster == true {
			By("Provisioning a downstream RKE2 cluster...")
			clusterNameRollbackMigration, err = resources.CreateRKE2Cluster(clientWithSession, CloudCredentialName)
			Expect(err).NotTo(HaveOccurred())
		}

		utils.SafeCleanup(fmt.Sprintf("Deleting resources used for storage type: %s", params.StorageType), func() {
			err := charts.DeleteStorageResources(params.StorageType, clientWithSession, BackupRestoreConfig)
			Expect(err).NotTo(HaveOccurred())
		})

		// Get the latest version of the backup restore chart
		if !initialBackupRestoreChart.IsAlreadyInstalled {
			rollbackChartVersion, err = charts.InstallLatestBackupRestoreChart(
				clientWithSession,
				project,
				cluster,
				params.StorageType,
				secretName,
				BackupRestoreConfig,
				"",
			)
			Expect(err).NotTo(HaveOccurred())
			e2e.Logf("Installed Backup and Restore Chart Version: %s", rollbackChartVersion)
		}
		By("Check if the backup needs to be encrypted, if yes create the encryptionconfig secret")
		if params.BackupOptions.EncryptionConfigSecretName != "" {
			secretName, err = charts.CreateEncryptionConfigSecret(client.Steve, params.EncryptionConfigFilePath,
				params.BackupOptions.EncryptionConfigSecretName, charts.RancherBackupRestoreNamespace)
			if err != nil {
				e2e.Failf("Error applying encryption config: %v", err)
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

		// As we have the backup now we should upgrade the cluster and then again rollback
		By("As backup is present we can upgrading the instance now... ")

		upgradeToRancherRepoURL := os.Getenv("UPGRADE_RANCHER_REPO_URL")
		upgradeRancherVersion := os.Getenv("UPGRADE_RANCHER_VERSION")
		rancherVersion := tfCtx.Options.Vars["rancher_version"].(string)
		By(fmt.Sprintf("It will upgrade from %s to %s ", rancherVersion, upgradeRancherVersion))

		password := os.Getenv("RANCHER_PASSWORD")
		err = resources.UpgradeRancher("", upgradeToRancherRepoURL, upgradeRancherVersion, clientWithSession.RancherConfig.Host, password)
		Expect(err).NotTo(HaveOccurred(), "Failed to upgrade the Rancher")

		By("Wait see the rancher is been upgraded and working condition")
		rancherConfig := new(rancher.Config)
		config.LoadConfig(rancher.ConfigurationFileKey, rancherConfig)
		token, err := pipeline.CreateAdminToken(password, rancherConfig)
		Expect(err).To(BeNil())
		rancherConfig.AdminToken = token
		config.UpdateConfig(rancher.ConfigurationFileKey, rancherConfig)

		By("Verify that the correct version of rancher is showing up")
		afterUpgradeRancherVersion, err := localkubectl.Execute(
			"get", "deploy", "rancher",
			"-n", "cattle-system",
			"-o", "jsonpath={.spec.template.spec.containers[0].image}",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(afterUpgradeRancherVersion).To(ContainSubstring(upgradeRancherVersion))

		By("Verify that the downstream clusters are showing up correctly")
		err = resources.VerifyCluster(clientWithSession, clusterNameRollbackMigration)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Downstream Cluster %s is not getting Active.", clusterNameRollbackMigration))

		By("Update the rancher to use the latest backup and restore chart")
		time.Sleep(3 * time.Minute)
		_, err = charts.InstallLatestBackupRestoreChart(
			clientWithSession,
			project,
			cluster,
			params.StorageType,
			secretName,
			BackupRestoreConfig,
			utils.GetEnvOrDefault("BACKUP_RESTORE_CHART_VERSION", ""),
		)
		Expect(err).NotTo(HaveOccurred())

		By("As backup is present we can remove/clean the instance for migration using ")
		_, err = tfCtx.DestroyTarget("module.ec2.aws_instance.rke2_node")
		if err != nil {
			e2e.Failf("Remove rke2_node destroy failed:")
		}
		By("Old server is destroyed, will spin up new machine and start restoring the backup")
		tfCtx.Options.Vars["install_rancher"] = false
		_, err = tfCtx.InitAndApply()
		Expect(err).ToNot(HaveOccurred(), "Failed to spinup the new machine")

		By(fmt.Sprintf("Configuring/Creating required resources for the storage type: %s testing", params.StorageType))
		_, err = localkubectl.Execute(
			"create", "secret", "generic", "s3-creds",
			"--from-literal=accessKey="+CredentialConfig.AccessKey,
			"--from-literal=secretKey="+CredentialConfig.SecretKey,
		)
		Expect(err).NotTo(HaveOccurred(), "Failed to create secret for backup and restore")

		By("Create the cattle-system namespace")
		createNamespace := []string{"create", "namespace", "cattle-system"}
		_, err = localkubectl.Execute(createNamespace...)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		// Todo Add the way to fetch the rancher version pass to install it
		By("Checkout the charts repo based on the rancher upstream version ")
		branch := "dev-" + strings.Join(strings.Split(upgradeRancherVersion, ".")[:2], ".")
		if !strings.HasPrefix(branch, "dev-v") {
			branch = strings.Replace(branch, "dev-", "dev-v", 1)
		}
		chartDir, err := charts.DownloadAndExtractRancherCharts(branch)
		Expect(err).NotTo(HaveOccurred(), "Failed to download and extract repo")
		e2e.Logf("Extracted charts directory: %s\n", chartDir)

		By("install the rancher-backup-crd")
		rancherBackupCrdPath := filepath.Join(chartDir, "charts", "rancher-backup-crd")
		err = helm.InstallChartFromPath("rancher-backup-crd", rancherBackupCrdPath, rollbackChartVersion, charts.RancherBackupRestoreNamespace)
		Expect(err).NotTo(HaveOccurred(), "Failed to install the rancher-backup-crd")

		By("install the rancher-backup")
		rancherBackupPath := filepath.Join(chartDir, "charts", "rancher-backup")
		err = helm.InstallChartFromPath("rancher-backup", rancherBackupPath, rollbackChartVersion, charts.RancherBackupRestoreNamespace)
		Expect(err).NotTo(HaveOccurred(), "Failed to install the rancher-backup-crd")

		_, err = helm.Execute("", "list", "-n", "cattle-resources-system")
		Expect(err).NotTo(HaveOccurred(), "rancher-backup and rancher-backup-crd are deployed")

		By("Create the encryption config")
		encryptionconfigFilePath := utils.GetYamlPath("tests/helper/yamls/encryption-provider-config.yaml")
		_, err = localkubectl.Execute(
			"create", "secret", "generic", "encryptionconfig",
			"--from-file="+encryptionconfigFilePath,
			"-n", "cattle-resources-system",
		)
		Expect(err).NotTo(HaveOccurred(), "Failed to create the encryptionconfig")

		By("create the restore-migation yaml and apply it")
		migrationYamlData := charts.MigrationYamlData{
			BackupFilename: filename,
			BucketName:     BackupRestoreConfig.S3BucketName,
			Folder:         BackupRestoreConfig.S3FolderName,
			Region:         BackupRestoreConfig.S3Region,
			Endpoint:       BackupRestoreConfig.S3Endpoint,
		}
		err = utils.GenerateYAMLFromTemplate(
			utils.GetYamlPath("tests/helper/yamls/restore-migration.template.yaml"),
			"restore-migration.yaml",
			migrationYamlData,
		)
		Expect(err).NotTo(HaveOccurred(), "Failed to create the backup restore file")

		_, err = localkubectl.Execute("apply", "-f", "restore-migration.yaml")
		Expect(err).NotTo(HaveOccurred(), "Failed to apply the Restore Migration Process")
		e2e.Logf("Waiting for 3 minutes to see backups appear...")
		time.Sleep(3 * time.Minute)

		// TODO : There has been active issue here https://github.com/rancher/rancher/issues/50638
		output, err := localkubectl.Execute("get", "restore")
		Expect(err).NotTo(HaveOccurred(), "Failed restore the backup")
		Expect(string(output)).To(ContainSubstring("Completed"), "Restore not completed")

		rancherRepoURL := tfCtx.Options.Vars["rancher_repo_url"].(string)
		password = os.Getenv("RANCHER_PASSWORD")

		By("Now Install the rancher as the restore is been successful")
		err = resources.InstallRancher("", rancherRepoURL, rancherVersion, clientWithSession.RancherConfig.Host, password)
		Expect(err).NotTo(HaveOccurred(), "Failed to install the rancher after the restore")

		rancherConfig = new(rancher.Config)
		config.LoadConfig(rancher.ConfigurationFileKey, rancherConfig)
		token, err = pipeline.CreateAdminToken(os.Getenv("RANCHER_PASSWORD"), rancherConfig)
		Expect(err).To(BeNil())
		rancherConfig.AdminToken = token
		config.UpdateConfig(rancher.ConfigurationFileKey, rancherConfig)

		By("Veriy that the downstream clusters are showing up correctly")
		err = resources.VerifyCluster(clientWithSession, clusterNameRollbackMigration)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Downstream Cluster %s is not getting Active.", clusterNameRollbackMigration))
	},

	// **Test Case: Rancher inplace backup and restore test scenarios
	Entry("(with encryption)", Label("LEVEL0", "backup-restore", "upgrade_rollback_migration"), UpgradeRollbackMigrationParams{
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
		CreateCluster:            true,
		EncryptionConfigFilePath: charts.EncryptionConfigFilePath,
	}),
)
