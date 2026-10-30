package main

import "testing"

func TestDiffLines(t *testing.T) {
	if got := diffLines(" 12 files changed, 480 insertions(+), 75 deletions(-)"); got != 555 {
		t.Fatalf("got %d, want 555", got)
	}
	if got := diffLines(" 1 file changed, 1 insertion(+)"); got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

func TestEnforceDeployRules(t *testing.T) {
	failed := "smoke: ...\nresult: FAILED, rolling back to x"
	r := enforceDeployRules(DeployReview{Risk: "low", Rollback: false}, failed, "")
	if !r.Rollback || r.Risk != "high" {
		t.Fatalf("failed deploy must force rollback+high, got %+v", r)
	}
	r = enforceDeployRules(DeployReview{Risk: "low"}, "result: SUCCESS", " 3 files changed, 900 insertions(+)")
	if r.Risk != "medium" || r.Rollback {
		t.Fatalf("large diff must be >= medium without rollback, got %+v", r)
	}
	r = enforceDeployRules(DeployReview{Risk: "low"}, "result: SUCCESS", " 1 file changed, 3 insertions(+)")
	if r.Risk != "low" {
		t.Fatalf("small clean deploy stays low, got %+v", r)
	}
}
