# infra

**Milestones 2–3 · Terraform + Kubernetes**

Infrastructure as code. Local Kubernetes manifests (kind) at M2, then Terraform for the
real AWS footprint at M3: VPC, EKS cluster, managed node group, least-privilege IAM, and
ECR. A documented `terraform destroy` teardown path — EKS incurs cost.

| Path | What |
|---|---|
| `kind/cluster.yaml` | Local single-node cluster `sentinel` |
| `k8s/` | Kustomize base: namespaces, `ResourceQuota/sentinel-quota` (the guardrail reads it), least-privilege RBAC, control-plane |

`make up` creates the cluster, builds and loads the images, and applies `k8s/`. `make down` deletes it.

**M3 (EKS):** `terraform/` (VPC, EKS, managed node group, ECR, S3 remote state) and
`k8s-eks/` (overlay: base + ingress). See `../milestones/M3.md` for `make eks-up / eks-push /
eks-deploy / eks-destroy`. **Always run `make eks-destroy` when you're done**, because EKS bills by the hour.
