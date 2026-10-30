variable "region" {
  type    = string
  default = "eu-central-1"
}

variable "cluster_name" {
  type    = string
  default = "sentinel"
}

variable "kubernetes_version" {
  type    = string
  default = "1.34"
}

variable "node_instance_type" {
  type    = string
  default = "t3.medium"
}

variable "node_count" {
  type    = number
  default = 2
}

variable "ecr_repositories" {
  type    = list(string)
  default = ["control-plane", "hello"]
}

variable "github_repo" {
  description = "owner/name allowed to deploy via GitHub Actions OIDC"
  type        = string
  default     = "mahesh-sadupalli/ai-assisted-k8s"
}
