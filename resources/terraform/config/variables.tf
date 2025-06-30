variable "aws_region_instance" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "us-east-2"
}

variable "aws_region_s3" {
  description = "AWS region to create s3 bucket for backup"
  type        = string
  default     = "us-west-2"
}

variable "vpc_cidr" {
  description = "CIDR block for VPC"
  type        = string
  default     = "10.0.0.0/16"
}

variable "subnet_cidr" {
  description = "CIDR block for Subnet"
  type        = string
  default     = "10.0.0.0/24"
}

variable "aws_zone" {
  description = "Availability Zone for subnet"
  type        = string
  default     = "us-east-2a"
}

variable "ami_id" {
  description = "AMI ID for the EC2 instance"
  type        = string
  default     = "ami-00eb69d236edcfaf8"
}

variable "instance_type" {
  description = "Type of EC2 instance"
  type        = string
  default     = "t2.2xlarge"
}

variable "key_name" {
  description = "Key pair name for EC2"
  type        = string
  sensitive   = true
}

variable "private_key_path" {
  description = "Absolute path to the SSH private key"
  type        = string
}

variable "root_volume_size" {
  description = "Root volume size for EC2 instance"
  type        = number
  default     = 60
}

variable "prefix" {
  description = "Prefix for naming resources"
  type        = string
  default     = "test"
}

variable "rke2_version" {
  description = "RKE2 version to install"
  type        = string
  default     = "v1.32.5+rke2r1"
}

variable "cert_manager_version" {
  description = "Cert Manager version to install"
  type        = string
  default     = "v1.15.3"
}

variable "encryption_secret_key" {
  description = "Secret key for Kubernetes encryption"
  type        = string
  sensitive   = true
}

variable "cattle_config" {
  description = "cattle config file path"
  type        = string
  default     = "/tmp/cattle-config.yaml"
}

variable "input_cluster_config" {
  description = "cluster config file path"
  type        = string
  default     = "/tmp/inputClusterConfig.yaml"
}

variable "preserve_eip" {
  description = "create the static eip and attach that to instance for migration scenario"
  type        = bool
  default     = true
}
variable "rancher_version" {
  description = "version of rancher under test"
}

variable "rancher_password" {
  description = "Bootstrap password for Rancher"
  type        = string
  sensitive   = true
}
variable "rancher_repo_url" {
  description = "Helm repository URL to install Rancher"
  type        = string
}

variable "install_rancher" {
  type        = bool
  default     = true
  description = "Whether to install Rancher after installing RKE2"
}
