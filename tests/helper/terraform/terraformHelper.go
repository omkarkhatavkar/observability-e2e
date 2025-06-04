package terraform

import (
	"fmt"
	"os"
)

// LoadVarsFromEnv builds a map of Terraform variables by checking env vars.
// Only adds the variable if the corresponding environment variable is set.
func LoadVarsFromEnv(envToTfVarMap map[string]string) map[string]interface{} {
	vars := make(map[string]interface{})
	for envKey, tfVarName := range envToTfVarMap {
		val := os.Getenv(envKey)
		if val != "" {
			vars[tfVarName] = val
		}
	}
	return vars
}

// Sets TF_VAR_* environment variables from a map and returns a cleanup function
func SetTerraformEnvVarsFromMap(envToTfVarMap map[string]string) error {
	for envKey, tfVarName := range envToTfVarMap {
		val := os.Getenv(envKey)
		if val == "" {
			continue // skip unset values
		}
		tfEnvKey := fmt.Sprintf("TF_VAR_%s", tfVarName)
		err := os.Setenv(tfEnvKey, val)
		if err != nil {
			return fmt.Errorf("failed to set %s: %w", tfEnvKey, err)
		}
	}
	return nil
}
