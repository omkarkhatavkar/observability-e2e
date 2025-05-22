variable "vpc_cidr" {
  description = "CIDR block for VPC"
  type        = string
}

variable "subnet_cidr" {
  description = "CIDR block for Subnet"
  type        = string
}

variable "aws_zone" {
  description = "Availability Zone for subnet"
  type        = string
}

variable "prefix" {
  description = "Prefix for naming resources"
  type        = string
}
