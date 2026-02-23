#!/usr/bin/env bash
# teardown.sh — Remove the kind cluster created for E2E tests.
set -euo pipefail

CLUSTER="${KIND_CLUSTER_NAME:-snowplane-e2e}"

echo "==> Deleting kind cluster '${CLUSTER}'"
kind delete cluster --name "${CLUSTER}" 2>/dev/null || true
echo "==> Done"
