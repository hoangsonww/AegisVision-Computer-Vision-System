# AegisVision cluster Terraform — EKS-flavored. Mirrors the topology from
# deploy/k8s/base/namespaces.yaml: a control-plane node group, a streaming
# node group, GPU node groups (L4/L40S inference + H100 training), and a
# storage node group with NVMe-backed nodes.
#
# Provider blocks are kept generic so the same module can target GKE or
# Azure by swapping the cluster + nodepool resources; the surrounding VPC,
# IRSA, and Vault wiring are AWS-specific.

terraform {
  required_version = ">= 1.7.0"
  required_providers {
    aws        = { source = "hashicorp/aws",        version = "~> 5.50" }
    kubernetes = { source = "hashicorp/kubernetes", version = "~> 2.27" }
    helm       = { source = "hashicorp/helm",       version = "~> 2.13" }
  }
}

variable "region"      { type = string  default = "us-east-1" }
variable "cluster_name" { type = string  default = "aegis-prod" }
variable "k8s_version" { type = string  default = "1.30" }
variable "vpc_cidr"    { type = string  default = "10.0.0.0/16" }
variable "single_nat_gateway" { type = bool default = false }
variable "node_groups" {
  type = map(object({
    instance_types : list(string)
    min_size : number
    max_size : number
    desired_size : number
    labels : map(string)
    taints : optional(list(object({ key = string, value = string, effect = string })), [])
    capacity_type : optional(string, "ON_DEMAND")
  }))
  default = {
    control = {
      instance_types = ["m6i.xlarge"]
      min_size = 3, max_size = 6, desired_size = 3
      labels = { "aegisvision.io/pool" = "control" }
    }
    stream = {
      instance_types = ["c6i.4xlarge"]
      min_size = 3, max_size = 20, desired_size = 4
      labels = { "aegisvision.io/pool" = "stream" }
    }
    gpu-inference = {
      instance_types = ["g6.xlarge"]   # L4
      min_size = 2, max_size = 20, desired_size = 2
      labels = { "aegisvision.io/pool" = "gpu-inference", "nvidia.com/gpu" = "true" }
      taints = [{ key = "nvidia.com/gpu", value = "true", effect = "NO_SCHEDULE" }]
    }
    gpu-training = {
      instance_types = ["p5.48xlarge"]  # H100; preemptible-friendly
      min_size = 0, max_size = 8, desired_size = 0
      labels = { "aegisvision.io/pool" = "gpu-training", "nvidia.com/gpu" = "true" }
      taints = [{ key = "nvidia.com/gpu", value = "true", effect = "NO_SCHEDULE" }]
      capacity_type = "SPOT"
    }
    storage = {
      instance_types = ["i4i.4xlarge"]
      min_size = 3, max_size = 6, desired_size = 3
      labels = { "aegisvision.io/pool" = "storage" }
    }
  }
}

# ---------- VPC ----------
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.7"
  name    = "${var.cluster_name}-vpc"
  cidr    = var.vpc_cidr
  azs     = ["${var.region}a", "${var.region}b", "${var.region}c"]
  private_subnets = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  public_subnets  = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]
  enable_nat_gateway = true
  single_nat_gateway = var.single_nat_gateway
  enable_dns_hostnames = true
  enable_flow_log      = true
  flow_log_destination_type = "cloud-watch-logs"
  tags = { "aegisvision.io/cluster" = var.cluster_name }
}

# ---------- EKS ----------
module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.13"

  cluster_name    = var.cluster_name
  cluster_version = var.k8s_version
  vpc_id          = module.vpc.vpc_id
  subnet_ids      = module.vpc.private_subnets

  cluster_endpoint_public_access  = true
  cluster_endpoint_private_access = true

  cluster_encryption_config = {
    provider_key_arn = aws_kms_key.eks.arn
    resources        = ["secrets"]
  }

  cluster_addons = {
    coredns                = { most_recent = true }
    kube-proxy             = { most_recent = true }
    vpc-cni                = { most_recent = true }
    aws-ebs-csi-driver     = { most_recent = true }
    eks-pod-identity-agent = { most_recent = true }
  }

  enable_irsa = true

  eks_managed_node_groups = {
    for k, v in var.node_groups : k => {
      name           = k
      instance_types = v.instance_types
      min_size       = v.min_size
      max_size       = v.max_size
      desired_size   = v.desired_size
      capacity_type  = v.capacity_type
      labels         = v.labels
      taints         = v.taints
      iam_role_additional_policies = {
        ssm = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
      }
    }
  }

  tags = { "aegisvision.io/cluster" = var.cluster_name }
}

resource "aws_kms_key" "eks" {
  description             = "EKS secrets encryption"
  deletion_window_in_days = 30
  enable_key_rotation     = true
  tags = { "aegisvision.io/cluster" = var.cluster_name }
}

output "cluster_endpoint" { value = module.eks.cluster_endpoint }
output "cluster_name"     { value = module.eks.cluster_name }
output "kubeconfig_command" {
  value = "aws eks update-kubeconfig --name ${module.eks.cluster_name} --region ${var.region}"
}
