package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const logTailLines = 50

// kubeClient uses $KUBECONFIG or ~/.kube/config, like kubectl.
func kubeClient() (kubernetes.Interface, error) {
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), nil).ClientConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

// unhealthy reports whether a pod is worth diagnosing: restarting, waiting, or not running.
func unhealthy(p corev1.Pod) bool {
	if p.Status.Phase != corev1.PodRunning && p.Status.Phase != corev1.PodSucceeded {
		return true
	}
	for _, c := range p.Status.ContainerStatuses {
		if c.RestartCount > 0 || c.State.Waiting != nil {
			return true
		}
	}
	return false
}

// UnhealthyPods lists the pods in ns that look broken.
func UnhealthyPods(ctx context.Context, cs kubernetes.Interface, ns string) ([]corev1.Pod, error) {
	pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var out []corev1.Pod
	for _, p := range pods.Items {
		if unhealthy(p) {
			out = append(out, p)
		}
	}
	return out, nil
}

// PodEvidence gathers what an SRE would look at — status, limits, events, recent logs —
// into one text block for the model. This is the agent's "perceive" step on a live cluster.
func PodEvidence(ctx context.Context, cs kubernetes.Interface, ns, name string) (string, error) {
	pod, err := cs.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Pod: %s/%s\nPhase: %s\n", ns, name, pod.Status.Phase)

	for _, c := range pod.Spec.Containers {
		fmt.Fprintf(&b, "Container %s image=%s limits: cpu=%s memory=%s\n",
			c.Name, c.Image, c.Resources.Limits.Cpu(), c.Resources.Limits.Memory())
	}
	restarted := map[string]bool{}
	for _, s := range pod.Status.ContainerStatuses {
		fmt.Fprintf(&b, "Container %s ready=%v restarts=%d\n", s.Name, s.Ready, s.RestartCount)
		if w := s.State.Waiting; w != nil {
			fmt.Fprintf(&b, "  State: Waiting reason=%s message=%s\n", w.Reason, w.Message)
		}
		if t := s.LastTerminationState.Terminated; t != nil {
			fmt.Fprintf(&b, "  Last state: Terminated reason=%s exit_code=%d\n", t.Reason, t.ExitCode)
		}
		restarted[s.Name] = s.RestartCount > 0
	}

	events, err := cs.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("involvedObject.name", name).String(),
	})
	if err == nil && len(events.Items) > 0 {
		b.WriteString("Events:\n")
		for _, e := range events.Items {
			fmt.Fprintf(&b, "  %s %s (x%d): %s\n", e.Type, e.Reason, e.Count, e.Message)
		}
	}

	for _, c := range pod.Spec.Containers {
		// After a crash the useful lines are in the previous container's logs.
		if restarted[c.Name] {
			b.WriteString(podLogs(ctx, cs, ns, name, c.Name, true))
		}
		b.WriteString(podLogs(ctx, cs, ns, name, c.Name, false))
	}
	return b.String(), nil
}

func podLogs(ctx context.Context, cs kubernetes.Interface, ns, pod, container string, previous bool) string {
	tail := int64(logTailLines)
	rc, err := cs.CoreV1().Pods(ns).GetLogs(pod, &corev1.PodLogOptions{
		Container: container, Previous: previous, TailLines: &tail,
	}).Stream(ctx)
	if err != nil {
		return ""
	}
	defer rc.Close()
	logs, _ := io.ReadAll(rc)
	which := "current"
	if previous {
		which = "previous (crashed)"
	}
	return fmt.Sprintf("Logs %s container %s, last %d lines:\n%s\n", which, container, logTailLines, logs)
}
