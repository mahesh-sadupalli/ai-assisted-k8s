package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPromQuery(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" || r.URL.Query().Get("query") != "up" {
			t.Errorf("unexpected request %s", r.URL)
		}
		w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"job":"control-plane"},"value":[1730000000,"1"]}]}}`))
	}))
	defer ts.Close()
	out, err := PromQuery(context.Background(), ts.URL, "up")
	if err != nil || !strings.Contains(out, "control-plane") || !strings.Contains(out, "=> 1") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestLokiQuery(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "5" || r.URL.Query().Get("since") != "15m0s" {
			t.Errorf("unexpected params %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"status":"success","data":{"result":[{"stream":{},"values":[["1","apply failed: forbidden"]]}]}}`))
	}))
	defer ts.Close()
	out, err := LokiQuery(context.Background(), ts.URL, `{app="control-plane"}`, 15*time.Minute, 5)
	if err != nil || !strings.Contains(out, "forbidden") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestPromQueryError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()
	if _, err := PromQuery(context.Background(), ts.URL, "up"); err == nil {
		t.Fatal("want error on 502")
	}
}
