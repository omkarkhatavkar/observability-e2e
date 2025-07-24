# 🧠 Local values to derive AMI ID and instance type based on architecture
locals {
  architecture_config = {
    x86 = {
      ami_id        = "ami-00eb69d236edcfaf8"
      instance_type = "t2.2xlarge"
    }
    arm = {
      ami_id        = "ami-09b94ded43627954d"
      instance_type = "t4g.large"
    }
  }

  ami_id        = local.architecture_config[var.architecture].ami_id
  instance_type = local.architecture_config[var.architecture].instance_type
}
