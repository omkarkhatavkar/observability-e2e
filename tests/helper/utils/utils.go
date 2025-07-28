package utils

import (
	"archive/tar"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/creasty/defaults"
	ginkgo "github.com/onsi/ginkgo/v2"
	rancher "github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/kubectl"
	shepherdConfig "github.com/rancher/shepherd/pkg/config"
	"gopkg.in/yaml.v2"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type Config struct {
	Brand          string `json:"brand" yaml:"brand"`
	GitCommit      string `json:"gitCommit" yaml:"gitCommit"`
	IsPrime        bool   `json:"isPrime" yaml:"isPrime" default:"false"`
	RancherVersion string `json:"rancherVersion" yaml:"rancherVersion"`
	Registry       string `json:"registry" yaml:"registry"`
}

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

// GetEnvOrDefault retrieves the value of an environment variable
// or returns a default value if the variable is not set.
func GetEnvOrDefault(key, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue
	}
	return value
}

func RequestRancherVersion(rancherURL string) (*Config, error) {
	httpURL := "https://" + rancherURL + "/rancherversion"

	// Insecure TLS config to skip certificate verification
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ⚠️ For dev only
	}
	client := &http.Client{Transport: tr}

	resp, err := client.Get(httpURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	byteObject, err := io.ReadAll(resp.Body)
	if err != nil || byteObject == nil {
		return nil, err
	}

	var jsonObject map[string]interface{}
	err = json.Unmarshal(byteObject, &jsonObject)
	if err != nil {
		return nil, err
	}

	configObject := new(Config)
	configObject.IsPrime, _ = strconv.ParseBool(jsonObject["RancherPrime"].(string))
	configObject.RancherVersion = jsonObject["Version"].(string)
	configObject.GitCommit = jsonObject["GitCommit"].(string)

	return configObject, nil
}

// GetRancherVersion returns the Rancher version in X.Y format (major.minor)
// Handles formats like:
//   - "rancher/rancher:v2.10.3"          → "2.10"
//   - "v2.11-abc123-head"                → "2.11"
//   - "2.12.1"                           → "2.12"
//   - "rancher/rancher:v2.13.0-alpha1"   → "2.13"
func GetRancherVersion(clientWithSession *rancher.Client) (string, error) {
	rancherConfig, err := RequestRancherVersion(clientWithSession.RancherConfig.Host)
	if err != nil {
		return "", fmt.Errorf("failed to get Rancher version: %w", err)
	}

	image := strings.TrimSpace(rancherConfig.RancherVersion)
	if image == "" {
		return "", fmt.Errorf("empty Rancher version string")
	}

	// Extract version part, e.g., "v2.11-abc123-head" or "2.10.3"
	var versionWithV string
	if strings.Contains(image, ":") {
		parts := strings.Split(image, ":")
		if len(parts) != 2 {
			return "", fmt.Errorf("unexpected version format: %s", image)
		}
		versionWithV = parts[1]
	} else {
		versionWithV = image
	}

	// Trim leading 'v' if present
	version := strings.TrimPrefix(versionWithV, "v")

	// Remove any suffix like "-head" or "-<commit>"
	if dashIdx := strings.Index(version, "-"); dashIdx != -1 {
		version = version[:dashIdx]
	}

	// Split into major.minor.patch
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid version format: %s", version)
	}

	major := parts[0]
	minor := parts[1]

	return fmt.Sprintf("%s.%s", major, minor), nil
}

func CreateTempDir(dirName string) (string, error) {
	tmpPath := filepath.Join(os.TempDir(), dirName)
	err := os.MkdirAll(tmpPath, 0755) // 0755 = rwxr-xr-x
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	return tmpPath, nil
}

func ExtractTarGz(srcFile, destDir string) error {
	f, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

// findProjectRoot searches upward for a directory containing markers like ".git" or "go.mod"
func findProjectRoot(startDir string, markers []string) (string, error) {
	dir := startDir
	for {
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}
	return "", fmt.Errorf("project root not found (markers: %v)", markers)
}

// GetYamlPath returns the absolute path to a YAML file given its path relative to project root.
// Panics on failure to simplify caller code.
func GetYamlPath(relativeYamlPath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("failed to get current directory: %v", err))
	}

	root, err := findProjectRoot(cwd, []string{".git", "go.mod"})
	if err != nil {
		panic(fmt.Sprintf("failed to find project root: %v", err))
	}

	absPath := filepath.Join(root, relativeYamlPath)
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		panic(fmt.Sprintf("failed to get absolute path: %v", err))
	}

	return absPath
}

func GenerateYAMLFromTemplate(templateFile, outputFile string, data any) error {
	tmpl, err := template.ParseFiles(templateFile)
	if err != nil {
		return err
	}

	output, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer output.Close()

	return tmpl.Execute(output, data)
}

// SafeCleanup wraps a cleanup function to ensure it only runs once.
// It registers it with Ginkgo's DeferCleanup and returns a manual trigger.
func SafeCleanup(description string, cleanupFunc func()) func() {
	var once sync.Once
	wrapped := func() {
		ginkgo.By(description)
		cleanupFunc()
	}
	ginkgo.DeferCleanup(func() {
		once.Do(wrapped)
	})
	return func() {
		once.Do(wrapped)
	}
}
