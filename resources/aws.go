package resources

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	localConfig "github.com/rancher/observability-e2e/tests/helper/config"
	"github.com/rancher/observability-e2e/tests/helper/utils"
)

// S3Client wraps the AWS S3 service client
type S3Client struct {
	client *s3.S3
}

// NewS3Client creates an AWS S3 client from BackupRestoreConfig
func NewS3Client(config *localConfig.BackupRestoreConfig) (*S3Client, error) {
	// Load the config if not provided
	if config == nil {
		// Load default config
		config = &localConfig.BackupRestoreConfig{}
		err := utils.LoadConfigIntoStruct("BackupRestoreConfigurationFileKey", config)
		if err != nil {
			return nil, fmt.Errorf("failed to load default config: %v", err)
		}
	}

	// Create a new AWS session using the region from the config
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(config.S3Region),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %v", err)
	}

	// Create and return the S3 client
	client := s3.New(sess)
	return &S3Client{client: client}, nil
}

// CreateBucket creates the S3 bucket with the specified name and region
func (s *S3Client) CreateBucket(bucketName string, region string) error {
	// Create the S3 bucket
	_, err := s.client.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
		CreateBucketConfiguration: &s3.CreateBucketConfiguration{
			LocationConstraint: aws.String(region),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create bucket '%s' in region '%s': %v", bucketName, region, err)
	}
	return nil
}

// DeleteAllObjects deletes all objects in an S3 bucket
func (s *S3Client) DeleteAllObjects(bucketName string) error {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	}
	for {
		result, err := s.client.ListObjectsV2(input)
		if err != nil {
			return fmt.Errorf("failed to list objects: %v", err)
		}

		if len(result.Contents) == 0 {
			break
		}

		var objectsToDelete []*s3.ObjectIdentifier
		for _, obj := range result.Contents {
			objectsToDelete = append(objectsToDelete, &s3.ObjectIdentifier{Key: obj.Key})
		}

		_, err = s.client.DeleteObjects(&s3.DeleteObjectsInput{
			Bucket: aws.String(bucketName),
			Delete: &s3.Delete{
				Objects: objectsToDelete,
				Quiet:   aws.Bool(true),
			},
		})
		if err != nil {
			return fmt.Errorf("failed to delete objects: %v", err)
		}
	}

	return nil
}

// DeleteBucket deletes the S3 bucket
func (s *S3Client) DeleteBucket(bucketName string) error {
	// Delete all objects inside the bucket
	if err := s.DeleteAllObjects(bucketName); err != nil {
		return err
	}
	_, err := s.client.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return fmt.Errorf("failed to delete bucket '%s': %v", bucketName, err)
	}
	return nil
}
