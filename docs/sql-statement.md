---
layout: default
title: SQLStatement (Escape Hatch)
parent: Guides
nav_order: 10
description: "Execute arbitrary SQL that doesn't map to any first-class CRD."
---

# SQLStatement (Escape Hatch)
{: .fs-8 }

Execute arbitrary SQL that doesn't map to any first-class Snowflake CRD.
{: .fs-5 .fw-300 }

---

## Overview

`SQLStatement` is an **escape-hatch resource** that executes user-provided SQL verbatim. It fills gaps where Snowplane doesn't yet have a dedicated CRD for a specific Snowflake operation — for example, `GRANT` statements, `ALTER ACCOUNT` settings, session parameters, or any DDL/DML not covered by the 60+ first-class resource types.

{: .warning }
> The SQLStatement controller is **disabled by default**. Enable it with the `--enable-sql-statement` flag or `enableSQLStatement: true` in the Helm values.

## Quick Start

### Basic Statement

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: SQLStatement
metadata:
  name: create-audit-table
spec:
  providerRef:
    name: default
  execute: |
    CREATE TABLE IF NOT EXISTS MY_DB.PUBLIC.AUDIT_LOG (
      id INT AUTOINCREMENT,
      event_type STRING,
      event_time TIMESTAMP_LTZ DEFAULT CURRENT_TIMESTAMP()
    )
```

### With Observe and Revert

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: SQLStatement
metadata:
  name: grant-select-on-analytics
spec:
  providerRef:
    name: default
  execute: |
    GRANT SELECT ON ALL TABLES IN SCHEMA MY_DB.ANALYTICS TO ROLE ANALYST_ROLE
  revert: |
    REVOKE SELECT ON ALL TABLES IN SCHEMA MY_DB.ANALYTICS FROM ROLE ANALYST_ROLE
  observe: |
    SHOW GRANTS ON SCHEMA MY_DB.ANALYTICS
  observeExpect:
    - column: privilege
      value: SELECT
    - column: grantee_name
      value: ANALYST_ROLE
```

## Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `execute` | `string` | ✅ | SQL to run when the resource is created. Supports multiple statements separated by `;` (respects single-quoted string literals). |
| `revert` | `string` | ❌ | SQL to run when the resource is deleted. If omitted, deletion is a no-op in Snowflake. |
| `observe` | `string` | ❌ | SQL to run periodically to check if the executed state still holds. |
| `observeExpect` | `[]object` | ❌ | Column/value pairs that the observe query result must satisfy. Requires `observe` to be set. |
| `idempotent` | `bool` | ❌ | Informational metadata indicating whether the execute SQL is safe to run multiple times. Default `false`. |
| `useRole` | `string` | ❌ | Snowflake role to `USE ROLE` before execution. Immutable after creation. |
| `dangerousAllowDestructive` | `bool` | ❌ | Must be `true` when `execute` contains `DROP`, `TRUNCATE`, `DELETE`, or `REMOVE`. Revert SQL is exempt. |

## Lifecycle

### Create

1. The controller builds an identifier from the K8s object name.
2. If `observe` SQL is set, it runs the observe query to check whether the resource already exists.
3. If the resource doesn't exist (or no observe SQL), the `execute` SQL is run.
4. The SHA-256 hash of `execute` is stored in `status.executeHash`.

### Observe (Steady State)

When `observe` SQL is set, the controller periodically runs the query and checks expectations:

- If all expectations match → resource is up-to-date (Ready).
- If expectations fail → `DriftDetected` condition is set.

Without observe SQL, the resource reaches Ready after the first successful execution. The execute SQL runs exactly once — subsequent reconcile loops detect the stored `status.executeHash` and skip re-execution.

### Delete

If `revert` SQL is set, it is executed when the Kubernetes resource is deleted. If `revert` is not set, the Snowflake side-effects remain intact (no-op deletion).

## Immutability

`spec.execute` is **immutable** after the first execution. Changing it results in a validation error. To change the SQL:

1. Add the `snowplane.hupe1980.github.io/force-new` annotation with value `"true"`.
2. The controller will delete (revert) and recreate the resource with the new SQL.

```yaml
metadata:
  annotations:
    snowplane.hupe1980.github.io/force-new: "true"
```

## Safety Guards

### Destructive SQL Protection

SQL containing `DROP`, `TRUNCATE`, `DELETE`, or `REMOVE` (case-insensitive, word-boundary matched) is blocked in `spec.execute` by default. To allow destructive execute SQL, set `dangerousAllowDestructive: true`.

`spec.revert` is exempt from this check because revert SQL is inherently destructive by design (DROP, REVOKE, etc.).

```yaml
spec:
  dangerousAllowDestructive: true
  execute: DROP TABLE IF EXISTS MY_DB.STAGING.TEMP_IMPORT
```

### Feature Gate

The controller is disabled by default to prevent accidental use. Enable it explicitly:

**Helm:**
```yaml
enableSQLStatement: true
```

**CLI flag:**
```
--enable-sql-statement
```

## Multi-Statement SQL

The `execute` and `revert` fields support multiple SQL statements separated by `;`. The splitter respects single-quoted string literals (e.g., `'hello; world'` is not treated as two statements).

{: .note }
> **Limitation:** Double-quoted identifiers, `$$` blocks, and SQL comments containing semicolons are NOT handled by the splitter. For those cases, use a single SQL statement per resource.

## FieldExport Support

SQLStatement supports [FieldExport]({% link api-reference.md %}), allowing you to export status fields to ConfigMaps or Secrets:

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: FieldExport
metadata:
  name: audit-table-hash
spec:
  from:
    resource:
      kind: SQLStatement
      name: create-audit-table
    path: .status.executeHash
  to:
    kind: ConfigMap
    name: sql-metadata
    key: audit-hash
```

## Status Fields

| Field | Description |
|-------|-------------|
| `status.executeHash` | SHA-256 hash of the execute SQL at creation time |
| `status.observeResult.rowCount` | Number of rows returned by the last observe query |
| `status.observeResult.matched` | Whether all expectations were satisfied |
| `status.fullyQualifiedName` | Set to the K8s object name |

## Use Cases

- **GRANTs and REVOKEs** not covered by the Grant* family of CRDs
- **ALTER ACCOUNT** or **ALTER SESSION** settings
- **Custom DDL** for objects not yet in the CRD catalog (e.g., Cortex resources)
- **Data seeding** — INSERT initial reference data
- **Migration scripts** — one-time schema changes managed via GitOps
