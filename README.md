<div align="center">

# Self-Service Kubernetes Platform with AI-Assisted Observability & Reliability

`codename: Sentinel`

**A self-service Kubernetes platform where reliability is observable end-to-end — and an AI agent turns metrics and logs into triaged, auditable fixes.**

![Status](https://img.shields.io/badge/status-Milestone%201%20in%20progress-orange)
![License](https://img.shields.io/badge/license-MIT-blue)
![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white)
![Kubernetes](https://img.shields.io/badge/Kubernetes-326CE5?logo=kubernetes&logoColor=white)
![AWS](https://img.shields.io/badge/AWS-EKS-232F3E?logo=amazonaws&logoColor=white)
![AI Agent](https://img.shields.io/badge/AI%20agent-Claude%20API-8A63D2)

</div>

---

## Table of contents

- [Overview](#overview)
- [Three pillars](#three-pillars)
- [Architecture](#architecture)
- [The AI thread](#the-ai-thread-the-spine-from-day-1)
- [Milestones](#milestones)
- [Tech stack](#tech-stack)
- [Repository layout](#repository-layout)
- [Working approach](#working-approach)
- [Scope](#scope)
- [License](#license)

---

## Overview

Sentinel is a self-service deployment platform on Kubernetes (AWS EKS). A **Go control-plane API** lets a developer ship a service with a single request; an **LGTM-stack observability layer** (Loki, Grafana, Tempo, Prometheus) makes every service's health visible; and an **AI reliability agent** reads those metrics and logs to triage incidents and propose safe, auditable remediations.

The project is deliberately built as **one system rather than five disconnected tutorials.** Each milestone bolts a real layer onto the same platform, and the AI agent runs through all of them as the spine — not a final-quarter add-on.

## Three pillars

Every milestone advances three pillars **together**, and the AI is the one that ties them into a single system:

- **AI** *(central)* — an agent, built on the Claude API, that reasons over the platform: reads logs and metrics, reviews infrastructure changes, and proposes auditable remediations. This is the through-line; each milestone is measured as much by **what the agent can now reason about and act on** as by the infrastructure beneath it.
- **Cloud** — AWS, EKS, Terraform, networking, ingress, and observability: the environment the platform runs in.
- **Software** — idiomatic Go services, API design, testing, and CI/CD: the machinery the platform is made of.

The agent is not a feature bolted on at the end — it is the reason the cloud and software layers exist in one place. As an AI engineer, I treat every milestone as *"ship real infrastructure **and** extend what the AI can do with it."*

## Architecture

```
                        Developer
                   (deploy via CLI / API)
                            │
                ┌───────────▼───────────┐
                │   Control-plane API    │  ◄── Go · Software
                │ provisioning·guardrails│
                └───────────┬───────────┘
                            │
   Terraform ──────►  Kubernetes / EKS  ◄────── CI/CD
   (IaC · Cloud)      (the runtime · Cloud)   (Software)
                            │
                Observability (LGTM stack)     ◄── Cloud
             Prometheus·Grafana·Loki·SLOs
                            │
             ┌──────────────▼──────────────┐
             │      AI RELIABILITY AGENT    │  ◄── AI · the spine, every milestone
             │ triage → suggest → remediate │
             └──────────────────────────────┘
       reads metrics + logs · reviews plans/PRs · auditable
```

## The AI thread (the spine, from Day 1)

The AI agent is the **central focus of every milestone.** It grows **monotonically** — each stage does something the previous stage could not. This table is the single source of truth for what the agent can do at each milestone.

| Stage | The agent can now… |
|-------|--------------------|
| **M1** | Summarize a **log snippet handed to it** — no cluster yet, pure agent mechanics |
| **M2** | Pull logs/state **from the cluster itself** (and query logs via Loki that it did not collect by hand) |
| **M3** | Read **infra/cloud context** and review a **Terraform plan** before merge |
| **M4** | Watch **CI/CD**, flag a risky deploy, and suggest a rollback |
| **M5** | Close the **full reliability loop** — from a Prometheus alert to a proposed, auditable remediation |

Built on the **Claude API directly** (function calling / tool use) so the mechanics are understood before wrapping any framework around it. Every agent action is **auditable** — proposals are logged, and any state-changing remediation is gated behind a human-approve step.

## Milestones

Dependency-ordered so the infrastructure builds on itself — Go control-plane → containers/K8s → EKS → CI/CD → observability depth. But **the AI is the constant:** every milestone below ships a real cloud/software layer *and* extends the agent. Observability starts thin at M2 and deepens at M5 so the repo demonstrates it early, not only at the end.

Each milestone lists a **goal**, **what gets built**, the **AI focus**, the **skills demonstrated**, and **"done when"** — concrete, verifiable acceptance criteria.

---

### M1 · Go control-plane API + first agent
**Bucket:** Systems programming (Go) · **Pillars:** Software + AI

- **Goal:** Ship the software foundation — a Go HTTP control-plane that validates deploy requests against a real guardrail — and, the same day, the AI foundation: an agent that reasons over a log snippet to explain what broke. Software and AI, together from Day 1.
- **What gets built:**
  - Go HTTP API with `POST /deployments` and `GET /deployments/{id}`
  - Request validation and **one real guardrail** — e.g. a per-namespace resource quota check plus a policy rule that actually rejects a bad request (missing resource limits, or a disallowed image registry)
  - A simple persistence layer (SQLite / in-memory) for deployment intents, plus structured logging
  - `agent/` v0: takes a log snippet and returns a structured summary (*what broke → likely cause → suggested next step*) via Claude API tool use
  - Unit tests for the guardrail and validation logic
- **AI focus:** Establish the agent's reasoning core — structured tool-use against a log snippet handed to it. This is the seed every later milestone grows from.
- **Skills demonstrated:** Claude API function calling and agent design · Go HTTP services, idiomatic layout, testing · API design and policy/validation.
- **Done when:** an invalid request is rejected by the guardrail with a clear error; a valid one is accepted and retrievable; `agent summarize <logfile>` returns a structured summary; unit tests pass.

---

### M2 · Containers + Kubernetes + thin observability slice
**Bucket:** IaC & containers · **Pillars:** Cloud + AI

- **Goal:** Take the platform onto Kubernetes and give the agent its first live senses — it stops being handed logs and starts pulling them from the cluster itself. The cloud runtime and the agent's awareness advance together.
- **What gets built:**
  - Multi-stage Dockerfiles (small images) for the control-plane and a sample "hello" service
  - Kubernetes manifests: `Deployment`, `Service`, `ConfigMap`, and a `Namespace` with a `ResourceQuota` (wired to the M1 guardrail)
  - A local cluster via **kind**; the control-plane applies manifests to actually deploy the sample service (thin self-service loop)
  - **Prometheus** scraping `/metrics` from the control-plane, instrumented on the **RED method** (request rate, errors, duration histogram) via `client_golang`
  - **One Grafana dashboard** (RED for the control-plane)
  - `agent/` v1: reads pod logs from the cluster via `client-go` (and, if Loki is deployed, queries logs it did not collect by hand)
- **AI focus:** Give the agent real-world input — connect it to live cluster state so its reasoning runs on actual telemetry, not a pasted snippet.
- **Skills demonstrated:** Agent + `client-go` / cluster integration · Docker multi-stage builds, core Kubernetes objects · Prometheus instrumentation, Grafana dashboards.
- **Done when:** `kubectl get pods` shows the control-plane and sample service running on kind; the control-plane can deploy the sample service into a namespace; Prometheus scrapes its metrics; one Grafana RED dashboard renders; the agent summarizes a real pod's logs pulled from the cluster.

---

### M3 · AWS EKS + networking / ingress
**Bucket:** AWS platform depth · **Pillars:** Cloud + AI

- **Goal:** Run on real AWS (EKS via Terraform, fronted by ingress) and let the agent reason about cloud infrastructure itself — reviewing a `terraform plan` before it merges. Cloud depth and AI judgment advance together.
- **What gets built:**
  - **Terraform**: VPC, EKS cluster, managed node group, least-privilege IAM roles, and an ECR repository
  - Control-plane + sample service deployed to EKS; images pushed to ECR
  - **Ingress** (AWS Load Balancer Controller or ingress-nginx) exposing the sample service over a real URL
  - `agent/` v2: reads infra/cloud context and reviews a `terraform plan`, flagging risky changes (e.g. destroy of a stateful resource, an over-permissive security group)
- **AI focus:** Move the agent from runtime telemetry to *infrastructure intent* — it now reasons about change safety, the foundation of guardrails and auditability.
- **Skills demonstrated:** Agent reasoning over Terraform plans (change-safety analysis) · AWS (EKS, VPC, IAM, ECR) · Terraform (modules, remote state), Kubernetes networking/ingress.
- **Done when:** `terraform apply` stands up an EKS cluster from scratch; the sample service is reachable via its ingress URL; images live in ECR; the agent parses a `terraform plan` and flags at least one class of risky change. *(A `terraform destroy` teardown path is documented — EKS incurs cost.)*

---

### M4 · CI/CD with GitHub Actions
**Bucket:** CI/CD & automation · **Pillars:** Software + AI

- **Goal:** Automate delivery and put the agent in the deploy path — watching pipelines, flagging risky releases, and suggesting rollbacks. Software delivery and AI oversight advance together.
- **What gets built:**
  - **GitHub Actions** pipeline: lint + test Go → build + push image to ECR → deploy to EKS, triggered on merge to `main`, authenticating to AWS via OIDC (no long-lived keys)
  - Test gates (unit tests must pass to deploy) and a post-deploy smoke test
  - A **rollback trigger** — a failing smoke test rolls the deployment back
  - `agent/` v3: watches a workflow/deploy, flags a risky deploy (large diff, failing smoke test) and suggests a rollback
- **AI focus:** Put the agent in the release loop — reasoning about deploy risk in real time, the bridge between passive analysis and active reliability.
- **Skills demonstrated:** Agent-in-the-loop deploy-risk analysis · GitHub Actions, GitOps-style delivery · deployment safety (smoke test + rollback), OIDC cloud auth.
- **Done when:** a PR merge drives the pipeline end-to-end (test → build → push → deploy); a failing test blocks the deploy; a failing smoke test triggers an automatic rollback; the agent posts a summary/flag on the run.

---

### M5 · Observability depth + reliability-agent capstone
**Bucket:** Prometheus / Grafana / Loki / SLOs · **Pillars:** Cloud + AI *(capstone)*

- **Goal:** Deepen observability into SLOs, burn-rate alerting, and centralized logs, then close the AI reliability loop — the agent goes from an alert to an auditable remediation. This is where every pillar converges on the agent.
- **What gets built:**
  - A defined **SLO** (e.g. 99.9% availability / latency target) with **multi-window, multi-burn-rate alerts** in Prometheus + Alertmanager
  - **Loki** for centralized logs, with Grafana dashboards correlating metrics and logs *(Tempo/traces = stretch)*
  - `agent/` v4 — the capstone: receives a Prometheus alert via webhook, pulls the relevant metrics and Loki logs, triages (*what / why / impact*), and proposes an **auditable remediation** gated behind a human-approve step
  - An **audit log** of every agent proposal and decision
- **AI focus:** Close the loop — the agent becomes the reliability layer, turning an alert into a triaged, auditable, human-gated remediation. The whole platform exists to make this possible.
- **Skills demonstrated:** Safe agentic remediation (guardrails / approval / audit), incident-response modeling · SLOs and error budgets, burn-rate alerting, Alertmanager · Loki, metric/log correlation.
- **Done when:** an injected failure fires an SLO burn-rate alert; the alert reaches the agent; the agent produces a triage plus a proposed remediation with a full audit trail; a human-approve step gates any state change; the capstone demo is recorded.

---

## Tech stack

`Agentic LLM workflows (Claude API)` · `Go` · `Kubernetes (EKS)` · `Prometheus` · `Grafana` · `Loki` · `Terraform` · `GitHub Actions` · `SLOs & burn-rate alerting` · `AWS (VPC, IAM, ECR)` · `Docker`

_Stretch: `Tempo` (distributed traces) to complete the LGTM picture, if time allows._

## Repository layout

```
agent/           AI reliability agent — the spine, grows M1 → M5
control-plane/   M1 · Go control-plane API
infra/           M2–M3 · Terraform + EKS
observability/   M2 thin → M5 deep · Prometheus + Grafana + Loki + SLOs
.github/         M4 · CI/CD workflows
docs/            Roadmap + living build log
```

## Working approach

A focused solo sprint. The workflow is **concept-first each day** (understand the concept), **then hands-on** (build it), **then notes** (record what was actually built and learned). AI-agentic work leads from Day 1 — the agent is the first thing built and the last thing deepened. The day-by-day breakdown lives in `docs/`.

## Scope

A **thin but complete vertical slice** — one real service flowing through the whole platform — not a production-grade internal developer platform. This is the right size for solo work while still exercising every skill.

"Guardrails" means something concrete even in the thin slice: **M1 ships at least one real guardrail** (a per-namespace resource quota plus a policy check that actually rejects a bad request) — not just the word on a slide.

## License

Released under the [MIT License](LICENSE).
