# agent

**AI reliability agent · grows M1 → M5**

The spine of the project. Built on the Claude API directly (tool use).
Every action is auditable; any state-changing remediation is gated behind human approval.

```bash
export ANTHROPIC_API_KEY=...        # or put it in .env
go run ./agent summarize agent/testdata/oomkilled.log   # M1: a log file
go run ./agent pod team-a <pod>                         # M2: one live pod
go run ./agent scan team-a                              # M2: every unhealthy pod in a namespace
go run ./agent plan plan.json                           # M3: review `terraform show -json` output
```

`pod`/`scan` use your kubeconfig, like kubectl. For each pod the agent collects status,
limits, last termination reason, events and the last 50 log lines (including the **previous,
crashed** container's), then diagnoses it.

The model answers by calling a `report_diagnosis` tool with a **strict** JSON schema
(`severity`, `what_broke`, `likely_cause`, `suggested_action`), so the output is always
parseable. No more stripping markdown fences.

Capability by milestone:

| Stage | Capability |
|-------|-----------|
| M1 | Summarize a log snippet handed to it |
| M2 | Pull logs/state from the cluster (client-go / Loki) |
| M3 | Review a `terraform plan`, flag risky changes |
| M4 | Watch CI/CD, flag risky deploys, suggest rollback |
| M5 | Full loop: Prometheus alert → triage → auditable remediation |

`plan` works in two layers: deterministic rules (`stateful-destroy`, `open-ingress`,
`iam-wildcard`) always run, and Claude reviews the whole plan with those hits as context. The
model can add risks but can never clear a rule hit: an `approve` gets downgraded to
`needs_review`. Exit code 1 on `block`, so CI can gate a merge on it.
