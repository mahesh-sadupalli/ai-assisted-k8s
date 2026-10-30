package main

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func oomPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "hello-1", Namespace: "team-a"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "hello", Image: "ghcr.io/mahesh-sadupalli/hello:dev",
			Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			}},
		}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "hello", RestartCount: 3,
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
					Reason: "OOMKilled", ExitCode: 137,
				}},
			}},
		},
	}
}

func healthyPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "ok-1", Namespace: "team-a"},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{
			Name: "ok", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		}}},
	}
}

func TestUnhealthyPods(t *testing.T) {
	cs := fake.NewClientset(oomPod(), healthyPod())
	pods, err := UnhealthyPods(context.Background(), cs, "team-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 1 || pods[0].Name != "hello-1" {
		t.Fatalf("want only hello-1, got %d pods", len(pods))
	}
}

func TestPodEvidence(t *testing.T) {
	cs := fake.NewClientset(oomPod())
	ev, err := PodEvidence(context.Background(), cs, "team-a", "hello-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"restarts=3", "CrashLoopBackOff", "reason=OOMKilled exit_code=137", "memory=64Mi", "previous (crashed)"} {
		if !strings.Contains(ev, want) {
			t.Errorf("evidence missing %q:\n%s", want, ev)
		}
	}
}
