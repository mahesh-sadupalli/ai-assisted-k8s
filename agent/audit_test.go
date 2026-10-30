package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditChain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	l, err := OpenAuditLog(path)
	if err != nil {
		t.Fatal(err)
	}
	l.Record("alert_received", "", "agent", map[string]string{"alert": "BurnFast"})
	l.Record("proposal", "p1", "agent", map[string]string{"action": "rollout_undo"})

	// Reopening continues the same chain.
	l2, _ := OpenAuditLog(path)
	l2.Record("approved", "p1", "mahesh", nil)

	events, err := ReadAudit(path)
	if err != nil || len(events) != 3 {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
	if bad := VerifyAudit(events); bad != -1 {
		t.Fatalf("intact log reported bad at %d", bad)
	}

	// Tamper: change who approved it.
	data, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(data), `"actor":"mahesh"`, `"actor":"someone"`, 1)), 0o644)
	events, _ = ReadAudit(path)
	if bad := VerifyAudit(events); bad != 2 {
		t.Fatalf("tampered line should be caught at index 2, got %d", bad)
	}
}
