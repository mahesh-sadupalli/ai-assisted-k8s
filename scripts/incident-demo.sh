#!/usr/bin/env bash
# M5 capstone incident: a "routine" manifest change ships the control-plane with the
# default ServiceAccount. Pods are healthy, but every apply is forbidden -> 502s -> the
# SLO burn-rate alert fires -> the agent triages and proposes a remediation.
set -euo pipefail
kubectl apply -f infra/k8s-demo/loadgen.yaml
echo "baseline traffic for 3 minutes..."
sleep 180
echo "injecting the bad change: serviceAccountName=default"
kubectl -n sentinel-system patch deployment control-plane \
  -p '{"spec":{"template":{"spec":{"serviceAccountName":"default"}}}}'
kubectl -n sentinel-system rollout status deployment/control-plane --timeout=120s
echo "now watch: the alert fires in ~3-5 minutes, then"
echo "  kubectl -n sentinel-system port-forward svc/sentinel-agent 9095:80"
echo "  go run ./agent proposals"
