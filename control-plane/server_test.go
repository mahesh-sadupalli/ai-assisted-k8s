package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeApplier struct{ err error }

func (f fakeApplier) Apply(context.Context, Deployment) error { return f.err }

func newTestServerWith(a Applier) *httptest.Server {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httptest.NewServer(NewServer(NewStore(), testGuardrail(), a, log).Routes())
}

func newTestServer() *httptest.Server { return newTestServerWith(nil) }

func post(t *testing.T, ts *httptest.Server, body string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := http.Post(ts.URL+"/deployments", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

const goodBody = `{"name":"hello","namespace":"team-a","image":"ghcr.io/mahesh-sadupalli/hello:1.0",
	"replicas":2,"resources":{"cpu_millis":250,"memory_mib":256}}`

func TestCreateAndGet(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp, out := post(t, ts, goodBody)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, body %v", resp.StatusCode, out)
	}
	id, _ := out["id"].(string)

	get, err := http.Get(ts.URL + "/deployments/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	var d Deployment
	json.NewDecoder(get.Body).Decode(&d)
	if get.StatusCode != http.StatusOK || d.Name != "hello" || d.Status != "accepted" {
		t.Fatalf("get: status %d, %+v", get.StatusCode, d)
	}
}

func TestCreateRejections(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status int
		errHas string
	}{
		{"bad json", `{`, 400, "invalid JSON"},
		{"unknown field", `{"nme":"x"}`, 400, "unknown field"},
		{"missing limits", `{"name":"hello","namespace":"team-a","image":"ghcr.io/mahesh-sadupalli/hello:1.0","replicas":1}`, 400, "resource limits"},
		{"bad registry", strings.Replace(goodBody, "ghcr.io/mahesh-sadupalli", "docker.io/evil", 1), 403, "allowed registry"},
		{"over quota", strings.Replace(goodBody, `"replicas":2`, `"replicas":5`, 1), 403, "quota exceeded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer()
			defer ts.Close()
			resp, out := post(t, ts, tt.body)
			msg, _ := out["error"].(string)
			if resp.StatusCode != tt.status || !strings.Contains(msg, tt.errHas) {
				t.Fatalf("want %d with %q, got %d %q", tt.status, tt.errHas, resp.StatusCode, msg)
			}
		})
	}
}

func TestGetMissing(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/deployments/nope")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestCreateApplies(t *testing.T) {
	tests := []struct {
		name       string
		applier    Applier
		wantCode   int
		wantStatus string
	}{
		{"applied", fakeApplier{}, http.StatusCreated, "applied"},
		{"apply fails", fakeApplier{err: errors.New("cluster down")}, http.StatusBadGateway, "apply_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServerWith(tt.applier)
			defer ts.Close()
			resp, out := post(t, ts, goodBody)
			if resp.StatusCode != tt.wantCode || out["status"] != tt.wantStatus {
				t.Fatalf("want %d/%s, got %d/%v", tt.wantCode, tt.wantStatus, resp.StatusCode, out["status"])
			}
		})
	}
}

func TestMetrics(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	post(t, ts, goodBody)
	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	want := `http_requests_total{code="201",method="POST",route="POST /deployments"}`
	if !strings.Contains(string(body), want) {
		t.Fatalf("metrics missing %s", want)
	}
}
