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

### `adoption-policy`

```yaml
snowplane.hupe1980.github.io/adoption-policy: "adopt"
```

Controls what happens when a Snowflake object with the same name already exists at creation time.

| Default | Values | Description |
|:--------|:-------|:------------|
| `fail-if-exists` | `adopt` | Import the existing object — no `CREATE` is issued |
| | `fail-if-exists` | Fail with `Ready=False` if the object already exists |

{: .note }
> After adoption completes, the controller manages the object normally — including drift correction, `ALTER`, and `DROP` on deletion.

---

### `drift-policy`

```yaml
snowplane.hupe1980.github.io/drift-policy: "detect-only"
```

Controls how the controller handles configuration drift between the Kubernetes spec and the live Snowflake object.

| Default | Values | Description |
|:--------|:-------|:------------|
| `correct` | `correct` | Detect and automatically fix drift via `ALTER` |
| | `detect-only` | Detect and report drift in `.status.conditions` but do not correct it |

See [Drift Detection]({% link drift-detection.md %}) for details.

---

### `use-create-or-alter`

```yaml
snowplane.hupe1980.github.io/use-create-or-alter: "true"
```

When `true`, the controller uses Snowflake's atomic `CREATE OR ALTER` statement instead of the two-step `CREATE IF NOT EXISTS` + `ALTER` flow.

| Default | Values |
|:--------|:-------|
| `true` | `true` / `false` |

Supported resource types: Database, Schema, Table, Warehouse, Task, Tag, View, FileFormat, MaskingPolicy, PasswordPolicy, NetworkRule, RowAccessPolicy, User.

{: .note }
> `CREATE OR ALTER` is a Snowflake preview feature. If Snowflake returns an unsupported-feature error, the controller automatically falls back to the two-step flow.

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
