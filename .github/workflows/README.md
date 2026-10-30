# .github/workflows

**Milestone 4 · CI/CD**

| Workflow | Trigger | What it does |
|---|---|---|
| `ci.yml` | every PR, push to `main`, and called by `deploy.yml` | gofmt, `go vet`, `go test -race`, `terraform validate`, image builds |
| `deploy.yml` | merge to `main` | CI gate → AWS via **OIDC** (no keys) → push to ECR → `scripts/deploy.sh` (rollout + behavioural smoke test + **auto-rollback**) → `agent deploy-review` posts risk + rollback advice to the run summary |

`deploy` is skipped until the `AWS_ROLE_ARN` repository variable is set. The header of
`deploy.yml` lists every variable it needs (all from `terraform output`), plus the Claude
Workload Identity Federation variables.
