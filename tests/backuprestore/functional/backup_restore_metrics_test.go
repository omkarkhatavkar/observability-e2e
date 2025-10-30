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
	localkubectl "github.com/rancher/observability-e2e/tests/helper/kubectl"
	"github.com/rancher/observability-e2e/tests/helper/promclient"
	"github.com/rancher/observability-e2e/tests/helper/utils"
	"github.com/rancher/shepherd/clients/rancher/catalog"
	extencharts "github.com/rancher/shepherd/extensions/charts"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type MetricsParams struct {
	StorageType              string
	BackupOptions            charts.BackupOptions
	BackupFileExtension      string
	Prune                    bool
	CreateCluster            bool
	EncryptionConfigFilePath string
	EnableMonitoring         bool
}

var _ = DescribeTable("Test: Rancher backup and restore metrics tests",
	func(params MetricsParams) {
		if params.StorageType == "s3" && skipS3Tests {
			Skip("Skipping S3 tests as the access key is empty.")
		}
		testName := CurrentSpecReport().LeafNodeText
		testCaseID = int64(charts.ExtractQaseID(testName))

		By("Creating a client session")
		clientWithSession, err := client.WithSession(sess)
		Expect(err).NotTo(HaveOccurred())

		err = charts.SelectResourceSetName(clientWithSession, &params.BackupOptions)
		Expect(err).NotTo(HaveOccurred())

		By("Installing or checking for existing Monitoring Chart with 'rancherBackupMonitoring' enabled")
		initialMonitoringChart, err := extencharts.GetChartStatus(clientWithSession, project.ClusterID, charts.RancherMonitoringNamespace, charts.RancherMonitoringName)
		Expect(err).NotTo(HaveOccurred())

		if initialMonitoringChart.IsAlreadyInstalled {
			e2e.Logf("Monitoring chart is already installed in project: %v", exampleAppProjectName)
		} else {
			latestMonitoringVersion, err := clientWithSession.Catalog.GetLatestChartVersion(charts.RancherMonitoringName, catalog.RancherChartRepo)
			Expect(err).NotTo(HaveOccurred())

			monitoringInstOpts := &charts.InstallOptions{
				Cluster:   cluster,
				Version:   latestMonitoringVersion,
				ProjectID: project.ID,
			}

			monitoringOpts := &charts.RancherMonitoringOpts{
				IngressNginx:      true,
				ControllerManager: true,
				Etcd:              true,
				Proxy:             true,
				Scheduler:         true,
			}
			e2e.Logf("Retrieved latest monitoring chart version: %v", latestMonitoringVersion)

			By("Installing the monitoring chart with the latest version")
			err = charts.InstallRancherMonitoringChart(clientWithSession, monitoringInstOpts, monitoringOpts)
			Expect(err).NotTo(HaveOccurred(), "Failed to install the monitoring chart.")
		}

		By(fmt.Sprintf("Installing/Checking for existing Backup Restore Chart with %s", params.StorageType))
		initialBackupRestoreChart, err := extencharts.GetChartStatus(clientWithSession, project.ClusterID, charts.RancherBackupRestoreNamespace, charts.RancherBackupRestoreName)
		Expect(err).NotTo(HaveOccurred())
		if initialBackupRestoreChart.IsAlreadyInstalled {
			e2e.Logf("Backup and Restore chart is already installed in project: %v", exampleAppProjectName)
		}

		By(fmt.Sprintf("Configuring resources for storage type: %s", params.StorageType))
		secretName, err := charts.CreateStorageResources(params.StorageType, clientWithSession, BackupRestoreConfig)
		Expect(err).NotTo(HaveOccurred())

		By("Creating two users, projects, and role templates")
		userList, projList, roleList, err := resources.CreateRancherResources(clientWithSession, project.ClusterID, "cluster")
		e2e.Logf("Created resources: users=%v, projects=%v, roles=%v", userList, projList, roleList)
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() {
			By("Uninstalling the rancher backup-restore chart")
			err := charts.UninstallBackupRestoreChart(clientWithSession, project.ClusterID, charts.RancherBackupRestoreNamespace)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("Deleting storage resources for: %s", params.StorageType))
			err = charts.DeleteStorageResources(params.StorageType, clientWithSession, BackupRestoreConfig)
			Expect(err).NotTo(HaveOccurred())
		})

		installParams := charts.BackupChartInstallParams{
			StorageType:      params.StorageType,
			SecretName:       secretName,
			BackupConfig:     BackupRestoreConfig,
			ChartVersion:     utils.GetEnvOrDefault("BACKUP_RESTORE_CHART_VERSION", ""),
			EnableMonitoring: true,
		}

		By("Installing the latest backup and restore chart")
		_, err = charts.InstallLatestBackupRestoreChart(
			clientWithSession,
			project,
			cluster,
			&installParams,
		)
		Expect(err).NotTo(HaveOccurred())

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

		By("Creating a Rancher backup and verifying its completion")
		_, filename, err := charts.CreateRancherBackupAndVerifyCompleted(clientWithSession, params.BackupOptions)
		Expect(err).NotTo(HaveOccurred())
		Expect(filename).To(ContainSubstring(params.BackupOptions.Name))
		Expect(filename).To(ContainSubstring(params.BackupFileExtension))

		By(fmt.Sprintf("Creating a restore using backup file: %v", filename))
		restoreTemplate := bv1.NewRestore("", "", charts.SetRestoreObject(params.BackupOptions.Name, params.Prune, params.BackupOptions.EncryptionConfigSecretName))
		restoreTemplate.Spec.BackupFilename = filename

		client, err := client.ReLogin()
		Expect(err).NotTo(HaveOccurred())

		createdRestore, err := client.Steve.SteveType(charts.RestoreSteveType).Create(restoreTemplate)
		Expect(err).NotTo(HaveOccurred())

		restoreObj, err := client.Steve.SteveType(charts.RestoreSteveType).ByID(createdRestore.ID)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying that the restore is completed successfully")
		status, err := charts.VerifyRestoreCompleted(client, charts.RestoreSteveType, restoreObj)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(true))

		By("Create the recurring backup in Rancher Manager")
		// Copy params.BackupOptions into a new object
		recurringBackupOptions := params.BackupOptions
		recurringBackupOptions.Name = namegen.AppendRandomString("recurring-backup")

		// Add schedule for recurring backups (example: every 5 minutes)
		recurringBackupOptions.Schedule = "*/1 * * * *"

		_, _, err = charts.CreateRancherBackupAndVerifyCompleted(clientWithSession, recurringBackupOptions)
		Expect(err).NotTo(HaveOccurred())
		e2e.Logf("Recurring backup completed successfully")

		// create a invalid Backup object using wrong bucket name
		_, err = localkubectl.Execute("apply", "-f", utils.GetYamlPath("tests/helper/yamls/invalidBackupCreate.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to create an invalid backup with bucket name")

		// assert that invalid backup was created
		time.Sleep(2 * time.Minute)
		Eventually(func() string {
			out, err := localkubectl.Execute(
				"get", "backup", "invalid-backup",
				"-o", "jsonpath={.status.conditions[?(@.reason==\"Error\")].message}",
			)
			if err != nil {
				return "" // conditions not ready yet
			}
			return out
		}, 2*time.Minute, 5*time.Second).Should(
			ContainSubstring("failed to check if s3 bucket"),
			"Expected error message for invalid S3 bucket not found",
		)

		By("Creating a Prometheus client for querying backup metrics")
		prometheusAPIPath := "k8s/clusters/local/api/v1/namespaces/cattle-monitoring-system/services/http:rancher-monitoring-prometheus:9090/proxy"
		prometheusURL := fmt.Sprintf("https://%s/%s", clientWithSession.RancherConfig.Host, prometheusAPIPath)
		promClient, err := promclient.NewClient(prometheusURL, clientWithSession.RancherConfig.AdminToken)
		Expect(err).ToNot(HaveOccurred())
		time.Sleep(2 * time.Minute)

		By("Executing Prometheus queries to validate backup and restore metrics")
		queries := map[string]float64{
			`sum(rancher_restore_count)`:                        1.0,
			`sum(rancher_backup_info{status!="Completed"})`:     1.0,
			`sum(rancher_backup_info{backupType!="Recurring"})`: 2.0,
			`sum(rancher_backup_info{status="Completed"})`:      2.0,
		}

		for q, expected := range queries {
			By(fmt.Sprintf("Running Prometheus query: %s", q))
			result, err := promClient.Query(q)
			Expect(err).ToNot(HaveOccurred())
			e2e.Logf("Prometheus query result for %s: %+v", q, result)
			if len(*result) == 0 {
				Fail(fmt.Sprintf("Prometheus query returned no results: %s", q))
			} else {
				Expect(float64((*result)[0].Value)).To(Equal(expected),
					fmt.Sprintf("unexpected value for query: %s", q))
			}
		}
	},

	charts.QaseEntry("[QASE-8273] (without encryption)",
		[]interface{}{Label("LEVEL0", "metrics", "s3", "backup-restore")},
		MetricsParams{
			StorageType: "s3",
			BackupOptions: charts.BackupOptions{
				Name:            namegen.AppendRandomString("backup"),
				ResourceSetName: "rancher-resource-set",
				RetentionCount:  10,
			},
			BackupFileExtension:      ".tar.gz",
			Prune:                    true,
			EncryptionConfigFilePath: charts.EncryptionConfigFilePath,
			EnableMonitoring:         true,
		},
	),
)
