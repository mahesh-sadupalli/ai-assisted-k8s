package main

import (
	"strings"
	"testing"
)

func validRequest() DeploymentRequest {
	return DeploymentRequest{
		Name:      "hello",
		Namespace: "team-a",
		Image:     "ghcr.io/mahesh-sadupalli/hello:1.0",
		Replicas:  2,
		Resources: Resources{CPUMillis: 250, MemoryMiB: 256},
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*DeploymentRequest)
		wantErr string // empty = valid
	}{
		{"valid", func(r *DeploymentRequest) {}, ""},
		{"uppercase name", func(r *DeploymentRequest) { r.Name = "Hello" }, "DNS label"},
		{"empty namespace", func(r *DeploymentRequest) { r.Namespace = "" }, "namespace"},
		{"missing image", func(r *DeploymentRequest) { r.Image = " " }, "image is required"},
		{"zero replicas", func(r *DeploymentRequest) { r.Replicas = 0 }, "replicas"},
		{"too many replicas", func(r *DeploymentRequest) { r.Replicas = 11 }, "replicas"},
		{"missing limits", func(r *DeploymentRequest) { r.Resources = Resources{} }, "resource limits are required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validRequest()
			tt.mutate(&r)
			err := r.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("want valid, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
