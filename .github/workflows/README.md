# .github/workflows

**Milestone 4 · CI/CD**

GitHub Actions pipelines: lint + test Go → build + push image to ECR → deploy to EKS,
triggered on merge to `main`, authenticating to AWS via OIDC (no long-lived keys). Includes
a post-deploy smoke test and an automatic rollback trigger.

> Workflows land with M4. (This README keeps the folder tracked until then.)
