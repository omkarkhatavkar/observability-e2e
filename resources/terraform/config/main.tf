module "vpc" {
  source      = "../modules/vpc"
  vpc_cidr    = var.vpc_cidr
  subnet_cidr = var.subnet_cidr
  aws_zone    = var.aws_zone
  prefix      = var.prefix
}

module "s3" {
  source = "../modules/s3"
  prefix = var.prefix
}

module "ec2" {
  source = "../modules/ec2"

  ami_id                = var.ami_id
  instance_type         = var.instance_type
  key_name              = var.key_name
  root_volume_size      = var.root_volume_size
  prefix                = var.prefix
  subnet_id             = module.vpc.subnet_id
  vpc_id                = module.vpc.vpc_id
  security_group_id     = module.vpc.security_group_id
  private_key_path      = var.private_key_path
  preserve_eip          = false
  rke2_version          = var.rke2_version
  cert_manager_version  = var.cert_manager_version
  encryption_secret_key = var.encryption_secret_key
  input_cluster_config  = var.input_cluster_config
  cattle_config         = var.cattle_config
}
