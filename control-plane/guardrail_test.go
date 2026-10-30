package main

import (
	"errors"
	"strings"
	"testing"
)

func testGuardrail() Guardrail {
	return Guardrail{
		AllowedRegistries: []string{"ghcr.io/mahesh-sadupalli/"},
		DefaultQuota:      Quota{CPUMillis: 1000, MemoryMiB: 1024},
		Quotas:            map[string]Quota{"big": {CPUMillis: 8000, MemoryMiB: 8192}},
	}
}

func TestGuardrailRegistry(t *testing.T) {
	r := validRequest()
	r.Image = "docker.io/random/hello:latest"
	err := testGuardrail().Check(r, nil)
	var pe *PolicyError
	if !errors.As(err, &pe) || !strings.Contains(err.Error(), "allowed registry") {
		t.Fatalf("want registry PolicyError, got %v", err)
	}
}

func TestGuardrailQuota(t *testing.T) {
	g := testGuardrail()
	running := []Deployment{{DeploymentRequest: DeploymentRequest{
		Name: "other", Namespace: "team-a", Replicas: 2,
		Resources: Resources{CPUMillis: 300, MemoryMiB: 256},
	}}} // uses 600m / 512Mi of 1000m / 1024Mi

	tests := []struct {
		name    string
		mutate  func(*DeploymentRequest)
		wantErr string
	}{
		{"fits", func(r *DeploymentRequest) { r.Replicas = 1 }, ""},        // +250m
		{"cpu over", func(r *DeploymentRequest) { r.Replicas = 2 }, "CPU"}, // +500m -> 1100m
		{"mem over", func(r *DeploymentRequest) { r.Replicas = 1; r.Resources = Resources{CPUMillis: 100, MemoryMiB: 600} }, "memory"},
		{"other namespace not counted", func(r *DeploymentRequest) { r.Namespace = "team-b"; r.Replicas = 4 }, ""},
		{"per-namespace override", func(r *DeploymentRequest) { r.Namespace = "big"; r.Replicas = 10 }, ""},
		{"redeploy replaces itself", func(r *DeploymentRequest) { r.Name = "other"; r.Replicas = 4 }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validRequest()
			tt.mutate(&r)
			err := g.Check(r, running)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("want allowed, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
