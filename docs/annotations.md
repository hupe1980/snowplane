---
layout: default
title: Annotations Reference
parent: Concepts
nav_order: 3
description: "Complete reference for all Snowplane annotations and labels."
---

# Annotations Reference
{: .fs-8 }

Snowplane uses annotations to control per-resource behavior at runtime — no controller restart needed.
{: .fs-5 .fw-300 }

{: .note }
> **Lifecycle policies are spec fields.** `adoptionPolicy`, `driftPolicy`, and `createOrAlter` are configured via `spec.managementPolicies` with enum validation and `kubectl explain` support. See [Management Policies](#management-policies) below.

---

## Spec Fields

### `spec.paused`

```yaml
spec:
  paused: true
```

Suspends all reconciliation for this resource. When `true`, the controller skips all Snowflake operations (Observe, Create, Alter, Drop) and sets `Synced=False` with reason `ReconcilePaused`. The Snowflake resource is not modified or deleted while paused.

| Default | Type |
|:--------|:-----|
| `false` | `boolean` |

**Use cases:**
- **Emergency brake** — stop reconciliation for a misbehaving resource without deleting it
- **Maintenance windows** — pause during Snowflake maintenance to prevent spurious drift alerts
- **Debugging** — freeze the resource state while investigating an issue

```bash
# Pause reconciliation
kubectl patch database my-db --type merge -p '{"spec":{"paused":true}}'

# Check status
kubectl get database my-db -o jsonpath='{.status.conditions[?(@.type=="Synced")].reason}'
# ReconcilePaused

# Resume
kubectl patch database my-db --type merge -p '{"spec":{"paused":false}}'
```

---

### `spec.managementPolicies` {#management-policies}

```yaml
spec:
  managementPolicies:
    adoptionPolicy: adopt          # or "fail-if-exists" (default)
    driftPolicy: detect-only       # or "correct" (default)
    createOrAlter: true            # default: true
```

Lifecycle policies for the managed resource. All fields are optional and default to production-safe values.

| Field | Default | Values | Description |
|:------|:--------|:-------|:------------|
| `adoptionPolicy` | `fail-if-exists` | `adopt`, `fail-if-exists` | Controls what happens when a Snowflake object already exists at creation time |
| `driftPolicy` | `correct` | `correct`, `detect-only` | Controls whether detected drift is corrected or only reported |
| `createOrAlter` | `true` | `true`, `false` | Use `CREATE OR ALTER` (atomic) vs legacy `CREATE` + `ALTER` two-step flow |

{: .note }
> Spec fields are the single source of truth for lifecycle policies. Changes to these fields bump the resource generation, which triggers reconciliation automatically.

---

## Annotations

### `force-new`

```yaml
snowplane.hupe1980.github.io/force-new: "true"
```

When set to `true`, the controller will **delete and recreate** the Snowflake object when an immutable spec field changes, instead of rejecting the change with an error.

| Default | Values |
|:--------|:-------|
| `false` | `true` / `false` |

{: .warning }
> Force-new performs a destructive `DROP` + `CREATE`. All data in the dropped object is lost. Use with caution on production resources like Tables and Databases.

---

### `allow-dangerous-grant`

```yaml
snowplane.hupe1980.github.io/allow-dangerous-grant: "true"
```

Allows grants to powerful system roles (e.g., `ACCOUNTADMIN`, `SECURITYADMIN`) and dangerous privileges (e.g., `OWNERSHIP`). Without this annotation, such grants are rejected with `Ready=False`.

| Default | Values |
|:--------|:-------|
| `false` | `true` / `false` |

---

### `abandon-on-delete`

```yaml
snowplane.hupe1980.github.io/abandon-on-delete: "true"
```

When set to `true`, deleting the Kubernetes resource **removes the finalizer without dropping the Snowflake object**. The object is "orphaned" — it continues to exist in Snowflake but is no longer managed by Snowplane.

| Default | Values |
|:--------|:-------|
| `false` | `true` / `false` |

**Use cases:**

- **Unblocking stuck deletions** — If a `DROP` fails permanently (e.g., permissions revoked, object has dependencies), set this annotation to force the Kubernetes resource to be deleted.
- **Migrating ownership** — Hand off a Snowflake object to another team/tool without destroying it.
- **Testing** — Clean up Kubernetes resources after tests without affecting the Snowflake account.

**Example: Unblocking a stuck deletion**

```bash
# 1. Resource is stuck deleting (DROP keeps failing)
kubectl get database my-db
# NAME    SYNCED   READY   AGE
# my-db   False    False   2d    # Stuck in deletion

# 2. Set the abandon annotation
kubectl annotate database my-db \
  snowplane.hupe1980.github.io/abandon-on-delete=true

# 3. The controller skips DROP, removes the finalizer, and the resource is deleted
# 4. The Snowflake database MY_DB still exists — clean it up manually if needed
```

{: .warning }
> Abandoned resources remain in Snowflake and may incur costs. Track orphaned resources via the `snowplane_orphaned_resources_total` metric.

---

### Force Reconcile (Trigger Pattern)

There is no dedicated force-reconcile annotation. Instead, any annotation change triggers an immediate reconciliation because the controller watches for annotation updates via `AnnotationChangedPredicate`.

**To force an immediate reconciliation**, set any arbitrary annotation:

```bash
# Force reconcile by setting a timestamp annotation
kubectl annotate database my-db \
  snowplane.hupe1980.github.io/reconcile-trigger="$(date +%s)" --overwrite
```

This works for **all** managed resource types (Database, Schema, Table, Warehouse, etc.).

**When to use:**

- After making manual changes in the Snowflake console that you want the controller to detect immediately
- During debugging, when you don't want to wait for the default requeue interval (5 min)
- After fixing a terminal error condition (e.g., granting missing privileges) to retry immediately

{: .note }
> The `reconcile-trigger` annotation has no special meaning to the controller — it simply causes Kubernetes to emit an update event. Any annotation change (add, modify, or remove) will trigger reconciliation.

---

## Labels

### `maturity`

```yaml
snowplane.hupe1980.github.io/maturity: "stable"
```

Set automatically on CRDs to indicate their maturity level. Used by the controller to gate alpha-maturity resources behind `controller.enableAlphaResources`.

| Values |
|:-------|
| `alpha` / `beta` / `stable` |

### `external-name-hash`

```yaml
snowplane.hupe1980.github.io/external-name-hash: "a1b2c3d4"
```

Set automatically by the controller. Contains a SHA-256 prefix of the Snowflake object's fully qualified name. Prevents duplicate CRs from managing the same Snowflake object.

---

## Internal Annotations

The following annotations use the `internal.snowplane.hupe1980.github.io/` prefix to signal they are **controller-internal** and should not be set or modified by users.

### `creation-initiated`

```yaml
internal.snowplane.hupe1980.github.io/creation-initiated: "true"
```

Set by the reconciler just before issuing a Snowflake `CREATE`. Ensures crash safety: if the controller restarts between `CREATE` and status commit, the annotation tells the next reconcile that the resource was created by Snowplane (not pre-existing), avoiding incorrect adoption-or-reject behavior.

### `late-initialized`

```yaml
internal.snowplane.hupe1980.github.io/late-initialized: "true"
```

Set after adopting an existing Snowflake resource. Indicates that `status.atProvider` fields were populated from the live Snowflake object during adoption.
