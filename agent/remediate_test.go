package main

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func replicaSet(rev, sa string, owner *appsv1.Deployment) *appsv1.ReplicaSet {
	yes := true
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "control-plane-" + rev, Namespace: "sentinel-system",
			Annotations:     map[string]string{revisionAnnotation: rev},
			OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: owner.Name, UID: owner.UID, Controller: &yes}},
		},
		Spec: appsv1.ReplicaSetSpec{Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "control-plane", "pod-template-hash": "h" + rev}},
			Spec:       corev1.PodSpec{ServiceAccountName: sa},
		}},
	}
}

// The M5 incident: revision 2 shipped with the wrong ServiceAccount.
func brokenRollout() *fake.Clientset {
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "control-plane", Namespace: "sentinel-system", UID: types.UID("dep-uid"),
			Annotations: map[string]string{revisionAnnotation: "2"},
		},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{ServiceAccountName: "default"}}},
	}
	return fake.NewClientset(d, replicaSet("1", "control-plane", d), replicaSet("2", "default", d))
}

func TestRolloutUndo(t *testing.T) {
	cs := brokenRollout()
	ctx := context.Background()
	out, err := Action{Type: "rollout_undo", Namespace: "sentinel-system", Deployment: "control-plane"}.Execute(ctx, cs)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := cs.AppsV1().Deployments("sentinel-system").Get(ctx, "control-plane", metav1.GetOptions{})
	if sa := d.Spec.Template.Spec.ServiceAccountName; sa != "control-plane" {
		t.Fatalf("want ServiceAccount restored to control-plane, got %q (%s)", sa, out)
	}
	if _, ok := d.Spec.Template.Labels["pod-template-hash"]; ok {
		t.Fatal("pod-template-hash must not be copied onto the Deployment")
	}
}

func TestScaleAndRestart(t *testing.T) {
	cs := brokenRollout()
	ctx := context.Background()
	if _, err := (Action{Type: "rollout_restart", Namespace: "sentinel-system", Deployment: "control-plane"}).Execute(ctx, cs); err != nil {
		t.Fatal(err)
	}
	d, _ := cs.AppsV1().Deployments("sentinel-system").Get(ctx, "control-plane", metav1.GetOptions{})
	if d.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatal("restart annotation not set")
	}
}

func TestValidateRejects(t *testing.T) {
	cs := brokenRollout()
	ctx := context.Background()
	for _, tt := range []struct {
		a    Action
		want string
	}{
		{Action{Type: "delete_namespace", Namespace: "team-a"}, "not allowlisted"},
		{Action{Type: "rollout_restart", Namespace: "kube-system", Deployment: "coredns"}, "namespace"},
		{Action{Type: "scale", Namespace: "sentinel-system", Deployment: "control-plane", Replicas: 50}, "replicas"},
		{Action{Type: "rollout_undo", Namespace: "team-a", Deployment: "ghost"}, "ghost"},
	} {
		if err := tt.a.Validate(ctx, cs); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%v: want error containing %q, got %v", tt.a, tt.want, err)
		}
	}
	if err := (Action{Type: "none"}).Validate(ctx, cs); err != nil {
		t.Errorf("none must always be valid: %v", err)
	}
}
