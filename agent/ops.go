package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// serve runs the incident responder (in-cluster, or locally against your kubeconfig).
func serve(client anthropic.Client) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cs, err := kubeClient()
	if err != nil {
		fail("kubeconfig", err)
	}
	audit, err := OpenAuditLog(getenv("AUDIT_LOG", "audit.jsonl"))
	if err != nil {
		fail("audit log", err)
	}
	r := NewResponder(cs, client,
		getenv("PROMETHEUS_URL", "http://kps-kube-prometheus-stack-prometheus.monitoring:9090"),
		getenv("LOKI_URL", "http://loki.monitoring:3100"), audit, log)
	addr := getenv("ADDR", ":8080")
	log.Info("sentinel agent listening", "addr", addr, "audit_log", audit.path)
	if err := http.ListenAndServe(addr, r.Routes()); err != nil {
		fail("serve", err)
	}
}

// agentURL is where the human-side commands find a running `agent serve`
// (e.g. kubectl -n sentinel-system port-forward svc/sentinel-agent 9095:80).
func agentURL() string { return getenv("SENTINEL_AGENT_URL", "http://localhost:9095") }

func call(method, path string, body any) []byte {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequestWithContext(context.Background(), method, agentURL()+path, rd)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fail("agent API", err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		fail("agent API", fmt.Errorf("%s: %s", resp.Status, out))
	}
	return out
}

func printProposals() {
	var ps []Proposal
	json.Unmarshal(call(http.MethodGet, "/proposals", nil), &ps)
	if len(ps) == 0 {
		fmt.Println("no proposals")
	}
	for _, p := range ps {
		fmt.Printf("── %s  [%s]  %s\n", p.ID, p.Status, p.Alert)
		fmt.Printf("   What:   %s\n   Why:    %s\n   Impact: %s\n", p.Triage.What, p.Triage.Why, p.Triage.Impact)
		fmt.Printf("   Action: %s  (confidence %s)\n", p.Triage.Action, p.Triage.Confidence)
		if p.ValidationNote != "" {
			fmt.Printf("   Note:   %s\n", p.ValidationNote)
		}
		if p.DecidedBy != "" {
			fmt.Printf("   Decided by %s: %s\n", p.DecidedBy, p.Result)
		}
	}
}

func decideCmd(id, approver string, approve bool, reason string) {
	verb := "reject"
	if approve {
		verb = "approve"
	}
	var p Proposal
	json.Unmarshal(call(http.MethodPost, "/proposals/"+id+"/"+verb, map[string]string{"approver": approver, "reason": reason}), &p)
	fmt.Printf("%s: %s by %s → %s\n", p.ID, p.Status, p.DecidedBy, p.Result)
}

// verifyAuditFile checks a local copy of the audit log's hash chain.
func verifyAuditFile(path string) {
	events, err := ReadAudit(path)
	if err != nil {
		fail("audit", err)
	}
	for _, e := range events {
		fmt.Printf("%s  %-17s %-22s by %s\n", e.Time.Format("15:04:05"), e.Kind, e.ProposalID, e.Actor)
	}
	if bad := VerifyAudit(events); bad != -1 {
		fmt.Printf("TAMPERED: chain breaks at event %d\n", bad)
		os.Exit(1)
	}
	fmt.Printf("chain intact: %d events\n", len(events))
}
