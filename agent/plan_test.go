package main

import (
	"os"
	"strings"
	"testing"
)

func loadPlan(t *testing.T, path string) Plan {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParsePlan(data)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScanRiskyPlan(t *testing.T) {
	got := map[string]string{}
	for _, f := range Scan(loadPlan(t, "testdata/plan-risky.json")) {
		got[f.Address] = f.Rule
	}
	want := map[string]string{
		"aws_db_instance.orders":      "stateful-destroy",
		"aws_security_group_rule.ssh": "open-ingress",
		"aws_iam_policy.ci":           "iam-wildcard",
	}
	for addr, rule := range want {
		if got[addr] != rule {
			t.Errorf("%s: want rule %q, got %q", addr, rule, got[addr])
		}
	}
	if _, ok := got["module.eks.aws_eks_node_group.default"]; ok {
		t.Error("scaling a node group is not risky")
	}
}

func TestScanSafePlan(t *testing.T) {
	if f := Scan(loadPlan(t, "testdata/plan-safe.json")); len(f) != 0 {
		t.Fatalf("want no findings, got %+v", f)
	}
}

func TestOpenIngressAllowsWeb(t *testing.T) {
	rc := ResourceChange{Type: "aws_security_group_rule"}
	rc.Change.After = map[string]any{"type": "ingress", "from_port": 443.0, "to_port": 443.0, "cidr_blocks": []any{"0.0.0.0/0"}}
	if openToWorld(rc) {
		t.Fatal("443 from the world is normal for a public LB")
	}
}

func TestSummarySkipsNoop(t *testing.T) {
	s := loadPlan(t, "testdata/plan-safe.json").Summary()
	if strings.Contains(s, "aws_s3_bucket.logs") || !strings.Contains(s, "[update]") {
		t.Fatalf("unexpected summary:\n%s", s)
	}
}

func TestEnforceFindings(t *testing.T) {
	r := enforceFindings(PlanReview{Verdict: "approve", Findings: []Finding{{Rule: "iam-wildcard"}}})
	if r.Verdict != "needs_review" {
		t.Fatalf("rule hit must downgrade approve, got %s", r.Verdict)
	}
	if r := enforceFindings(PlanReview{Verdict: "block"}); r.Verdict != "block" {
		t.Fatalf("block must stay block, got %s", r.Verdict)
	}
}
