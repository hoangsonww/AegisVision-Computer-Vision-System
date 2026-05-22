# Cluster Terraform

Provisions the AegisVision cluster topology from the architecture doc §11:

- VPC with private + public subnets across 3 AZs
- EKS 1.30 cluster with secrets KMS encryption
- 5 node groups: control / stream / gpu-inference (L4) / gpu-training (H100 spot, scale-to-zero) / storage

## Usage

```bash
cd deploy/terraform
terraform init
terraform plan  -var cluster_name=aegis-staging
terraform apply -var cluster_name=aegis-staging
$(terraform output -raw kubeconfig_command)
```

After the cluster comes up:

1. Install ArgoCD: `kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml`
2. Apply the root app: `kubectl apply -f ../platform/argocd/root-app.yaml`
3. ArgoCD reconciles the platform tier (Istio, Vault, ESO, SPIRE, observability, Kyverno) and then the service tier.

## Multi-region

For multi-region, run this module per region with a unique `cluster_name`
and federate the data plane via the ApplicationSet `clusters` generator.
