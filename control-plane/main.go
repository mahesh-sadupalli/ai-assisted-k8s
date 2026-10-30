// Command control-plane is Sentinel's self-service deploy API.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// kubeConfig returns in-cluster config when running as a pod, else $KUBECONFIG.
// nil means no cluster: the control-plane only records intents (M1 mode).
func kubeConfig() *rest.Config {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg
	}
	if path := os.Getenv("KUBECONFIG"); path != "" {
		if cfg, err := clientcmd.BuildConfigFromFlags("", path); err == nil {
			return cfg
		}
	}
	return nil
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	guardrail := Guardrail{
		// comma-separated image prefixes, e.g. "ghcr.io/me/,123.dkr.ecr.eu-central-1.amazonaws.com/"
		AllowedRegistries: strings.Split(getenv("ALLOWED_REGISTRIES", "ghcr.io/mahesh-sadupalli/"), ","),
		DefaultQuota:      Quota{CPUMillis: 2000, MemoryMiB: 2048},
	}

	var applier Applier
	if cfg := kubeConfig(); cfg != nil {
		kube := NewKube(kubernetes.NewForConfigOrDie(cfg))
		applier = kube
		guardrail.Lookup = func(ns string) (Quota, bool) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			return kube.Quota(ctx, ns)
		}
		log.Info("cluster mode: deployments are applied, quotas read from ResourceQuota/" + quotaName)
	} else {
		log.Info("record-only mode: no cluster config found")
	}

	addr := getenv("ADDR", ":8080")
	srv := NewServer(NewStore(), guardrail, applier, log)
	log.Info("control-plane listening", "addr", addr, "allowed_registries", guardrail.AllowedRegistries)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
