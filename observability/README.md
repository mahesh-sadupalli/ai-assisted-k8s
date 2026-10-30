# observability

**Milestone 2 (thin) → Milestone 5 (deep) · LGTM stack**

What the AI agent runs on. Starts thin at M2 — Prometheus scraping RED metrics from the
control-plane, one Grafana dashboard. Deepens at M5 — an SLO with multi-window
multi-burn-rate alerting, Loki for centralized logs, and Grafana correlating metrics with
logs. Tempo (traces) is a stretch goal.

**M2 (done):** `prometheus/values.yaml` (kube-prometheus-stack, trimmed for kind),
`prometheus/servicemonitor.yaml` (scrapes the control-plane), `grafana/red-dashboard.json`
(rate, 5xx ratio, p50/p95/p99, guardrail outcomes). Install with `make observability`.
