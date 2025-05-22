package rancher

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/rancher/shepherd/clients/rancher"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"
	"gopkg.in/yaml.v3"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	machineConfigAPIPath = "v1/rke-machine-config.cattle.io.amazonec2configs/fleet-default"
	clusterspecAPIPath   = "v1/provisioning.cattle.io.clusters"
)

// Root structure
type Config struct {
	Token         string        `yaml:"token" json:"token"`
	MachineConfig MachineConfig `yaml:"machineconfig" json:"machineConfig"`
	ClusterSpec   ClusterSpec   `yaml:"clusterspec" json:"clusterSpec"`
}

// MachineConfig structure
type MachineConfig struct {
	Data MachineData `yaml:"data" json:"data"`
}

// MachineData structure
type MachineData struct {
	InstanceType  string   `yaml:"instanceType" json:"instanceType"`
	Metadata      Metadata `yaml:"metadata" json:"metadata"`
	Region        string   `yaml:"region" json:"region"`
	SecurityGroup []string `yaml:"securityGroup" json:"securityGroup"`
	SubnetID      string   `yaml:"subnetId" json:"subnetId"`
	VpcID         string   `yaml:"vpcId" json:"vpcId"`
	Zone          string   `yaml:"zone" json:"zone"`
	Type          string   `yaml:"type" json:"type"`
}

// Metadata structure
type Metadata struct {
	Namespace string `yaml:"namespace" json:"namespace"`
}

// ClusterSpec structure
type ClusterSpec struct {
	Metadata ClusterMetadata `yaml:"metadata" json:"metadata"`
	Spec     ClusterSpecData `yaml:"spec" json:"spec"`
}

// ClusterMetadata structure
type ClusterMetadata struct {
	Namespace string `yaml:"namespace" json:"namespace"`
	Name      string `yaml:"name" json:"name"`
}

// ClusterSpecData structure
type ClusterSpecData struct {
	RKEConfig                 RKEConfig `yaml:"rkeConfig" json:"rkeConfig"`
	KubernetesVersion         string    `yaml:"kubernetesVersion" json:"kubernetesVersion"`
	CloudCredentialSecretName string    `yaml:"cloudCredentialSecretName" json:"cloudCredentialSecretName"`
}

// RKEConfig structure
type RKEConfig struct {
	ChartValues     map[string]interface{} `yaml:"chartValues" json:"chartValues"`
	UpgradeStrategy UpgradeStrategy        `yaml:"upgradeStrategy" json:"upgradeStrategy"`
	MachinePools    []MachinePool          `yaml:"machinePools" json:"machinePools"`
}

// UpgradeStrategy structure
type UpgradeStrategy struct {
	ControlPlaneConcurrency string `yaml:"controlPlaneConcurrency" json:"controlPlaneConcurrency"`
	WorkerConcurrency       string `yaml:"workerConcurrency" json:"workerConcurrency"`
}

// MachinePool structure
type MachinePool struct {
	Name             string           `yaml:"name" json:"name"`
	EtcdRole         bool             `yaml:"etcdRole" json:"etcdRole"`
	ControlPlaneRole bool             `yaml:"controlPlaneRole" json:"controlPlaneRole"`
	WorkerRole       bool             `yaml:"workerRole" json:"workerRole"`
	Quantity         int              `yaml:"quantity" json:"quantity"`
	MachineConfigRef MachineConfigRef `yaml:"machineConfigRef" json:"machineConfigRef"`
}

// MachineConfigRef structure
type MachineConfigRef struct {
	Kind string `yaml:"kind" json:"kind"`
	Name string `yaml:"name" json:"name"`
}

// Function to read and parse YAML from a file
func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// Function to create a machine configuration and get the generated name
func getMachineConfigName(apiPath string, data MachineData, token string) (string, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		e2e.Logf("Error marshalling machine config data:%s", err)
		return "", err
	}

	body, err := makeRequest("POST", apiPath, string(dataBytes), token)
	if err != nil {
		e2e.Logf("Error getting api response:%s", err)
		return "", err
	}
	var response struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}

	err = json.Unmarshal([]byte(body), &response)
	if err != nil {
		e2e.Logf("Error parsing JSON response:%s", err)
		return "", err
	}

	return response.Metadata.Name, nil
}

// makeRequest sends an HTTP request with dynamic method selection (GET or POST)
func makeRequest(method, url, data, token string) (string, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	// Create a new HTTP request
	req, err := http.NewRequest(method, url, bytes.NewBuffer([]byte(data)))
	if err != nil {
		e2e.Logf("Error creating request: %s", err)
		return "", err
	}
	// Set headers
	req.Header.Set("Accept", "application/json") // Always set Accept header

	// Set Content-Type only for methods that send a body
	if method == "POST" || method == "PUT" || method == "PATCH" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		e2e.Logf("Error making request: %s", err)
		return "", err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		e2e.Logf("Error reading response: %s", err)
		return "", err
	}

	// Log response status for debugging
	e2e.Logf("Response status: %s", resp.Status)

	return string(body), nil
}

// verifyCluster checks the status of the cluster using the Clusters API
func VerifyCluster(rancherClient *rancher.Client, clusterName string) error {
	clusterAPIPath := fmt.Sprintf("https://%s/v3/clusters", rancherClient.RancherConfig.Host)

	timeout := time.After(15 * time.Minute)    // Timeout after 15 minutes
	ticker := time.NewTicker(30 * time.Second) // Check status every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for cluster %s to become Active", clusterName)

		case <-ticker.C:
			response, err := makeRequest("GET", clusterAPIPath, "", rancherClient.RancherConfig.AdminToken)
			if err != nil || response == "" {
				e2e.Logf("Failed to retrieve clusters data: %v", err)
				continue
			}

			// Struct to parse the Clusters API response
			var responseData struct {
				Data []struct {
					ID    string `json:"id"`
					Name  string `json:"name"`
					State string `json:"state"`
				} `json:"data"`
			}

			err = json.Unmarshal([]byte(response), &responseData)
			if err != nil {
				e2e.Logf("Error unmarshalling clusters response: %s", err)
				continue
			}

			// Iterate over all clusters to find the one we're monitoring
			for _, cluster := range responseData.Data {
				if cluster.Name == clusterName {
					e2e.Logf("Cluster status: %s", cluster.State)

					if cluster.State == "active" {
						e2e.Logf("Cluster %s is now Active", clusterName)
						return nil
					}
				}
			}
		}
	}
}

func CreateRKE2Cluster(rancherClient *rancher.Client, cloudCredentialName string) (string, error) {
	config, err := loadConfig("./../../../tests/helper/yamls/inputClusterConfig.yaml")
	if err != nil {
		e2e.Logf("Error loading config: %v", err)
		return "nil", err
	}
	url := fmt.Sprintf("https://%s/%s", rancherClient.RancherConfig.Host, machineConfigAPIPath)
	machineConfigName, err := getMachineConfigName(url, config.MachineConfig.Data, rancherClient.RancherConfig.AdminToken)
	if err != nil {
		e2e.Logf("Error while getting the machine configuration:%s", err)
		return "", err
	}
	if machineConfigName == "" {
		err := fmt.Errorf("failed to retrieve machine config name")
		return "nil", err
	}

	config.ClusterSpec.Spec.RKEConfig.MachinePools[0].MachineConfigRef.Name = machineConfigName
	config.ClusterSpec.Spec.CloudCredentialSecretName = cloudCredentialName
	config.ClusterSpec.Metadata.Name = namegen.AppendRandomString(config.ClusterSpec.Metadata.Name)
	dataBytes, err := json.Marshal(config.ClusterSpec)
	if err != nil {
		err := fmt.Errorf("error marshalling cluster spec data")
		return "nil", err
	}
	url = fmt.Sprintf("https://%s/%s", rancherClient.RancherConfig.Host, clusterspecAPIPath)
	_, err = makeRequest("POST", url, string(dataBytes), rancherClient.RancherConfig.AdminToken)
	if err != nil {
		e2e.Logf("Error getting api response:%s", err)
		return "", err
	}
	err = VerifyCluster(rancherClient, config.ClusterSpec.Metadata.Name)
	if err != nil {
		err := fmt.Errorf("cluster %s is now Active", config.ClusterSpec.Metadata.Name)
		return "nil", err
	}
	return config.ClusterSpec.Metadata.Name, nil
}

// DeleteCluster deletes a Rancher cluster using the provisioning API
func DeleteCluster(rancherClient *rancher.Client, clusterName string) error {
	// Build the URL for deleting the cluster
	url := fmt.Sprintf("https://%s/%s/fleet-default/%s", rancherClient.RancherConfig.Host, clusterspecAPIPath, clusterName)
	e2e.Logf("Sending DELETE request to %s", url)

	// Send the DELETE request
	_, err := makeRequest("DELETE", url, "", rancherClient.RancherConfig.AdminToken)
	if err != nil {
		e2e.Logf("Error deleting cluster %s: %v", clusterName, err)
		return fmt.Errorf("failed to delete cluster %s: %w", clusterName, err)
	}

	e2e.Logf("Successfully deleted cluster %s.", clusterName)
	return nil
}
