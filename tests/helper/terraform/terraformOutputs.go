package terraform

import (
	"encoding/json"
	"fmt"
)

type TerraformOutputs struct {
	PublicIP     string `json:"public_ip"`
	S3BucketName string `json:"s3_bucket_name"`
	NetworkInfo  struct {
		SubnetID string `json:"subnet_id"`
		VPCID    string `json:"vpc_id"`
	} `json:"network_info"`
}

func ParseTerraformOutputs(tfCtx *TerraformContext) (*TerraformOutputs, error) {
	rawOutputs := tfCtx.OutputAll()

	buf, err := json.Marshal(rawOutputs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal terraform outputs: %w", err)
	}

	var outputs TerraformOutputs
	err = json.Unmarshal(buf, &outputs)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal terraform outputs: %w", err)
	}

	return &outputs, nil
}
