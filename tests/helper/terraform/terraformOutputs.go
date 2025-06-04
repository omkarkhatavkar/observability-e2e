package terraform

import (
	"encoding/json"
	"fmt"
)

type TerraformValue struct {
	Sensitive bool        `json:"sensitive"`
	Type      string      `json:"type"`
	Value     interface{} `json:"value"`
}

type TerraformOutputs struct {
	PublicIP     string `json:"ec2_public_ip"`
	S3BucketName string `json:"s3_bucket_name"`
	SubnetID     string `json:"subnet_id"`
	VPCID        string `json:"vpc_id"`
}

func ParseTerraformOutputs(tfCtx *TerraformContext) (*TerraformOutputs, error) {
	rawOutputs := tfCtx.OutputAll()

	// Marshal to JSON
	buf, err := json.Marshal(rawOutputs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal terraform outputs: %w", err)
	}

	// Unmarshal into correct structure
	var outputs TerraformOutputs
	if err := json.Unmarshal(buf, &outputs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal terraform outputs: %w", err)
	}

	return &outputs, nil
}
