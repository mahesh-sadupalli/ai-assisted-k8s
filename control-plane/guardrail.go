package main

import (
	"fmt"
	"strings"
)

// Quota is the total CPU/memory all deployments in one namespace may claim.
type Quota struct {
	CPUMillis int
	MemoryMiB int
}

// Guardrail is the policy every deployment must pass before it is accepted.
type Guardrail struct {
	AllowedRegistries []string // image must start with one of these, e.g. "ghcr.io/mahesh-sadupalli/"
	DefaultQuota      Quota
	Quotas            map[string]Quota // per-namespace override
}

// PolicyError is a request that is well-formed but not allowed.
type PolicyError struct{ Reason string }

func (e *PolicyError) Error() string { return "rejected by guardrail: " + e.Reason }

func (g Guardrail) quotaFor(ns string) Quota {
	if q, ok := g.Quotas[ns]; ok {
		return q
	}
	return g.DefaultQuota
}

// Check rejects a request whose image is from a disallowed registry, or that would push
// its namespace over quota given the deployments already running there.
func (g Guardrail) Check(req DeploymentRequest, existing []Deployment) error {
	allowed := false
	for _, prefix := range g.AllowedRegistries {
		if strings.HasPrefix(req.Image, prefix) {
			allowed = true
			break
		}
	}
	if !allowed {
		return &PolicyError{fmt.Sprintf("image %q is not from an allowed registry %v", req.Image, g.AllowedRegistries)}
	}

	var usedCPU, usedMem int
	for _, d := range existing {
		// A redeploy of the same name replaces the old one, so don't count it twice.
		if d.Namespace == req.Namespace && d.Name != req.Name {
			usedCPU += d.Replicas * d.Resources.CPUMillis
			usedMem += d.Replicas * d.Resources.MemoryMiB
		}
	}
	q := g.quotaFor(req.Namespace)
	wantCPU := usedCPU + req.Replicas*req.Resources.CPUMillis
	wantMem := usedMem + req.Replicas*req.Resources.MemoryMiB
	if wantCPU > q.CPUMillis {
		return &PolicyError{fmt.Sprintf("namespace %q CPU quota exceeded: %dm requested, %dm allowed", req.Namespace, wantCPU, q.CPUMillis)}
	}
	if wantMem > q.MemoryMiB {
		return &PolicyError{fmt.Sprintf("namespace %q memory quota exceeded: %dMi requested, %dMi allowed", req.Namespace, wantMem, q.MemoryMiB)}
	}
	return nil
}
