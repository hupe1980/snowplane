---
layout: default
title: Resource Dependencies
parent: Guides
nav_order: 7
description: "Understand how Snowplane handles resource ordering, dependency resolution, and cross-resource data flow."
---

# Resource Dependencies & Ordering
{: .fs-8 }

How Snowplane manages resource dependencies — from built-in readiness gating to multi-resource orchestration.
{: .fs-5 .fw-300 }

---

## Overview

Snowflake resources have natural dependencies — a Schema lives inside a Database, a Table lives inside a Schema, a Grant references a Role. Snowplane provides multiple layers of dependency management:

| Layer | Mechanism | When to Use |
|:------|:----------|:------------|
| **Built-in** | `databaseRef` / `schemaRef` readiness gating | Always — zero configuration |
| **Pre-flight** | Auto database/schema existence checks | Automatic for raw `databaseName`/`schemaName` |
| **FieldExport** | Cross-resource data flow via ConfigMaps/Secrets | Share computed values between resources |
| **kro** | DAG-based multi-resource orchestration | Bundle resources into self-service APIs |
| **GitOps** | Sync waves (Argo CD) / `dependsOn` (Flux) | Complement built-in ordering in CI/CD |

---

## Built-in Reference Resolution

### How It Works

Schema-scoped resources (Tables, Views, Stages, etc.) reference their parent Database and Schema using **CR references** or **raw names**:

```yaml
# Option A: CR reference (recommended)
spec:
  databaseRef:
    name: analytics-db        # References a Database CR
    namespace: infra           # Optional — cross-namespace
  schemaRef:
    name: analytics-schema     # References a Schema CR

# Option B: Raw name (external databases)
spec:
  databaseName: ANALYTICS      # Snowflake identifier directly
  schemaName: PUBLIC
```

**CR references** (`databaseRef`, `schemaRef`) provide automatic readiness gating:

1. The reconciler resolves the reference to the target CR
2. It checks the target CR's `Ready` condition
3. If the target is **not ready**, the reconciler sets `DependencyNotReady` and requeues
4. Only when the target is `Ready=True` does reconciliation proceed

This means you can apply a Database and a Table simultaneously — the Table will wait for the Database to become ready before attempting to create in Snowflake.

### Cross-Namespace References

CR references support cross-namespace resolution. A Schema in namespace `team-a` can reference a Database in namespace `infra`:

```yaml
apiVersion: snowplane.io/v1alpha1
kind: Schema
metadata:
  name: team-a-schema
  namespace: team-a
spec:
  name: TEAM_A_SCHEMA
  databaseRef:
    name: shared-db
    namespace: infra          # Cross-namespace reference
```

{: .note }
> Cross-namespace references require appropriate RBAC permissions. The Snowplane controller needs `get` access to the referenced resource in the target namespace.

---

## Pre-flight Checks

### Automatic Validation

When raw `databaseName` or `schemaName` strings are used (instead of CR references), the reconciler automatically validates that the referenced database/schema exists in Snowflake **before** attempting the main operation.

```yaml
spec:
  databaseName: ANALYTICS    # Pre-flight: SHOW DATABASES LIKE 'ANALYTICS'
  schemaName: RAW_DATA       # Pre-flight: SHOW SCHEMAS LIKE 'RAW_DATA' IN DATABASE "ANALYTICS"
```

If the database or schema doesn't exist:

```
STATUS:
  conditions:
  - type: Ready
    status: "False"
    reason: DependencyNotReady
    message: "pre-flight check failed: database not found in Snowflake: \"ANALYTICS\""
```

The resource requeues with backoff until the dependency exists. This makes the raw-name path functionally equivalent to the CR reference path for error reporting.

### Error Handling

Pre-flight checks distinguish between **definitive** and **non-definitive** errors:

| Error Type | Example | Behavior |
|:-----------|:--------|:---------|
| Definitive "not found" | `SHOW DATABASES` returns 0 rows | Hard fail → `DependencyNotReady` condition |
| Non-definitive | Connection timeout, auth failure | Skipped → reconcile proceeds normally |

Non-definitive errors are skipped because the subsequent Snowflake operations will surface the same connectivity issues with proper error handling. Pre-flight checks are an optimization, not a gate for infrastructure errors.

---

## FieldExport

### Cross-Resource Data Flow

`FieldExport` reads a status field from one Snowplane resource and writes it to a ConfigMap or Secret. This enables cross-resource data flow without hardcoding values.

**Example:** Export a Database's Snowflake name to a ConfigMap, then reference it in a Terraform job:

```yaml
# 1. Database CR
apiVersion: snowplane.io/v1alpha1
kind: Database
metadata:
  name: analytics-db
spec:
  name: ANALYTICS

---
# 2. Export the database name to a ConfigMap
apiVersion: snowplane.io/v1alpha1
kind: FieldExport
metadata:
  name: export-db-name
spec:
  from:
    resource:
      kind: Database
      name: analytics-db
    path: .status.showOutput.name
  to:
    configMap:
      name: snowflake-config
      key: database-name
```

### Common Patterns

| Pattern | Source Field | Use Case |
|:--------|:------------|:---------|
| Database name | `.status.showOutput.name` | Pass to downstream consumers |
| Schema name | `.status.showOutput.name` | Configure application schemas |
| Warehouse name | `.status.showOutput.name` | Set warehouse for CI/CD jobs |
| Account role | `.status.showOutput.name` | Configure RBAC for external tools |
| Connection URL | `.status.showOutput.name` | Build connection strings |

### Security

FieldExport enforces **same-namespace security** — the source resource, target ConfigMap/Secret, and the FieldExport itself must all reside in the same namespace. This prevents cross-namespace privilege escalation.

For cross-namespace data flow, combine FieldExport with Kubernetes mechanisms like:
- **Reflectors** (e.g., [Reflector](https://github.com/emberstack/kubernetes-reflector)) to sync ConfigMaps/Secrets across namespaces
- **External Secrets** for secrets management

### FieldExport + kro

When using kro to bundle resources, FieldExport can bridge between the kro-managed stack and external consumers:

```yaml
# In your RGD, export outputs for external consumption
apiVersion: snowplane.io/v1alpha1
kind: FieldExport
metadata:
  name: ${schema.metadata.name}-export
spec:
  from:
    resource:
      kind: Schema
      name: ${schema.metadata.name}
    path: .status.showOutput.name
  to:
    configMap:
      name: ${instance.metadata.name}-outputs
      key: schema-name
```

{: .tip }
> Within a kro RGD, you often don't need FieldExport — kro's CEL expressions can wire values directly between resources using `${resource.status.field}` syntax. Use FieldExport when you need to pass values **outside** the RGD boundary.

---

## Dependency Patterns

### Pattern 1: Linear Stack (Database → Schema → Table)

The most common pattern — each resource depends on the previous:

```yaml
apiVersion: snowplane.io/v1alpha1
kind: Database
metadata:
  name: analytics-db
spec:
  name: ANALYTICS
---
apiVersion: snowplane.io/v1alpha1
kind: Schema
metadata:
  name: analytics-schema
spec:
  name: RAW_DATA
  databaseRef:
    name: analytics-db          # Waits for Database
---
apiVersion: snowplane.io/v1alpha1
kind: Table
metadata:
  name: events-table
spec:
  name: EVENTS
  databaseRef:
    name: analytics-db
  schemaRef:
    name: analytics-schema      # Waits for Schema (and transitively, Database)
  columns:
    - name: ID
      type: NUMBER(38,0)
    - name: PAYLOAD
      type: VARIANT
```

Apply all three simultaneously — Snowplane handles ordering:

```bash
kubectl apply -f analytics-stack/
```

### Pattern 2: Fan-out (Role → N Grants)

One parent resource with multiple dependent resources:

```yaml
apiVersion: snowplane.io/v1alpha1
kind: AccountRole
metadata:
  name: reader-role
spec:
  name: ANALYTICS_READER
---
apiVersion: snowplane.io/v1alpha1
kind: GrantPrivilegesToAccountRole
metadata:
  name: grant-usage-db
spec:
  accountRoleRef:
    name: reader-role           # Waits for Role
  privilege: USAGE
  on:
    accountObject:
      objectType: DATABASE
      objectName: ANALYTICS
---
apiVersion: snowplane.io/v1alpha1
kind: GrantPrivilegesToAccountRole
metadata:
  name: grant-select-tables
spec:
  accountRoleRef:
    name: reader-role           # Waits for Role
  privilege: SELECT
  on:
    schemaObject:
      future:
        objectType: TABLE
        inSchema: '"ANALYTICS"."PUBLIC"'
```

### Pattern 3: Cross-Namespace (Platform + Team)

Platform team manages shared infrastructure, project teams manage their resources:

```
infra/                          team-a/
├── providerconfig.yaml         ├── schema.yaml (databaseRef → infra/shared-db)
├── database.yaml               ├── table.yaml
└── warehouse.yaml              └── view.yaml
```

```yaml
# team-a/schema.yaml
apiVersion: snowplane.io/v1alpha1
kind: Schema
metadata:
  name: team-a-schema
  namespace: team-a
spec:
  name: TEAM_A
  providerRef:
    name: snowflake-prod
    namespace: infra
  databaseRef:
    name: shared-db
    namespace: infra
```

### Pattern 4: kro Composition

Bundle everything into a single custom resource:

```yaml
apiVersion: kro.run/v1alpha1
kind: SnowflakeProject
metadata:
  name: analytics
spec:
  name: analytics
  environment: prod
  warehouseSize: MEDIUM
```

See the [kro guide](kro.md) for full examples and ready-to-use compositions.

---

## GitOps Integration

### Argo CD — Sync Waves

Snowplane's built-in readiness gating handles dependency ordering, but sync waves can prevent Argo CD from showing transient sync failures:

```yaml
metadata:
  annotations:
    argocd.argoproj.io/sync-wave: "1"    # Database first
---
metadata:
  annotations:
    argocd.argoproj.io/sync-wave: "2"    # Schema after Database
---
metadata:
  annotations:
    argocd.argoproj.io/sync-wave: "3"    # Tables after Schema
```

See the [Argo CD guide](gitops-argocd.md) for detailed setup.

### Flux — dependsOn

Flux Kustomizations support explicit dependency ordering:

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: schemas
spec:
  dependsOn:
    - name: databases
```

See the [Flux guide](gitops-flux.md) for detailed setup.

{: .important }
> Snowplane controllers already handle dependency ordering internally via `databaseRef`/`schemaRef` resolution. GitOps ordering mechanisms are **complementary** — they prevent the GitOps tool from reporting false failures while dependencies are being created, but are not strictly required.

---

## Decision Guide

| Scenario | Recommended Approach |
|:---------|:--------------------|
| Simple stack (DB → Schema → Table) | `databaseRef` / `schemaRef` — zero config |
| External Snowflake database (not managed by Snowplane) | Raw `databaseName` + auto pre-flight |
| Cross-namespace shared infrastructure | Cross-namespace `databaseRef` / `schemaRef` |
| Self-service developer API | kro RGD |
| Export values to external systems | FieldExport |
| CI/CD with Argo CD | Sync waves + built-in refs |
| CI/CD with Flux | `dependsOn` + built-in refs |
| Complex multi-resource bundle | kro RGD or Helm chart |

---

## Further Reading

- [kro (Kube Resource Orchestrator)](kro.md)
- [GitOps with Argo CD](gitops-argocd.md)
- [GitOps with Flux](gitops-flux.md)
- [Architecture — Pre-flight Checks](architecture.md#pre-flight-checks)
- [API Reference — FieldExport](api-reference.md)
- [Production Guide — Resource Scoping](production-guide.md#resource-scoping)
