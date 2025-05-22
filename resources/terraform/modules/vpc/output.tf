output "vpc_id" {
  value = aws_vpc.rancher_vpc.id
}

output "subnet_id" {
  value = aws_subnet.rancher_subnet.id
}

output "security_group_id" {
  value = aws_security_group.rancher_sg_allowall.id
}
