#!/usr/bin/env bash
# Minimal post-upgrade smoke test, invoked by spec.postUpgrade.smokeTestCommand.
# Customize this per-cluster to exercise the critical user-facing paths for your
# workloads (health endpoints, synthetic transactions, etc.). Exits non-zero on
# failure so the POST_UPGRADE_CHECK phase fails closed.
set -euo pipefail

NAMESPACE="${SMOKE_TEST_NAMESPACE:-default}"

echo "Running smoke tests in namespace ${NAMESPACE}..."

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl not found on PATH" >&2
  exit 1
fi

echo "Checking node readiness..."
kubectl get nodes --no-headers | awk '{print $2}' | grep -vq NotReady || {
  echo "one or more nodes are NotReady" >&2
  exit 1
}

echo "Checking core DNS resolution..."
kubectl run smoke-test-dns --rm -i --restart=Never --image=busybox:1.36 --namespace "${NAMESPACE}" \
  --command -- nslookup kubernetes.default >/dev/null

echo "Smoke tests passed."
