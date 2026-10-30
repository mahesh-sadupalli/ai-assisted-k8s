# control-plane

**Milestone 1 · Go**

The Go HTTP control-plane API, the front door of the platform. It accepts "deploy this
service" requests, validates them, runs them through the guardrail, and stores the intent.

| File | What it does |
|---|---|
| `deployment.go` | Request type + validation (DNS-label names, replicas 1–10, **resource limits required**) |
| `guardrail.go` | Policy: **image registry allowlist** + **per-namespace CPU/memory quota** |
| `store.go` | In-memory store; quota check and write happen under one lock |
| `server.go` | `POST /deployments`, `GET /deployments/{id}`, `GET /healthz`, `slog` JSON logs |

```bash
go run ./control-plane                      # :8080, ALLOWED_REGISTRIES=ghcr.io/mahesh-sadupalli/
curl -XPOST localhost:8080/deployments -d '{"name":"hello","namespace":"team-a",
  "image":"ghcr.io/mahesh-sadupalli/hello:1.0","replicas":2,
  "resources":{"cpu_millis":250,"memory_mib":256}}'     # 201 + id
curl localhost:8080/deployments/<id>                    # 200
```

Status codes: `400` malformed/invalid · `403` rejected by guardrail · `404` unknown id.
