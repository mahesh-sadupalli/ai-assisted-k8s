# agent

**AI reliability agent · grows M1 → M5**

The spine of the project. Built on the Claude API directly (function calling / tool use).
Every action is auditable; any state-changing remediation is gated behind human approval.

Capability by milestone:

| Stage | Capability |
|-------|-----------|
| M1 | Summarize a log snippet handed to it |
| M2 | Pull logs/state from the cluster (client-go / Loki) |
| M3 | Review a `terraform plan`, flag risky changes |
| M4 | Watch CI/CD, flag risky deploys, suggest rollback |
| M5 | Full loop: Prometheus alert → triage → auditable remediation |

> Starts at M1 with the log summarizer.
