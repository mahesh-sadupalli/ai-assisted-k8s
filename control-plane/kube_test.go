package main

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubeApply(t *testing.T) {
	cs := fake.NewClientset()
	k := NewKube(cs)
	ctx := context.Background()
	d := Deployment{DeploymentRequest: validRequest()}

	if err := k.Apply(ctx, d); err != nil {
		t.Fatal(err)
	}
	dep, err := cs.AppsV1().Deployments("team-a").Get(ctx, "hello", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	c := dep.Spec.Template.Spec.Containers[0]
	if *dep.Spec.Replicas != 2 || c.Image != d.Image {
		t.Fatalf("replicas=%d image=%s", *dep.Spec.Replicas, c.Image)
	}
	if got := c.Resources.Limits.Memory().String(); got != "256Mi" {
		t.Fatalf("memory limit %s, want 256Mi", got)
	}
	if got := c.Resources.Limits.Cpu().String(); got != "250m" {
		t.Fatalf("cpu limit %s, want 250m", got)
	}
	if _, err := cs.CoreV1().Services("team-a").Get(ctx, "hello", metav1.GetOptions{}); err != nil {
		t.Fatalf("service not created: %v", err)
	}
}

func TestKubeQuota(t *testing.T) {
	cs := fake.NewClientset(&corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: quotaName, Namespace: "team-a"},
		Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
			corev1.ResourceLimitsCPU:    resource.MustParse("1500m"),
			corev1.ResourceLimitsMemory: resource.MustParse("1Gi"),
		}},
	})
	k := NewKube(cs)
	q, ok := k.Quota(context.Background(), "team-a")
	if !ok || q != (Quota{CPUMillis: 1500, MemoryMiB: 1024}) {
		t.Fatalf("got %+v ok=%v", q, ok)
	}
	if _, ok := k.Quota(context.Background(), "no-quota"); ok {
		t.Fatal("want ok=false for namespace without quota")
	}
}
