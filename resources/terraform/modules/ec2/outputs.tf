output "public_ip" {
  value = local.rke2_host_ip
}

output "instance_id" {
  value = aws_instance.rke2_node.id
}
