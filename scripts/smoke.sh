#!/usr/bin/env bash
# Post-deploy smoke test for the control-plane: checks behaviour, not just liveness.
#   scripts/smoke.sh http://localhost:8080
set -uo pipefail
URL=${1:?usage: smoke.sh <base-url>}
fail=0

check() { # name, expected code, curl args...
  local name=$1 want=$2; shift 2
  local got
  got=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$@")
  if [[ $got == "$want" ]]; then echo "ok   $name ($got)"; else echo "FAIL $name: want $want, got $got"; fail=1; fi
}

check "unknown id is 404"          404 "$URL/deployments/does-not-exist"
check "malformed request is 400"   400 -X POST "$URL/deployments" -d '{'
check "guardrail blocks registry"  403 -X POST "$URL/deployments" \
  -d '{"name":"smoke","namespace":"team-a","image":"docker.io/evil:1","replicas":1,"resources":{"cpu_millis":1,"memory_mib":1}}'
exit $fail
