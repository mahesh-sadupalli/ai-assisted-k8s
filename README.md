<div align="center">

# Self-Service Kubernetes Platform with AI-Assisted Observability & Reliability

`codename: Sentinel`

**A self-service Kubernetes platform where reliability is observable end-to-end — and an AI agent turns metrics and logs into triaged, auditable fixes.**

![Status](https://img.shields.io/badge/status-Milestone%201%20in%20progress-orange)
![License](https://img.shields.io/badge/license-MIT-blue)
![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white)
![Kubernetes](https://img.shields.io/badge/Kubernetes-326CE5?logo=kubernetes&logoColor=white)
![AWS](https://img.shields.io/badge/AWS-EKS-232F3E?logo=amazonaws&logoColor=white)

</div>

---

## Table of contents

- [Overview](#overview)
- [Architecture](#architecture)
- [The AI thread](#the-ai-thread-the-spine-from-day-1)
- [Milestones](#milestones)
- [Tech stack](#tech-stack)
- [Repository layout](#repository-layout)
- [Timeline &amp; status](#timeline--status)
- [Scope](#scope)
- [Demo](#demo)
- [Quickstart](#quickstart)
- [License](#license)
- [Author](#author)

---

## Overview

Sentinel is a self-service deployment platform on Kubernetes (AWS EKS). A **Go control-plane API** lets a developer ship a service with a single request; an **LGTM-stack observability layer** (Loki, Grafana, Tempo, Prometheus) makes every service's health visible; and an **AI reliability agent** reads those metrics and logs to triage incidents and propose safe, auditable remediations.

The project is deliberately built as **one system rather than five disconnected tutorials.** Each milestone bolts a real layer onto the same platform, and the AI agent runs through all of them as the spine — not a final-quarter add-on. Observability is wired in from Milestone 2, because it is what the agent runs on.

It mirrors what an infrastructure / observability platform team actually does: self-service platform services in Go, infrastructure-as-code with reliability guarantees, Kubernetes and networking, observability with SLOs, and AI-assisted incident response.

## Architecture

```
                        Developer
                   (deploy via CLI / API)
                            │
                ┌───────────▼───────────┐
                │   Control-plane API    │  ◄── M1 · Go
                │ provisioning·guardrails│
                └───────────┬───────────┘
                            │
   Terraform ──────►  Kubernetes / EKS  ◄────── CI/CD
   (M2–M3, IaC)       (M2 → M3, runs it)     (M4, GH Actions)
                            │
                Observability (LGTM stack)     ◄── M2 thin → M5 deep
             Prometheus·Grafana·Loki·SLOs
                            │
             ┌──────────────▼──────────────┐
             │      AI RELIABILITY AGENT    │  ◄── AI thread, M1 → M5
             │ triage → suggest → remediate │
             └──────────────────────────────┘
       reads metrics + logs · reviews plans/PRs · auditable
```

## The AI thread (the spine, from Day 1)

The agent grows **monotonically** — each stage does something the previous stage could not. This table is the single source of truth for what the agent can do at each milestone.

| Stage | The agent can now… |
|-------|--------------------|
| **M1** | Summarize a **log snippet handed to it** — no cluster yet, pure agent mechanics |
| **M2** | Pull logs/state **from the cluster itself** (and query logs via Loki that it did not collect by hand) |
| **M3** | Read **infra/cloud context** and review a **Terraform plan** before merge |
| **M4** | Watch **CI/CD**, flag a risky deploy, and suggest a rollback |
| **M5** | Close the **full reliability loop** — from a Prometheus alert to a proposed, auditable remediation |

Built on the **Claude API directly** (function calling / tool use) so the mechanics are understood before wrapping any framework around it. Every agent action is **auditable** — proposals are logged, and any state-changing remediation is gated behind a human-approve step.

## Milestones

Dependency-ordered: Go first (everything is written in it) → containers/K8s (the runtime) → EKS (where it really runs) → CI/CD (how it ships) → observability depth + agent capstone (how it stays healthy). Observability starts thin at M2 and deepens at M5 so the repo demonstrates it early, not only at the end.

Each milestone lists a **goal**, **what gets built**, the **AI advance**, the **skills demonstrated**, and **"done when"** — concrete, verifiable acceptance criteria.

---

### M1 · Go control-plane API + first agent
**Bucket:** Systems programming (Go)

- **Goal:** A minimal HTTP control-plane in Go that accepts a "deploy this service" request, validates it against at least one *real* guardrail, and records the intent — plus the first AI agent that summarizes a log snippet.
- **What gets built:**
  - Go HTTP API with `POST /deployments` and `GET /deployments/{id}`
  - Request validation and **one real guardrail** — e.g. a per-namespace resource quota check plus a policy rule that actually rejects a bad request (missing resource limits, or a disallowed image registry)
  - A simple persistence layer (SQLite / in-memory) for deployment intents, plus structured logging
  - `agent/` v0: takes a log snippet and returns a structured summary (*what broke → likely cause → suggested next step*) via Claude API tool use
  - Unit tests for the guardrail and validation logic
- **AI advance:** Summarize a log snippet handed to it — pure agent mechanics, no cluster.
- **Skills demonstrated:** Go HTTP services, idiomatic project layout, testing, API design, policy/validation, Claude API function calling.
- **Done when:** an invalid request is rejected by the guardrail with a clear error; a valid one is accepted and retrievable; `agent summarize <logfile>` returns a structured summary; unit tests pass.

---

### M2 · Containers + Kubernetes + thin observability slice
**Bucket:** IaC & containers

- **Goal:** Containerize the control-plane and a sample workload, run them on a local Kubernetes cluster, and stand up the first observability slice.
- **What gets built:**
  - Multi-stage Dockerfiles (small images) for the control-plane and a sample "hello" service
  - Kubernetes manifests: `Deployment`, `Service`, `ConfigMap`, and a `Namespace` with a `ResourceQuota` (wired to the M1 guardrail)
  - A local cluster via **kind**; the control-plane applies manifests to actually deploy the sample service (thin self-service loop)
  - **Prometheus** scraping `/metrics` from the control-plane, instrumented on the **RED method** (request rate, errors, duration histogram) via `client_golang`
  - **One Grafana dashboard** (RED for the control-plane)
  - `agent/` v1: reads pod logs from the cluster via `client-go` (and, if Loki is deployed, queries logs it did not collect by hand)
- **AI advance:** Pull logs/state from the cluster itself.
- **Skills demonstrated:** Docker multi-stage builds, core Kubernetes objects, `client-go`, Prometheus instrumentation, Grafana dashboards.
- **Done when:** `kubectl get pods` shows the control-plane and sample service running on kind; the control-plane can deploy the sample service into a namespace; Prometheus scrapes its metrics; one Grafana RED dashboard renders; the agent summarizes a real pod's logs pulled from the cluster.

---

### M3 · AWS EKS + networking / ingress
**Bucket:** AWS platform depth

- **Goal:** Move from local kind to a real managed cluster on AWS EKS, provisioned with Terraform and fronted by ingress.
- **What gets built:**
  - **Terraform**: VPC, EKS cluster, managed node group, least-privilege IAM roles, and an ECR repository
  - Control-plane + sample service deployed to EKS; images pushed to ECR
  - **Ingress** (AWS Load Balancer Controller or ingress-nginx) exposing the sample service over a real URL
  - `agent/` v2: reads infra/cloud context and reviews a `terraform plan`, flagging risky changes (e.g. destroy of a stateful resource, an over-permissive security group)
- **AI advance:** Read infra/cloud context, review a Terraform plan before merge.
- **Skills demonstrated:** AWS (EKS, VPC, IAM, ECR), Terraform (modules, remote state), Kubernetes networking/ingress, cloud security basics.
- **Done when:** `terraform apply` stands up an EKS cluster from scratch; the sample service is reachable via its ingress URL; images live in ECR; the agent parses a `terraform plan` and flags at least one class of risky change. *(A `terraform destroy` teardown path is documented — EKS incurs cost.)*

---

### M4 · CI/CD with GitHub Actions
**Bucket:** CI/CD & automation

- **Goal:** Automate build → test → image push → deploy, with a safety gate and a rollback path.
- **What gets built:**
  - **GitHub Actions** pipeline: lint + test Go → build + push image to ECR → deploy to EKS, triggered on merge to `main`, authenticating to AWS via OIDC (no long-lived keys)
  - Test gates (unit tests must pass to deploy) and a post-deploy smoke test
  - A **rollback trigger** — a failing smoke test rolls the deployment back
  - `agent/` v3: watches a workflow/deploy, flags a risky deploy (large diff, failing smoke test) and suggests a rollback
- **AI advance:** Watch CI/CD, flag a risky deploy, suggest a rollback.
- **Skills demonstrated:** GitHub Actions, GitOps-style delivery, deployment safety (smoke test + rollback), OIDC cloud auth.
- **Done when:** a PR merge drives the pipeline end-to-end (test → build → push → deploy); a failing test blocks the deploy; a failing smoke test triggers an automatic rollback; the agent posts a summary/flag on the run.

---

### M5 · Observability depth + reliability-agent capstone
**Bucket:** Prometheus / Grafana / Loki / SLOs

- **Goal:** Deepen observability into SLOs, burn-rate alerting, and centralized logs, then close the AI reliability loop.
- **What gets built:**
  - A defined **SLO** (e.g. 99.9% availability / latency target) with **multi-window, multi-burn-rate alerts** in Prometheus + Alertmanager
  - **Loki** for centralized logs, with Grafana dashboards correlating metrics and logs *(Tempo/traces = stretch)*
  - `agent/` v4 — the capstone: receives a Prometheus alert via webhook, pulls the relevant metrics and Loki logs, triages (*what / why / impact*), and proposes an **auditable remediation** gated behind a human-approve step
  - An **audit log** of every agent proposal and decision
- **AI advance:** Close the full reliability loop — from a Prometheus alert to a proposed, auditable remediation.
- **Skills demonstrated:** SLOs and error budgets, burn-rate alerting, Alertmanager, Loki, incident-response modeling, safe agentic remediation (guardrails / approval / audit).
- **Done when:** an injected failure fires an SLO burn-rate alert; the alert reaches the agent; the agent produces a triage plus a proposed remediation with a full audit trail; a human-approve step gates any state change; the capstone demo is recorded.

---

## Tech stack

`Go` · `Kubernetes (EKS)` · `Prometheus` · `Grafana` · `Loki` · `Terraform` · `GitHub Actions` · `SLOs & burn-rate alerting` · `AWS (VPC, IAM, ECR)` · `Docker` · `Agentic LLM workflows (Claude API)`

_Stretch: `Tempo` (distributed traces) to complete the LGTM picture, if time allows._

## Repository layout

```
control-plane/   M1 · Go control-plane API
infra/           M2–M3 · Terraform + EKS
agent/           AI reliability agent (grows M1 → M5)
observability/   M2 thin → M5 deep · Prometheus + Grafana + Loki + SLOs
.github/         M4 · CI/CD workflows
docs/            Roadmap + living build log
```

## Timeline & status

Aggressive solo sprint, **~3–6 months** (target: applying by the end of the current contract). Workflow is **concept-first each day, then hands-on**; the week-by-week and day-by-day breakdown lives in `docs/`.

| Milestone | Status |
|-----------|--------|
| M1 · Go control-plane + first agent | 🚧 In progress |
| M2 · Containers + K8s + thin observability | ⬜ Not started |
| M3 · EKS + networking | ⬜ Not started |
| M4 · CI/CD | ⬜ Not started |
| M5 · Observability depth + capstone | ⬜ Not started |

## Scope

A **thin but complete vertical slice** — one real service flowing through the whole platform — not a production-grade internal developer platform. This is the right size for solo work in the timeline while still exercising every skill.

"Guardrails" means something concrete even in the thin slice: **M1 ships at least one real guardrail** (a per-namespace resource quota plus a policy check that actually rejects a bad request) — not just the word on a slide.

## Demo

<!-- TODO(M1): drop a GIF/screenshot here the moment the agent summarizes its first log. This is the difference between "plan" and "real." -->

_Coming with Milestone 1 — a screenshot of the agent summarizing a failed pod's logs._

## Quickstart

<!-- TODO(M1): fill in as soon as control-plane + agent run locally. -->

```bash
# Coming with Milestone 1
```

## License

Released under the [MIT License](LICENSE).

## Author

**Mahesh Sadupalli** — building toward an infrastructure / observability platform engineering role.

---

*This is a living build log. Progress, learnings, and real measured numbers are filled in as each milestone ships.*
