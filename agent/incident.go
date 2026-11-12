package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Alert is one alert from an Alertmanager webhook payload.
type Alert struct {
	Status      string            `json:"status"` // firing | resolved
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
}

// Triage is the model's read of an incident plus at most one proposed action.
type Triage struct {
	What       string `json:"what"`
	Why        string `json:"why"`
	Impact     string `json:"impact"`
	Confidence string `json:"confidence"`
	Action     Action `json:"action"`
}

// Proposal is a triaged incident waiting for a human. Nothing changes in the cluster
// until someone other than the agent approves it.
type Proposal struct {
	ID             string    `json:"id"`
	Alert          string    `json:"alert"`
	Triage         Triage    `json:"triage"`
	ValidationNote string    `json:"validation_note,omitempty"` // set when the model's action was refused
	Status         string    `json:"status"`                    // pending | rejected | executed | failed
	DecidedBy      string    `json:"decided_by,omitempty"`
	Result         string    `json:"result,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// Responder turns alerts into audited, human-gated remediation proposals.
type Responder struct {
	cs      kubernetes.Interface
	triage  func(ctx context.Context, evidence string) (Triage, error)
	promURL string
	lokiURL string
	audit   *AuditLog
	log     *slog.Logger

	mu        sync.Mutex
	proposals map[string]*Proposal
	wg        sync.WaitGroup // in-flight triages, so tests can wait for them
}

func NewResponder(cs kubernetes.Interface, client anthropic.Client, promURL, lokiURL string, audit *AuditLog, log *slog.Logger) *Responder {
	r := &Responder{cs: cs, promURL: promURL, lokiURL: lokiURL, audit: audit, log: log, proposals: map[string]*Proposal{}}
	r.triage = func(ctx context.Context, evidence string) (Triage, error) {
		return triageWithClaude(ctx, client, evidence)
	}
	return r
}

var triageTool = anthropic.ToolParam{
	Name:        "report_triage",
	Description: anthropic.String("Report the incident triage and at most one proposed remediation. Call this exactly once."),
	Strict:      anthropic.Bool(true),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"what":       map[string]any{"type": "string", "description": "What is broken, in one or two sentences."},
			"why":        map[string]any{"type": "string", "description": "Root cause, citing the evidence (metric, log line, rollout)."},
			"impact":     map[string]any{"type": "string", "description": "User impact and error-budget impact."},
			"confidence": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
			"action": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":       map[string]any{"type": "string", "enum": allowedActions},
					"namespace":  map[string]any{"type": "string"},
					"deployment": map[string]any{"type": "string"},
					"replicas":   map[string]any{"type": "integer", "description": "Only for scale; 0 otherwise."},
				},
				"required":             []string{"type", "namespace", "deployment", "replicas"},
				"additionalProperties": false,
			},
		},
		Required:    []string{"what", "why", "impact", "confidence", "action"},
		ExtraFields: map[string]any{"additionalProperties": false},
	},
}

const triagePrompt = `You are Sentinel, the on-call SRE agent for a Kubernetes platform.
You receive an SLO alert and the evidence gathered for it: metrics, logs, pod state and rollout history.
Work out what is broken, why (cite the evidence), and the impact on users and the error budget.
Then propose at most one remediation from the allowed actions:
- rollout_undo: when a recent rollout lines up with the errors
- rollout_restart: when pods are wedged but the spec is fine
- scale: when the service is saturated
- none: when no allowed action would fix it; explain what a human should do in "why"
A human approves or rejects your proposal before anything runs. Always answer by calling report_triage.`

func triageWithClaude(ctx context.Context, client anthropic.Client, evidence string) (Triage, error) {
	raw, err := callTool(ctx, client, triagePrompt, triageTool, evidence)
	if err != nil {
		return Triage{}, err
	}
	var t Triage
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return Triage{}, fmt.Errorf("parse triage: %w", err)
	}
	return t, nil
}

// RolloutHistory lists a Deployment's revisions with what changed between them.
// Correlating "errors started" with "revision N shipped" is most of incident triage.
func RolloutHistory(ctx context.Context, cs kubernetes.Interface, ns, name string) (string, error) {
	d, err := cs.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	list, err := cs.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	var owned []appsv1.ReplicaSet
	for _, rs := range list.Items {
		if metav1.IsControlledBy(&rs, d) {
			owned = append(owned, rs)
		}
	}
	sort.Slice(owned, func(i, j int) bool { return revision(owned[i].ObjectMeta) < revision(owned[j].ObjectMeta) })
	var b strings.Builder
	fmt.Fprintf(&b, "Deployment %s/%s current revision %d\n", ns, name, revision(d.ObjectMeta))
	for _, rs := range owned {
		spec := rs.Spec.Template.Spec
		image := ""
		if len(spec.Containers) > 0 {
			image = spec.Containers[0].Image
		}
		fmt.Fprintf(&b, "  revision %d created %s: image=%s serviceAccount=%s ready=%d/%d\n",
			revision(rs.ObjectMeta), rs.CreationTimestamp.UTC().Format(time.RFC3339), image,
			spec.ServiceAccountName, rs.Status.ReadyReplicas, rs.Status.Replicas)
	}
	return b.String(), nil
}

// gather collects the evidence for one alert. A source that is down is noted, not fatal:
// partial evidence is better than no triage during an incident.
func (r *Responder) gather(ctx context.Context, a Alert) string {
	svc, ns := a.Labels["service"], a.Labels["namespace"]
	var b strings.Builder
	fmt.Fprintf(&b, "ALERT %s (%s) firing since %s\n%s\n%s\n\n",
		a.Labels["alertname"], a.Labels["severity"], a.StartsAt.UTC().Format(time.RFC3339),
		a.Annotations["summary"], a.Annotations["description"])

	section := func(title, body string, err error) {
		if err != nil {
			body = "(unavailable: " + err.Error() + ")"
		}
		fmt.Fprintf(&b, "== %s ==\n%s\n", title, strings.TrimSpace(body))
	}

	out, err := PromQuery(ctx, r.promURL, "sentinel:api_error_ratio:rate5m")
	section("Error ratio, last 5m (SLO allows 0.001)", out, err)
	out, err = PromQuery(ctx, r.promURL, fmt.Sprintf(`sum by (route, code) (rate(http_requests_total{job=%q}[5m]))`, svc))
	section("Request rate by route and status code", out, err)
	out, err = LokiQuery(ctx, r.lokiURL, fmt.Sprintf(`{namespace=%q, app=%q} |~ "(?i)(error|fail|forbidden|denied)"`, ns, svc), 15*time.Minute, 20)
	section("Error logs, last 15m (Loki)", out, err)
	out, err = RolloutHistory(ctx, r.cs, ns, svc)
	section("Rollout history", out, err)

	pods, err := r.cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: "app=" + svc})
	if err != nil {
		section("Pods", "", err)
	} else {
		for i, p := range pods.Items {
			if i == 2 {
				break
			}
			out, err := PodEvidence(ctx, r.cs, ns, p.Name)
			section("Pod "+p.Name, out, err)
		}
	}
	return b.String()
}

func (r *Responder) hasPending(alertname string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.proposals {
		if p.Alert == alertname && p.Status == "pending" {
			return true
		}
	}
	return false
}

func (r *Responder) respond(ctx context.Context, a Alert) {
	name := a.Labels["alertname"]
	r.audit.Record("alert_received", "", "agent", a)
	evidence := r.gather(ctx, a)

	t, err := r.triage(ctx, evidence)
	if err != nil {
		r.log.Error("triage failed", "alert", name, "err", err)
		r.audit.Record("triage_failed", "", "agent", map[string]string{"alert": name, "error": err.Error()})
		return
	}
	p := &Proposal{ID: newProposalID(), Alert: name, Triage: t, Status: "pending", CreatedAt: time.Now().UTC()}
	// The model proposes; the allowlist decides. A refused action degrades to "none".
	if err := t.Action.Validate(ctx, r.cs); err != nil {
		p.ValidationNote = "proposed action refused: " + err.Error()
		p.Triage.Action = Action{Type: "none"}
	}
	r.mu.Lock()
	r.proposals[p.ID] = p
	r.mu.Unlock()
	r.audit.Record("triage", p.ID, "agent", map[string]any{"evidence": evidence, "triage": t})
	r.audit.Record("proposal", p.ID, "agent", map[string]any{"action": p.Triage.Action, "validation_note": p.ValidationNote})
	r.log.Info("proposal pending approval", "id", p.ID, "alert", name, "action", p.Triage.Action.String())
}

func newProposalID() string { return "p-" + time.Now().UTC().Format("20060102-150405.000") }

// Decide applies a human's approve/reject. The agent can never approve its own proposal.
func (r *Responder) Decide(ctx context.Context, id, approver string, approve bool, reason string) (*Proposal, error) {
	if approver == "" || approver == "agent" {
		return nil, fmt.Errorf("a named human approver is required")
	}
	r.mu.Lock()
	p, ok := r.proposals[id]
	if !ok {
		r.mu.Unlock()
		return nil, fmt.Errorf("proposal %s not found", id)
	}
	if p.Status != "pending" {
		r.mu.Unlock()
		return nil, fmt.Errorf("proposal %s is already %s", id, p.Status)
	}
	p.DecidedBy = approver
	p.Status = "deciding" // blocks a second concurrent decision
	r.mu.Unlock()

	if !approve {
		r.audit.Record("rejected", id, approver, map[string]string{"reason": reason})
		r.setStatus(p, "rejected", reason)
		return p, nil
	}
	r.audit.Record("approved", id, approver, map[string]any{"action": p.Triage.Action})
	result, err := p.Triage.Action.Execute(ctx, r.cs)
	if err != nil {
		r.audit.Record("execution_failed", id, "agent", map[string]string{"error": err.Error()})
		r.setStatus(p, "failed", err.Error())
		return p, err
	}
	r.audit.Record("executed", id, "agent", map[string]string{"result": result})
	r.setStatus(p, "executed", result)
	return p, nil
}

func (r *Responder) setStatus(p *Proposal, status, result string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.Status, p.Result = status, result
}

func (r *Responder) Proposals() []Proposal {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Proposal, 0, len(r.proposals))
	for _, p := range r.proposals {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (r *Responder) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	// Alertmanager webhook. Triage takes longer than Alertmanager waits, so answer 202 and work async.
	mux.HandleFunc("POST /alerts", func(w http.ResponseWriter, req *http.Request) {
		var payload struct {
			Alerts []Alert `json:"alerts"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, a := range payload.Alerts {
			name := a.Labels["alertname"]
			switch {
			case a.Status == "resolved":
				r.audit.Record("alert_resolved", "", "agent", map[string]string{"alert": name})
			case r.hasPending(name):
				r.log.Info("alert already has a pending proposal", "alert", name)
			default:
				r.wg.Add(1)
				go func() {
					defer r.wg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
					defer cancel()
					r.respond(ctx, a)
				}()
			}
		}
		w.WriteHeader(http.StatusAccepted)
	})

	mux.HandleFunc("GET /proposals", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, r.Proposals())
	})

	decide := func(approve bool) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			var body struct {
				Approver string `json:"approver"`
				Reason   string `json:"reason"`
			}
			json.NewDecoder(req.Body).Decode(&body)
			p, err := r.Decide(req.Context(), req.PathValue("id"), body.Approver, approve, body.Reason)
			if err != nil && p == nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, p)
		}
	}
	mux.HandleFunc("POST /proposals/{id}/approve", decide(true))
	mux.HandleFunc("POST /proposals/{id}/reject", decide(false))

	mux.HandleFunc("GET /audit", func(w http.ResponseWriter, _ *http.Request) {
		events, err := ReadAudit(r.audit.path)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"events": []AuditEvent{}, "intact": true})
			return
		}
		bad := VerifyAudit(events)
		writeJSON(w, http.StatusOK, map[string]any{"events": events, "intact": bad == -1, "first_bad": bad})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}
