package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Resources are the per-replica limits a deployment asks for.
type Resources struct {
	CPUMillis int `json:"cpu_millis"`
	MemoryMiB int `json:"memory_mib"`
}

// DeploymentRequest is what a developer sends to POST /deployments.
type DeploymentRequest struct {
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	Image     string    `json:"image"`
	Replicas  int       `json:"replicas"`
	Resources Resources `json:"resources"`
}

// Deployment is a stored, accepted deployment intent.
type Deployment struct {
	ID string `json:"id"`
	DeploymentRequest
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Kubernetes object names: lowercase alphanumerics and '-', max 63 chars.
var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

// Validate checks the request is well-formed. Policy (registry, quota) lives in guardrail.go.
func (r DeploymentRequest) Validate() error {
	var errs []error
	if !dnsLabel.MatchString(r.Name) {
		errs = append(errs, fmt.Errorf("name %q must be a DNS label (lowercase, digits, '-')", r.Name))
	}
	if !dnsLabel.MatchString(r.Namespace) {
		errs = append(errs, fmt.Errorf("namespace %q must be a DNS label", r.Namespace))
	}
	if strings.TrimSpace(r.Image) == "" {
		errs = append(errs, errors.New("image is required"))
	}
	if r.Replicas < 1 || r.Replicas > 10 {
		errs = append(errs, fmt.Errorf("replicas must be between 1 and 10, got %d", r.Replicas))
	}
	if r.Resources.CPUMillis <= 0 || r.Resources.MemoryMiB <= 0 {
		errs = append(errs, errors.New("resource limits are required: set resources.cpu_millis and resources.memory_mib"))
	}
	return errors.Join(errs...)
}
