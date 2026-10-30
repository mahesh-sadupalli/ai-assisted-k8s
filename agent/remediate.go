package main

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Action is a remediation the agent may propose. The model picks from this fixed menu;
// it can never run arbitrary commands.
type Action struct {
	Type       string `json:"type"` // rollout_undo | rollout_restart | scale | none
	Namespace  string `json:"namespace"`
	Deployment string `json:"deployment"`
	Replicas   int    `json:"replicas"` // scale only
}

var (
	allowedActions    = []string{"rollout_undo", "rollout_restart", "scale", "none"}
	allowedNamespaces = []string{"sentinel-system", "team-a"}
)

const maxReplicas = 5

func (a Action) String() string {
	switch a.Type {
	case "none":
		return "no automated action (escalate to a human)"
	case "scale":
		return fmt.Sprintf("scale %s/%s to %d replicas", a.Namespace, a.Deployment, a.Replicas)
	default:
		return fmt.Sprintf("%s %s/%s", a.Type, a.Namespace, a.Deployment)
	}
}

// Validate is the hard gate between what the model proposes and what may run.
func (a Action) Validate(ctx context.Context, cs kubernetes.Interface) error {
	if !slices.Contains(allowedActions, a.Type) {
		return fmt.Errorf("action %q is not allowlisted", a.Type)
	}
	if a.Type == "none" {
		return nil
	}
	if !slices.Contains(allowedNamespaces, a.Namespace) {
		return fmt.Errorf("namespace %q is not allowlisted", a.Namespace)
	}
	if a.Type == "scale" && (a.Replicas < 1 || a.Replicas > maxReplicas) {
		return fmt.Errorf("replicas must be 1..%d, got %d", maxReplicas, a.Replicas)
	}
	if _, err := cs.AppsV1().Deployments(a.Namespace).Get(ctx, a.Deployment, metav1.GetOptions{}); err != nil {
		return fmt.Errorf("deployment %s/%s: %w", a.Namespace, a.Deployment, err)
	}
	return nil
}

// Execute runs a validated action. Only called after a human approved it.
func (a Action) Execute(ctx context.Context, cs kubernetes.Interface) (string, error) {
	if err := a.Validate(ctx, cs); err != nil {
		return "", err
	}
	deps := cs.AppsV1().Deployments(a.Namespace)
	switch a.Type {
	case "none":
		return "nothing to do", nil
	case "scale":
		s, err := deps.GetScale(ctx, a.Deployment, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		from := s.Spec.Replicas
		s.Spec.Replicas = int32(a.Replicas)
		if _, err := deps.UpdateScale(ctx, a.Deployment, s, metav1.UpdateOptions{}); err != nil {
			return "", err
		}
		return fmt.Sprintf("scaled %d -> %d", from, a.Replicas), nil
	case "rollout_restart":
		d, err := deps.Get(ctx, a.Deployment, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		if d.Spec.Template.Annotations == nil {
			d.Spec.Template.Annotations = map[string]string{}
		}
		d.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().UTC().Format(time.RFC3339)
		if _, err := deps.Update(ctx, d, metav1.UpdateOptions{}); err != nil {
			return "", err
		}
		return "restarted", nil
	case "rollout_undo":
		return rolloutUndo(ctx, cs, a.Namespace, a.Deployment)
	}
	return "", fmt.Errorf("unreachable action %q", a.Type)
}

const revisionAnnotation = "deployment.kubernetes.io/revision"

func revision(meta metav1.ObjectMeta) int {
	r, _ := strconv.Atoi(meta.Annotations[revisionAnnotation])
	return r
}

// rolloutUndo does what `kubectl rollout undo` does: copy the pod template of the
// previous revision's ReplicaSet back onto the Deployment.
func rolloutUndo(ctx context.Context, cs kubernetes.Interface, ns, name string) (string, error) {
	d, err := cs.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	rsList, err := cs.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	var owned []appsv1.ReplicaSet
	for _, rs := range rsList.Items {
		if metav1.IsControlledBy(&rs, d) {
			owned = append(owned, rs)
		}
	}
	sort.Slice(owned, func(i, j int) bool { return revision(owned[i].ObjectMeta) > revision(owned[j].ObjectMeta) })

	current := revision(d.ObjectMeta)
	for _, rs := range owned {
		if r := revision(rs.ObjectMeta); r < current {
			tmpl := rs.Spec.Template.DeepCopy()
			delete(tmpl.Labels, appsv1.DefaultDeploymentUniqueLabelKey) // pod-template-hash belongs to the RS
			d.Spec.Template = *tmpl
			if _, err := cs.AppsV1().Deployments(ns).Update(ctx, d, metav1.UpdateOptions{}); err != nil {
				return "", err
			}
			return fmt.Sprintf("rolled back from revision %d to %d", current, r), nil
		}
	}
	return "", fmt.Errorf("no previous revision of %s/%s to roll back to", ns, name)
}
