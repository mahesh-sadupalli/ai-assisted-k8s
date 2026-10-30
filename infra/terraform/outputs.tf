output "cluster_name" {
  value = module.eks.cluster_name
}

output "region" {
  value = var.region
}

output "kubeconfig_command" {
  value = "aws eks update-kubeconfig --region ${var.region} --name ${module.eks.cluster_name}"
}

output "ecr_registry" {
  description = "Registry prefix to add to the control-plane's ALLOWED_REGISTRIES"
  value       = "${split("/", aws_ecr_repository.app["control-plane"].repository_url)[0]}/"
}
