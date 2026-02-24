#!/usr/bin/env bash
# setup-e2e-kind.sh — Bootstrap a kind cluster with the Snowplane operator
# for **manual** E2E testing or local development.
#
# NOTE: The automated E2E test suite (go test -tags e2e ./test/e2e/) now uses
# a self-contained k3s testcontainer and no longer requires this script.
# This script is kept in hack/ for ad-hoc / interactive testing.
#
# Prerequisites:
#   docker, kind (>=0.27), kubectl, helm
#
# Required env vars:
#   SNOWFLAKE_ACCOUNT, SNOWFLAKE_USER
#   One of: SNOWFLAKE_PASSWORD or SNOWFLAKE_PRIVATE_KEY
#
# Optional env vars:
#   SNOWFLAKE_ROLE      (default: SYSADMIN)
#   SNOWFLAKE_WAREHOUSE (default: COMPUTE_WH)
#   KIND_CLUSTER_NAME   (default: snowplane-e2e)
#   IMAGE_TAG           (default: e2e)
set -euo pipefail

CLUSTER="${KIND_CLUSTER_NAME:-snowplane-e2e}"
TAG="${IMAGE_TAG:-e2e}"
IMAGE="snowplane:${TAG}"
NAMESPACE="snowplane-system"
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

SNOWFLAKE_ROLE="${SNOWFLAKE_ROLE:-SYSADMIN}"
SNOWFLAKE_WAREHOUSE="${SNOWFLAKE_WAREHOUSE:-}" # optional; leave empty if not available

# ── Validate env ──────────────────────────────────────────────────────────────
for var in SNOWFLAKE_ACCOUNT SNOWFLAKE_USER; do
  if [[ -z "${!var:-}" ]]; then
    echo "ERROR: $var is not set" >&2
    exit 1
  fi
done

if [[ -z "${SNOWFLAKE_PASSWORD:-}" ]] && [[ -z "${SNOWFLAKE_PRIVATE_KEY:-}" ]]; then
  echo "ERROR: either SNOWFLAKE_PASSWORD or SNOWFLAKE_PRIVATE_KEY must be set" >&2
  exit 1
fi

for cmd in docker kind kubectl helm; do
  if ! command -v "$cmd" &>/dev/null; then
    echo "ERROR: $cmd is not installed" >&2
    exit 1
  fi
done

# ── Build image ───────────────────────────────────────────────────────────────
echo "==> Building Docker image ${IMAGE}"
docker build -t "${IMAGE}" "${REPO_ROOT}"

# ── Create kind cluster (idempotent) ─────────────────────────────────────────
if kind get clusters 2>/dev/null | grep -qx "${CLUSTER}"; then
  echo "==> Kind cluster '${CLUSTER}' already exists, reusing"
else
  echo "==> Creating kind cluster '${CLUSTER}'"
  kind create cluster --name "${CLUSTER}" --wait 60s
fi

# ── Load image into kind ─────────────────────────────────────────────────────
echo "==> Loading image into kind"
kind load docker-image "${IMAGE}" --name "${CLUSTER}"

# ── Install CRDs ─────────────────────────────────────────────────────────────
# CRDs are installed by Helm from charts/snowplane/crds/
# Pre-apply them via kubectl so Helm doesn't conflict on upgrades.
echo "==> Installing CRDs"
kubectl apply --server-side --force-conflicts -f "${REPO_ROOT}/config/crd/bases/"

# ── Deploy operator via Helm ─────────────────────────────────────────────────
echo "==> Deploying Snowplane operator via Helm"
helm upgrade --install snowplane "${REPO_ROOT}/charts/snowplane" \
  --namespace "${NAMESPACE}" --create-namespace \
  --skip-crds \
  --set image.repository=snowplane \
  --set image.tag="${TAG}" \
  --set image.pullPolicy=Never \
  --set leaderElection.enabled=false \
  --set controller.requeueInterval=10s \
  --set controller.enableAlphaResources=true \
  --wait --timeout 120s

# ── Wait for operator pod to be ready ─────────────────────────────────────────
echo "==> Waiting for operator pod"
kubectl -n "${NAMESPACE}" rollout status deployment/snowplane --timeout=120s

# ── Create Snowflake credentials secret ───────────────────────────────────────
echo "==> Creating Snowflake credentials"
kubectl -n "${NAMESPACE}" delete secret snowflake-credentials 2>/dev/null || true

if [[ -n "${SNOWFLAKE_PRIVATE_KEY:-}" ]]; then
  AUTH_TYPE="KeyPair"
  SECRET_KEY="privateKey"
  kubectl -n "${NAMESPACE}" create secret generic snowflake-credentials \
    --from-literal=privateKey="${SNOWFLAKE_PRIVATE_KEY}"
else
  AUTH_TYPE="UsernamePassword"
  SECRET_KEY="password"
  kubectl -n "${NAMESPACE}" create secret generic snowflake-credentials \
    --from-literal=password="${SNOWFLAKE_PASSWORD}"
fi

# ── Create ProviderConfig ─────────────────────────────────────────────────────
echo "==> Creating ProviderConfig (${AUTH_TYPE})"

WAREHOUSE_LINE=""
if [[ -n "${SNOWFLAKE_WAREHOUSE}" ]]; then
  WAREHOUSE_LINE="  warehouse: \"${SNOWFLAKE_WAREHOUSE}\""
fi

cat <<EOF | kubectl apply -f -
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
  namespace: ${NAMESPACE}
spec:
  account: "${SNOWFLAKE_ACCOUNT}"
  user: "${SNOWFLAKE_USER}"
  role: "${SNOWFLAKE_ROLE}"
${WAREHOUSE_LINE}
  authenticationType: ${AUTH_TYPE}
  credentials:
    secretRef:
      name: snowflake-credentials
      key: ${SECRET_KEY}
EOF

echo ""
echo "==> Setup complete! Run E2E tests with:"
echo "    go test -tags e2e -v -timeout 15m -count=1 ./test/e2e/"
