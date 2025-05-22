variable "aws_region_s3" {
  description = "AWS region to create s3 bucket for backup"
  type        = string
  default     = "us-west-2"
}
variable "prefix" {
  description = "Prefix to use for S3 bucket name"
  type        = string
}
