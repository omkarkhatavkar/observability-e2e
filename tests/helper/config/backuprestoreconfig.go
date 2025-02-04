package config

const (
	BackupRestoreConfigurationFileKey = "backupRestoreInput"
)

type BackupRestoreConfig struct {
	S3BucketName               string `json:"s3BucketName" yaml:"s3BucketName" default:"default-bucket"`
	S3FolderName               string `json:"s3FolderName" yaml:"s3FolderName" default:"/backups"`
	S3Region                   string `json:"s3Region" yaml:"s3Region" default:"us-west-1"`
	S3Endpoint                 string `json:"s3Endpoint" yaml:"s3Endpoint"`
	VolumeName                 string `json:"volumeName" yaml:"volumeName"`
	StorageClassName           string `json:"storageClassName" yaml:"storageClassName"`
	CredentialSecretName       string `json:"credentialSecretName" yaml:"credentialSecretName"`
	CredentialSecretNamespace  string `json:"credentialSecretNamespace" yaml:"credentialSecretNamespace" default:"default"`
	TLSSkipVerify              bool   `json:"tlsSkipVerify" yaml:"tlsSkipVerify" default:"true"`
	EndpointCA                 string `json:"endpointCA" yaml:"endpointCA"`
	DeleteTimeoutSeconds       int    `json:"deleteTimeoutSeconds" yaml:"deleteTimeoutSeconds" default:"300"`
	RetentionCount             int    `json:"retentionCount" yaml:"retentionCount" default:"10"`
	Prune                      bool   `json:"prune" yaml:"prune" default:"true"`
	ResourceSetName            string `json:"resourceSetName" yaml:"resourceSetName"`
	EncryptionConfigSecretName string `json:"encryptionConfigSecretName" yaml:"encryptionConfigSecretName"`
	Schedule                   string `json:"schedule" yaml:"schedule"`
	AccessKey                  string `json:"accessKey" yaml:"accessKey"`
	SecretKey                  string `json:"secretKey" yaml:"secretKey"`
}
