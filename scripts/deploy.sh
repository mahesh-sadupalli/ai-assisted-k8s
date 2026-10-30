#!/usr/bin/env bash
# Roll out a new control-plane image, smoke-test it, and roll back automatically on failure.
#   scripts/deploy.sh <image> [smoke-base-url]
# Without a URL it port-forwards to the Service. Writes deploy-report.txt for the agent.
set -uo pipefail
IMAGE=${1:?usage: deploy.sh <image> [smoke-url]}
SMOKE_URL=${2:-}
NS=${NAMESPACE:-sentinel-system}
DEPLOY=${DEPLOYMENT:-control-plane}
REPORT=${REPORT:-deploy-report.txt}

log() { echo "$*" | tee -a "$REPORT"; }
: > "$REPORT"

previous=$(kubectl -n "$NS" get deploy "$DEPLOY" -o jsonpath='{.spec.template.spec.containers[0].image}')
log "deploy: $DEPLOY $previous -> $IMAGE"
kubectl -n "$NS" set image "deployment/$DEPLOY" "$DEPLOY=$IMAGE" >/dev/null

ok=1
if ! kubectl -n "$NS" rollout status "deployment/$DEPLOY" --timeout=120s 2>&1 | tee -a "$REPORT"; then
  log "rollout: FAILED (pods never became ready)"
  ok=0
fi

if [[ $ok == 1 ]]; then
  pf=""
  if [[ -z $SMOKE_URL ]]; then
    kubectl -n "$NS" port-forward "svc/$DEPLOY" 18081:80 >/dev/null 2>&1 & pf=$!
    sleep 3
    SMOKE_URL=http://localhost:18081
  fi
  log "smoke: $SMOKE_URL"
  scripts/smoke.sh "$SMOKE_URL" 2>&1 | tee -a "$REPORT"
  [[ ${PIPESTATUS[0]} == 0 ]] || ok=0
  [[ -n $pf ]] && kill "$pf" 2>/dev/null
fi

if [[ $ok == 1 ]]; then
  log "result: SUCCESS"
  exit 0
fi

log "result: FAILED, rolling back to $previous"
kubectl -n "$NS" get pods -l "app=$DEPLOY" 2>&1 | tee -a "$REPORT"
kubectl -n "$NS" rollout undo "deployment/$DEPLOY" | tee -a "$REPORT"
kubectl -n "$NS" rollout status "deployment/$DEPLOY" --timeout=120s | tee -a "$REPORT"
exit 1
