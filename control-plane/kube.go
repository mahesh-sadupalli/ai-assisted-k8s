package main

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	fieldManager = "sentinel-control-plane"
	quotaName    = "sentinel-quota" // ResourceQuota the guardrail reads in each team namespace
)

// Kube applies accepted deployments to a cluster and reads namespace quotas from it.
type Kube struct {
	cs kubernetes.Interface
}

func NewKube(cs kubernetes.Interface) *Kube { return &Kube{cs: cs} }

// Apply server-side-applies a Deployment + Service for d. Applying again updates in place.
func (k *Kube) Apply(ctx context.Context, d Deployment) error {
	labels := map[string]string{"app": d.Name, "app.kubernetes.io/managed-by": "sentinel"}
	limits := corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewMilliQuantity(int64(d.Resources.CPUMillis), resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(int64(d.Resources.MemoryMiB)<<20, resource.BinarySI),
	}

	dep := appsv1ac.Deployment(d.Name, d.Namespace).
		WithLabels(labels).
		WithSpec(appsv1ac.DeploymentSpec().
			WithReplicas(int32(d.Replicas)).
			WithSelector(metav1ac.LabelSelector().WithMatchLabels(map[string]string{"app": d.Name})).
			WithTemplate(corev1ac.PodTemplateSpec().
				WithLabels(labels).
				WithSpec(corev1ac.PodSpec().WithContainers(corev1ac.Container().
					WithName(d.Name).
					WithImage(d.Image).
					WithImagePullPolicy(corev1.PullIfNotPresent).
					WithPorts(corev1ac.ContainerPort().WithContainerPort(8080)).
					// requests = limits: the quota counts exactly what was approved
					WithResources(corev1ac.ResourceRequirements().WithLimits(limits).WithRequests(limits)).
					WithReadinessProbe(corev1ac.Probe().WithHTTPGet(
						corev1ac.HTTPGetAction().WithPath("/healthz").WithPort(intstr.FromInt32(8080)))),
				))))
	opts := metav1.ApplyOptions{FieldManager: fieldManager, Force: true}
	if _, err := k.cs.AppsV1().Deployments(d.Namespace).Apply(ctx, dep, opts); err != nil {
		return fmt.Errorf("apply deployment: %w", err)
	}

	svc := corev1ac.Service(d.Name, d.Namespace).
		WithLabels(labels).
		WithSpec(corev1ac.ServiceSpec().
			WithSelector(map[string]string{"app": d.Name}).
			WithPorts(corev1ac.ServicePort().WithPort(80).WithTargetPort(intstr.FromInt32(8080))))
	if _, err := k.cs.CoreV1().Services(d.Namespace).Apply(ctx, svc, opts); err != nil {
		return fmt.Errorf("apply service: %w", err)
	}
	return nil
}

// Quota reads the namespace's sentinel-quota ResourceQuota (limits.cpu / limits.memory).
// ok=false means the namespace has none, and the guardrail falls back to its default.
func (k *Kube) Quota(ctx context.Context, ns string) (Quota, bool) {
	rq, err := k.cs.CoreV1().ResourceQuotas(ns).Get(ctx, quotaName, metav1.GetOptions{})
	if err != nil {
		return Quota{}, false
	}
	cpu, okCPU := rq.Spec.Hard[corev1.ResourceLimitsCPU]
	mem, okMem := rq.Spec.Hard[corev1.ResourceLimitsMemory]
	if !okCPU || !okMem {
		return Quota{}, false
	}
	return Quota{CPUMillis: int(cpu.MilliValue()), MemoryMiB: int(mem.Value() >> 20)}, true
}
