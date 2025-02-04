package charts

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rancher/shepherd/clients/rancher"
	"github.com/rancher/shepherd/extensions/clusters"

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/shepherd/pkg/api/steve/catalog/types"
	"github.com/rancher/shepherd/pkg/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	// defaultRegistrySettingID is a private constant string that contains the ID of system default registry setting.
	defaultRegistrySettingID = "system-default-registry"
	// serverURLSettingID is a private constant string that contains the ID of server URL setting.
	serverURLSettingID = "server-url"
	rancherChartsName  = "rancher-charts"
	active             = "active"
)

var (
	metadataName      = "metadata.name="
	FiveMinuteTimeout = int64(5 * 60)
)

// InstallOptions is a struct of the required options to install a chart.
type InstallOptions struct {
	Cluster   *clusters.ClusterMeta
	Version   string
	ProjectID string
}

// RancherMonitoringOpts is a struct of the required options to install Rancher Monitoring with desired chart values.
type RancherMonitoringOpts struct {
	IngressNginx      bool `json:"ingressNginx" yaml:"ingressNginx"`
	ControllerManager bool `json:"controllerManager" yaml:"controllerManager"`
	Etcd              bool `json:"etcd" yaml:"etcd"`
	Proxy             bool `json:"proxy" yaml:"proxy"`
	Scheduler         bool `json:"scheduler" yaml:"scheduler"`
}

// RancherBackupOpts is a struct of the required options to install Rancher Backups with desired chart values.
type RancherBackupRestoreOpts struct {
	VolumeName                string
	StorageClassName          string
	BucketName                string
	CredentialSecretName      string
	CredentialSecretNamespace string
	Enabled                   bool
	Endpoint                  string
	Folder                    string
	Region                    string
}

type PrometheusFederatorOpts struct {
	EnablePodSecurity bool
}

// RancherLoggingOpts is a struct of the required options to install Rancher Logging with desired chart values.
type RancherLoggingOpts struct {
	AdditionalLoggingSources bool
}

// RancherAlertingOpts is a struct of the required options to install Rancher Alerting Drivers with desired chart values.
type RancherAlertingOpts struct {
	SMS   bool
	Teams bool
}

// GetChartCaseEndpointResult is a struct that GetChartCaseEndpoint helper function returns.
// It contains the boolean for healthy response and the request body.
type GetChartCaseEndpointResult struct {
	Ok   bool
	Body string
}

// payloadOpts is a private struct that contains the options for the chart payloads.
// It is used to avoid passing the same options to different functions while using the chart helpers.
type PayloadOpts struct {
	InstallOptions
	Name            string
	Namespace       string
	Host            string
	DefaultRegistry string
}

// newChartInstallAction is a private constructor that creates a payload for chart install action with given namespace, projectID, and chartInstalls.
func newChartInstallAction(namespace, projectID string, chartInstalls []types.ChartInstall) *types.ChartInstallAction {
	return &types.ChartInstallAction{
		DisableHooks:             false,
		Timeout:                  &metav1.Duration{Duration: 600 * time.Second},
		Wait:                     true,
		Namespace:                namespace,
		ProjectID:                projectID,
		DisableOpenAPIValidation: false,
		Charts:                   chartInstalls,
	}
}

// newChartInstall is a private constructor that creates a chart install with given chart values that can be used for chart install action.
func newChartInstall(name, version, clusterID, clusterName, url, repoName, projectID, defaultRegistry string, chartValues map[string]interface{}) *types.ChartInstall {
	chartInstall := types.ChartInstall{
		Annotations: map[string]string{
			"catalog.cattle.io/ui-source-repo":      repoName,
			"catalog.cattle.io/ui-source-repo-type": "cluster",
		},
		ChartName:   name,
		ReleaseName: name,
		Version:     version,
		Values: v3.MapStringInterface{
			"global": map[string]interface{}{
				"cattle": map[string]string{
					"clusterId":             clusterID,
					"clusterName":           clusterName,
					"rkePathPrefix":         "",
					"rkeWindowsPathPrefix":  "",
					"systemDefaultRegistry": defaultRegistry,
					"url":                   url,
					"systemProjectId":       strings.TrimPrefix(projectID, "local:"),
				},
				"systemDefaultRegistry": defaultRegistry,
			},
		},
	}

	// Add the prometheus-node-exporter hostRootFsMount configuration
	chartInstall.Values["prometheus-node-exporter"] = map[string]interface{}{
		"hostRootFsMount": map[string]interface{}{
			"enabled": false,
		},
	}

	for k, v := range chartValues {
		chartInstall.Values[k] = v
	}

	return &chartInstall
}

// newChartUninstallAction is a private constructor that creates a default payload for chart uninstall action with all disabled options.
func newChartUninstallAction() *types.ChartUninstallAction {
	return &types.ChartUninstallAction{
		DisableHooks: false,
		DryRun:       false,
		KeepHistory:  false,
		Timeout:      nil,
		Description:  "",
	}
}

// UninstallChart uninstalls a Rancher chart from a specified cluster and namespace.
func UninstallChart(client *rancher.Client, clusterId, chartName, namespace string) error {
	e2e.Logf("Getting catalog client for cluster: %s", clusterId)
	catalogClient, err := client.GetClusterCatalogClient(clusterId)
	if err != nil {
		return err
	}

	defaultChartUninstallAction := newChartUninstallAction()

	// Uninstall the chart from the given namespace
	e2e.Logf("Uninstalling chart: %s in namespace: %s", chartName, namespace)
	err = catalogClient.UninstallChart(chartName, namespace, defaultChartUninstallAction)
	if err != nil {
		return err
	}

	// Watch for the app resource to be deleted
	e2e.Logf("Waiting for chart: %s to be fully uninstalled.", chartName)
	watchAppInterface, err := catalogClient.Apps(namespace).Watch(context.TODO(), metav1.ListOptions{
		FieldSelector:  metadataName + chartName, // Fix: Properly format FieldSelector
		TimeoutSeconds: &FiveMinuteTimeout,
	})
	if err != nil {
		return err
	}

	// Wait for the app to be removed from the namespace
	err = wait.WatchWait(watchAppInterface, func(event watch.Event) (bool, error) {
		switch event.Type {
		case watch.Deleted:
			e2e.Logf("Chart %s successfully uninstalled.", chartName)
			return true, nil // Uninstallation succeeded
		case watch.Error:
			return false, fmt.Errorf("error occurred while watching app uninstallation: %v", event.Object)
		default:
			return false, nil // Continue waiting
		}
	})
	if err != nil {
		return err
	}

	e2e.Logf("Successfully uninstalled chart: %s from namespace: %s", chartName, namespace)
	return nil
}
