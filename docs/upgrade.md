---
layout: default
title: Upgrade Guide
parent: Guides
nav_order: 5
description: "Version upgrade procedures, CRD migration steps, and rollback instructions."
---

# Upgrade Guide
{: .fs-8 }

Safe procedures for upgrading Snowplane across versions.
{: .fs-5 .fw-300 }

---

## Pre-Upgrade Checklist

Before upgrading Snowplane, verify the following:

- [ ] **Read the release notes** — Check for breaking changes, new CRDs, or deprecated features.
- [ ] **Back up CRDs and CRs** — See [Disaster Recovery]({% link production-guide.md %}#disaster-recovery).
- [ ] **Verify cluster health** — All current resources should be `Ready=True`.
- [ ] **Check Snowflake connectivity** — Ensure ProviderConfigs are healthy (`snowplane_providerconfig_healthy == 1`).
- [ ] **Plan a maintenance window** — Reconciliation pauses briefly during the upgrade.

```bash
# Verify all resources are healthy
kubectl get databases,schemas,warehouses,tables,views --all-namespaces \
  -o custom-columns='KIND:.kind,NAMESPACE:.metadata.namespace,NAME:.metadata.name,READY:.status.conditions[?(@.type=="Ready")].status'

# Check ProviderConfig health
kubectl get providerconfig -o wide
```

---

## Upgrade Procedure

### Step 1: Update CRDs

Helm does **not** upgrade CRDs automatically. Always apply CRDs before upgrading the Helm release:

```bash
# Apply CRDs with server-side apply (handles large CRDs safely)
kubectl apply --server-side --force-conflicts -f charts/snowplane/crds/
```

{: .warning }
> Skipping this step may cause the controller to fail with `no matches for kind` errors if new resource types were added.

### Step 2: Upgrade the Helm Release

```bash
helm upgrade snowplane charts/snowplane/ \
  --namespace snowplane-system \
  --reuse-values
```

To override specific values during upgrade:

```bash
helm upgrade snowplane charts/snowplane/ \
  --namespace snowplane-system \
  --reuse-values \
  --set controller.maxConcurrentReconciles=5 \
  --set resources.limits.memory=1Gi
```

### Step 3: Verify the Upgrade

```bash
# Wait for rollout to complete
kubectl -n snowplane-system rollout status deployment/snowplane

# Check controller logs for errors
kubectl -n snowplane-system logs -l app.kubernetes.io/name=snowplane --tail=50

# Verify ProviderConfig health
kubectl get providerconfig -o wide

# Check metrics endpoint
kubectl -n snowplane-system port-forward svc/snowplane-metrics 8080:8080 &
curl -s localhost:8080/metrics | grep snowplane_providerconfig_healthy
```

---

## Rollback Procedure

If the upgrade fails, roll back to the previous Helm release:

```bash
# List release history
helm history snowplane -n snowplane-system

# Rollback to previous revision
helm rollback snowplane <REVISION> -n snowplane-system

# Verify rollback
kubectl -n snowplane-system rollout status deployment/snowplane
```

{: .note }
> CRD rollback is NOT automatic. New CRDs added in the failed upgrade remain in the cluster. They are harmless — the old controller version simply ignores them. To fully clean up, manually delete unused CRDs: `kubectl delete crd <name>.snowplane.hupe1980.github.io`.

---

## Version Compatibility

| Snowplane Version | Kubernetes | Snowflake Driver | Notes |
|:-----------------|:-----------|:-----------------|:------|
| v0.x (current) | 1.28+ | gosnowflake v1.12+ | All CRDs at v1alpha1 |

{: .note }
> All CRDs are currently at `v1alpha1`. API changes between versions may be breaking. When Snowplane graduates CRDs to `v1beta1`, conversion webhooks will handle automatic migration.

---

## Breaking Change Handling

### Field Type Changes

If a field type changes between versions (e.g., `bool` → `*bool`):

1. Apply the new CRD first (`kubectl apply --server-side -f charts/snowplane/crds/`).
2. Update affected CRs to use the new field type.
3. Upgrade the Helm release.

### Removed CRDs

If a CRD is deprecated and removed:

1. Migrate resources to the replacement CRD before upgrading.
2. Delete all CRs of the deprecated type.
3. Upgrade the Helm release.
4. Delete the deprecated CRD: `kubectl delete crd <name>.snowplane.hupe1980.github.io`.

### Renamed Fields

If a spec field is renamed:

1. Apply the new CRD.
2. Update all CRs with the new field name.
3. Upgrade the Helm release.

---

## CRD-Only Updates

To update CRDs without upgrading the controller (e.g., to pick up validation improvements):

```bash
kubectl apply --server-side --force-conflicts -f charts/snowplane/crds/
```

This is safe — the running controller handles both old and new CRD schemas as long as no fields are removed.

---

## Multi-Instance Upgrades

When running multiple Snowplane instances (e.g., namespace-sharded or CRD-sharded):

1. Update CRDs once (shared across all instances).
2. Upgrade instances one at a time with rolling strategy.
3. Verify each instance is healthy before proceeding to the next.

```bash
# Update shared CRDs
kubectl apply --server-side --force-conflicts -f charts/snowplane/crds/

# Upgrade instances sequentially
for instance in snowplane-shard-0 snowplane-shard-1 snowplane-shard-2; do
  helm upgrade "$instance" charts/snowplane/ -n snowplane-system --reuse-values
  kubectl -n snowplane-system rollout status "deployment/$instance"
  echo "✓ $instance upgraded successfully"
done
```

---

## Troubleshooting Upgrades

### Controller CrashLoopBackOff After Upgrade

**Cause:** CRDs not updated before the Helm release.

```bash
kubectl apply --server-side --force-conflicts -f charts/snowplane/crds/
kubectl -n snowplane-system rollout restart deployment/snowplane
```

### Resources Stuck in Reconciling State

**Cause:** Controller restarted during reconciliation. Resources will auto-recover within one reconcile cycle (default: 5 minutes sync period).

### Webhook Errors After Upgrade

**Cause:** cert-manager certificate not renewed for new webhook configuration.

```bash
# Check certificate status
kubectl get certificate -n snowplane-system

# Force renewal
kubectl delete certificate snowplane-webhook-cert -n snowplane-system
# cert-manager will recreate it automatically
```

---

## Further Reading

- [Production Guide]({% link production-guide.md %}) — Deployment checklist and security hardening
- [Helm Chart]({% link helm-chart.md %}) — Full Helm values reference
- [Troubleshooting]({% link troubleshooting.md %}) — Diagnosing and resolving common issues
