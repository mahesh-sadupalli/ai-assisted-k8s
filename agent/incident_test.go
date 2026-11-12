package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const alertBody = `{"alerts":[{"status":"firing","labels":{"alertname":"SentinelAPIErrorBudgetBurnFast",
  "severity":"page","service":"control-plane","namespace":"sentinel-system"},
  "annotations":{"summary":"burning budget"},"startsAt":"2026-10-30T07:00:00Z"}]}`

// newTestResponder wires a Responder to the brokenRollout cluster, empty telemetry
// backends, and a fake model that returns the given action.
func newTestResponder(t *testing.T, action Action) (*Responder, *httptest.Server, *string) {
	t.Helper()
	telemetry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"result":[]}}`))
	}))
	t.Cleanup(telemetry.Close)
	audit, err := OpenAuditLog(filepath.Join(t.TempDir(), "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	r := &Responder{
		cs: brokenRollout(), promURL: telemetry.URL, lokiURL: telemetry.URL, audit: audit,
		log: slog.New(slog.NewTextHandler(io.Discard, nil)), proposals: map[string]*Proposal{},
	}
	var seen string
	r.triage = func(_ context.Context, evidence string) (Triage, error) {
		seen = evidence
		return Triage{What: "502s", Why: "revision 2 uses the default ServiceAccount", Confidence: "high", Action: action}, nil
	}
	ts := httptest.NewServer(r.Routes())
	t.Cleanup(ts.Close)
	return r, ts, &seen
}

func postJSON(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestAlertToApprovedRollback(t *testing.T) {
	undo := Action{Type: "rollout_undo", Namespace: "sentinel-system", Deployment: "control-plane"}
	r, ts, evidence := newTestResponder(t, undo)

	if resp := postJSON(t, ts.URL+"/alerts", alertBody); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("webhook status %d", resp.StatusCode)
	}
	r.wg.Wait()
	if !strings.Contains(*evidence, "revision 2") || !strings.Contains(*evidence, "serviceAccount=default") {
		t.Fatalf("evidence should include rollout history:\n%s", *evidence)
	}

	props := r.Proposals()
	if len(props) != 1 || props[0].Status != "pending" {
		t.Fatalf("want 1 pending proposal, got %+v", props)
	}
	id := props[0].ID

	// Nothing has changed yet: the proposal waits for a human.
	d, _ := r.cs.AppsV1().Deployments("sentinel-system").Get(context.Background(), "control-plane", metav1.GetOptions{})
	if d.Spec.Template.Spec.ServiceAccountName != "default" {
		t.Fatal("cluster changed before approval")
	}
	// The agent can't approve itself, and an approver is required.
	for _, body := range []string{`{}`, `{"approver":"agent"}`} {
		if resp := postJSON(t, ts.URL+"/proposals/"+id+"/approve", body); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("approve %s: want 400, got %d", body, resp.StatusCode)
		}
	}

	resp := postJSON(t, ts.URL+"/proposals/"+id+"/approve", `{"approver":"mahesh"}`)
	var p Proposal
	json.NewDecoder(resp.Body).Decode(&p)
	if p.Status != "executed" || p.DecidedBy != "mahesh" {
		t.Fatalf("got %+v", p)
	}
	d, _ = r.cs.AppsV1().Deployments("sentinel-system").Get(context.Background(), "control-plane", metav1.GetOptions{})
	if d.Spec.Template.Spec.ServiceAccountName != "control-plane" {
		t.Fatal("rollback did not restore the ServiceAccount")
	}
	if resp := postJSON(t, ts.URL+"/proposals/"+id+"/approve", `{"approver":"mahesh"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatal("a decided proposal must not run twice")
	}

	events, _ := ReadAudit(r.audit.path)
	var kinds []string
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}
	want := "alert_received,triage,proposal,approved,executed"
	if strings.Join(kinds, ",") != want || VerifyAudit(events) != -1 {
		t.Fatalf("audit trail %v (intact=%v), want %s", kinds, VerifyAudit(events) == -1, want)
	}
}

func TestDisallowedActionBecomesNone(t *testing.T) {
	r, ts, _ := newTestResponder(t, Action{Type: "rollout_restart", Namespace: "kube-system", Deployment: "coredns"})
	postJSON(t, ts.URL+"/alerts", alertBody)
	r.wg.Wait()
	p := r.Proposals()[0]
	if p.Triage.Action.Type != "none" || !strings.Contains(p.ValidationNote, "not allowlisted") {
		t.Fatalf("got action=%v note=%q", p.Triage.Action, p.ValidationNote)
	}
}

func TestRejectAndDedupe(t *testing.T) {
	r, ts, _ := newTestResponder(t, Action{Type: "none"})
	postJSON(t, ts.URL+"/alerts", alertBody)
	r.wg.Wait()
	postJSON(t, ts.URL+"/alerts", alertBody) // repeat notification while pending
	r.wg.Wait()
	if n := len(r.Proposals()); n != 1 {
		t.Fatalf("repeat alert must not create a second proposal, got %d", n)
	}
	id := r.Proposals()[0].ID
	resp := postJSON(t, ts.URL+"/proposals/"+id+"/reject", `{"approver":"mahesh","reason":"fixing by hand"}`)
	var p Proposal
	json.NewDecoder(resp.Body).Decode(&p)
	if p.Status != "rejected" || p.Result != "fixing by hand" {
		t.Fatalf("got %+v", p)
	}
}
