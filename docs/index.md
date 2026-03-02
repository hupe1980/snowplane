---
layout: default
title: Home
nav_order: 1
permalink: /
---

# Snowplane
{: .fs-9 }

Kubernetes-native control plane for Snowflake.
{: .fs-6 .fw-300 }

Manage Snowflake resources declaratively as Kubernetes custom resources — similar in spirit to [AWS ACK](https://aws-controllers-k8s.github.io/community/) and [Crossplane](https://crossplane.io). Define your Snowflake infrastructure as CRDs and let the operator reconcile them.
{: .fs-5 .fw-300 }

[Get Started](/snowplane/getting-started/){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View on GitHub](https://github.com/hupe1980/snowplane){: .btn .fs-5 .mb-4 .mb-md-0 }

---

## Custom Resources

Full lifecycle management for every resource — create, alter, drop, drift detection, adoption, and deletion policies.

| Category | Resources |
|:---------|:----------|
| **Core Infrastructure** | Database, Schema, Warehouse |
| **Data Objects** | Table, View, MaterializedView, Stage, StreamOnTable, StreamOnView, StreamOnExternalTable, StreamOnDirectoryTable, StreamOnDynamicTable, DynamicTable, FileFormat, Pipe, Sequence, ExternalTable |
| **Identity & Access** | User, AccountRole, DatabaseRole, AccountRoleGrant, DatabaseRoleGrant, AccountRoleAssignment, DatabaseRoleAssignment, ShareGrant, GrantOwnership |
| **Orchestration** | Task, Alert |
| **Integrations** | StorageIntegration, SecurityIntegration, NotificationIntegration |
| **Security & Governance** | NetworkPolicy, NetworkRule, PasswordPolicy, MaskingPolicy, RowAccessPolicy, Tag, ResourceMonitor |
| **Utilities** | FieldExport |

---

## Key Capabilities

<div class="code-example" markdown="1">

**Observe-Diff-Apply Reconciliation**
: Only altered fields are pushed to Snowflake — minimizing API calls and avoiding unnecessary mutations.

**Drift Detection & Correction**
: Field-level drift detection with structured reporting. Use `detect-only` policy for monitoring without correction.

**Cross-Resource References**
: Schemas reference Databases; Tables, Views, MaterializedViews, and Stages reference Schemas. Dependency resolution and backoff are automatic.

**Resource Adoption**
: Adopt pre-existing Snowflake objects via a single annotation — no data migration needed.

**Immutable Field Enforcement**
: Two layers of protection — CRD-level CEL validation and reconciler-level error reporting.

**Three Authentication Methods**
: RSA key pair, username/password, or Workload Identity Federation (EKS, GKE, AKS).

</div>

---

## Quick Start

### Install CRDs and run the operator

```bash
git clone https://github.com/hupe1980/snowplane.git && cd snowplane
kubectl apply -f config/crd/bases/
just build && ./bin/manager
```

### Configure a ProviderConfig

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  account: "your-account"
  user: "your-user"
  role: "SYSADMIN"
  authenticationType: KeyPair
  credentials:
    secretRef:
      name: snowflake-credentials
      key: privateKey
```

### Create a Database

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: analytics
spec:
  name: ANALYTICS
  comment: "Managed by Snowplane"
  providerRef:
    name: default
```

```bash
kubectl apply -f database.yaml
kubectl get databases
```

{: .note }
> For production deployments, use the [Helm chart](/snowplane/helm-chart/) instead of running the binary directly.

---

## Architecture

```
┌──────────────────────────────────────────────────┐
│                Kubernetes Cluster                 │
│                                                   │
│  ┌────────────────┐   ┌────────────────────────┐  │
│  │ ProviderConfig │   │   Snowflake Resource   │  │
│  │      CR        │   │    Custom Resources    │  │
│  └──────┬─────────┘   └────────┬───────────────┘  │
│         │                      │                   │
│  ┌──────▼──────────────────────▼──────────────────┐│
│  │          Snowplane Controller Manager          ││
│  │                                                ││
│  │  ┌─────────────────────────────────────────┐   ││
│  │  │  Reference Resolver                     │   ││
│  │  │  databaseRef/schemaRef → Snowflake FQN  │   ││
│  │  └─────────────────────────────────────────┘   ││
│  │                                                ││
│  │  ┌─────────────────────────────────────────┐   ││
│  │  │  Resource Controllers                   │   ││
│  │  │  Observe → Diff → Apply                 │   ││
│  │  └─────────────────────────────────────────┘   ││
│  │                                                ││
│  │  ┌──────────────────┐ ┌────────────────────┐   ││
│  │  │  FieldExport     │ │  Drift Engine      │   ││
│  │  └──────────────────┘ └────────────────────┘   ││
│  └────────────────────┬───────────────────────────┘│
│                       │                            │
└───────────────────────┼────────────────────────────┘
                        │
               ┌────────▼────────┐
               │    Snowflake    │
               │     Account    │
               └─────────────────┘
```

---

## Observability

Built-in Prometheus metrics, structured logging, Kubernetes events, and a Grafana dashboard.

| Metric | Type | Description |
|:-------|:-----|:------------|
| `snowplane_reconcile_duration_seconds` | Histogram | End-to-end reconciliation latency |
| `snowplane_snowflake_operation_duration_seconds` | Histogram | Individual Snowflake SQL operation latency |
| `snowplane_reconcile_total` | Counter | Total reconciliations (success/error) |
| `snowplane_snowflake_operation_total` | Counter | Total Snowflake operations |
| `snowplane_drift_detected_total` | Counter | Drift detection events |
| `snowplane_account_rate_limit_waits_total` | Counter | Per-account aggregate rate limiter waits |
| `snowplane_providerconfig_healthy` | Gauge | Provider health (1 = healthy) |

[Full metrics reference →](/snowplane/observability/) \| [API Reference →](/snowplane/api-reference/)

---

## License

Snowplane is licensed under the [Apache License 2.0](https://github.com/hupe1980/snowplane/blob/main/LICENSE).
