resource "random_id" "bucket_suffix" {
  byte_length = 4
}

resource "aws_s3_bucket" "backup_bucket" {
  provider      = aws.s3
  bucket        = "${var.prefix}-${random_id.bucket_suffix.hex}"
  force_destroy = true

  tags = {
    Name        = "${var.prefix}-${random_id.bucket_suffix.hex}"
    Environment = "test"
  }
}
