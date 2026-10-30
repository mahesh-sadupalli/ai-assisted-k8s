package main

import "testing"

func TestParseDiagnosis(t *testing.T) {
	d, err := parseDiagnosis(`{"severity":"high","what_broke":"pod restarts","likely_cause":"OOM","suggested_action":"raise memory limit"}`)
	if err != nil {
		t.Fatal(err)
	}
	if d.Severity != "high" || d.LikelyCause != "OOM" || !d.Pages() {
		t.Fatalf("got %+v", d)
	}
	if _, err := parseDiagnosis(`not json`); err == nil {
		t.Fatal("want error on bad JSON")
	}
}

func TestPages(t *testing.T) {
	for sev, want := range map[string]bool{"low": false, "medium": false, "high": true, "critical": true} {
		if got := (Diagnosis{Severity: sev}).Pages(); got != want {
			t.Errorf("%s: Pages()=%v, want %v", sev, got, want)
		}
	}
}
