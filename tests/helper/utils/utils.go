package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/creasty/defaults"
	rancher "github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/kubectl"
	shepherdConfig "github.com/rancher/shepherd/pkg/config"
	"gopkg.in/yaml.v2"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

func DeployPrometheusRule(mySession *rancher.Client, yamlPath string) error {

	yamlContent, err := os.ReadFile(yamlPath)
	if err != nil {
		log.Fatalf("Failed to read file %s: %v", yamlPath, err)
	}

	importYamlInput := &management.ImportClusterYamlInput{
		YAML: string(yamlContent),
	}

	apply := []string{"kubectl", "apply", "-f", "/root/.kube/my-pod.yaml"}
	prometheusRuleApply, err := kubectl.Command(mySession, importYamlInput, "local", apply, "")
	if err != nil {
		return err
	}
	e2e.Logf("Successfully: %v", prometheusRuleApply)

	return nil
}

func DeployAlertManagerConfig(mySession *rancher.Client, yamlPath string) error {

	yamlContent, err := os.ReadFile(yamlPath)
	if err != nil {
		log.Fatalf("Failed to read file %s: %v", yamlPath, err)
	}

	importYamlInput := &management.ImportClusterYamlInput{
		YAML: string(yamlContent),
	}

	apply := []string{"kubectl", "apply", "-f", "/root/.kube/my-pod.yaml"}
	alertManagerConfigApply, err := kubectl.Command(mySession, importYamlInput, "local", apply, "")
	if err != nil {
		return err
	}
	e2e.Logf("Successfully: %v", alertManagerConfigApply)

	return nil
}

func DeployLoggingClusterOutputAndClusterFlow(mySession *rancher.Client, yamlPath string) error {

	yamlContent, err := os.ReadFile(yamlPath)
	if err != nil {
		log.Fatalf("Failed to read file %s: %v", yamlPath, err)
	}

	importYamlInput := &management.ImportClusterYamlInput{
		YAML: string(yamlContent),
	}

	apply := []string{"kubectl", "apply", "-f", "/root/.kube/my-pod.yaml"}
	loggingResources, err := kubectl.Command(mySession, importYamlInput, "local", apply, "")
	if err != nil {
		return err
	}
	e2e.Logf("Successfully : %v", loggingResources)

	return nil
}

func DeploySyslogResources(mySession *rancher.Client, yamlPath string) error {

	yamlContent, err := os.ReadFile(yamlPath)
	if err != nil {
		log.Fatalf("Failed to read file %s: %v", yamlPath, err)
	}

	importYamlInput := &management.ImportClusterYamlInput{
		YAML: string(yamlContent),
	}

	apply := []string{"kubectl", "apply", "-f", "/root/.kube/my-pod.yaml"}
	syslogResources, err := kubectl.Command(mySession, importYamlInput, "local", apply, "")
	if err != nil {
		return err
	}
	e2e.Logf("Successfully : %v", syslogResources)

	return nil
}

func DeployYamlResource(mySession *rancher.Client, yamlPath string, namespace string) error {

	yamlContent, err := os.ReadFile(yamlPath)
	if err != nil {
		log.Fatalf("Failed to read file %s: %v", yamlPath, err)
	}

	importYamlInput := &management.ImportClusterYamlInput{
		YAML: string(yamlContent),
	}

	apply := []string{"kubectl", "apply", "-f", "/root/.kube/my-pod.yaml", "-n", namespace}
	yamlApply, err := kubectl.Command(mySession, importYamlInput, "local", apply, "")
	if err != nil {
		return err
	}
	e2e.Logf("Successfully fetchall: %v", yamlApply)

	return nil
}

func DeleteYamlResource(mySession *rancher.Client, yamlPath string, namespace string) error {

	yamlContent, err := os.ReadFile(yamlPath)
	if err != nil {
		log.Fatalf("Failed to read file %s: %v", yamlPath, err)
	}

	importYamlInput := &management.ImportClusterYamlInput{
		YAML: string(yamlContent),
	}

	apply := []string{"kubectl", "delete", "-f", "/root/.kube/my-pod.yaml", "-n", namespace}
	yamlApply, err := kubectl.Command(mySession, importYamlInput, "local", apply, "")
	if err != nil {
		return err
	}
	e2e.Logf("Successfully fetchall: %v", yamlApply)

	return nil
}

// LoadConfigIntoStruct loads a config file and unmarshals it into the given struct.
func LoadConfigIntoStruct(filePath string, config interface{}) error {
	// Load the config file as a map
	configMap := shepherdConfig.LoadConfigFromFile(filePath)
	if configMap == nil {
		return fmt.Errorf("failed to load config file: %s", filePath)
	}
	// Marshal the map into bytes
	configBytes, err := yaml.Marshal(configMap)
	if err != nil {
		return fmt.Errorf("failed to marshal config map: %w", err)
	}

	// Unmarshal the bytes into the config struct (ensure config is a pointer)
	if err := yaml.Unmarshal(configBytes, config); err != nil {
		return fmt.Errorf("failed to unmarshal into config struct: %w", err)
	}

	// Apply defaults, config must be a pointer
	if err := defaults.Set(config); err != nil {
		return fmt.Errorf("failed to set default values: %w", err)
	}
	return nil
}

// ConvertToStruct converts a source interface{} (typically JSON-like) into a target struct.
func ConvertToStruct(src interface{}, target interface{}) error {
	rawData, err := json.Marshal(src)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	if err := json.Unmarshal(rawData, target); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return nil
}
