---
layout: default
title: Enterprise Multi-Tenancy
parent: Guides
nav_order: 7
description: "Multi-tenant deployment patterns for enterprise environments with Snowplane."
---

# Enterprise Multi-Tenancy
{: .fs-8 }

Deploy Snowplane in multi-tenant enterprise environments with namespace isolation, cross-namespace references, and Snowflake resource scoping.
{: .fs-5 .fw-300 }

---

## Overview

Snowplane provides a layered isolation model for enterprise multi-tenancy:

| Layer | Mechanism | What It Controls |
|:------|:----------|:-----------------|
| **Kubernetes namespace** | `watchNamespaces`, K8s RBAC | Which namespaces the operator manages |
| **ProviderConfig binding** | `allowedNamespaces`, `allowedNamespaceSelector` | Which namespaces can use a given set of Snowflake credentials |
| **Snowflake scope** | `allowedDatabases`, `allowedSchemas` | Which Snowflake databases/schemas a team can target |
| **Snowflake role** | `spec.role`, `--allowed-roles` | Which Snowflake permissions are available |
| **Cross-namespace refs** | `databaseRef.namespace`, `schemaRef.namespace`, `allowedRefNamespaces` | Shared infrastructure references across namespaces (restricted by `allowedRefNamespaces`) |

---

## Deployment Models

### Model 1: Single Namespace (Simple)

All resources in one namespace. Suitable for small teams or single-project deployments.

```
snowflake/
├── providerconfig.yaml
├── database.yaml
├── schema.yaml
├── table.yaml
└── view.yaml
```

No special configuration needed — all references are namespace-local by default.

### Model 2: Platform + Project Teams (Recommended)

A platform team manages shared infrastructure in a central namespace. Project teams manage their own resources in separate namespaces, referencing shared infrastructure via cross-namespace refs.

```
infra/                              team-analytics/
├── providerconfig.yaml             ├── schema.yaml
├── database.yaml (ANALYTICS_DB)    ├── table.yaml
└── warehouse.yaml (COMPUTE_WH)    └── view.yaml

                                    team-ml/
                                    ├── schema.yaml
                                    ├── table.yaml
                                    └── dynamic-table.yaml
```

```yaml
# infra/providerconfig.yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: snowflake-prod
  namespace: infra
spec:
  account: myorg-myaccount
  user: SNOWPLANE_SVC
  role: SNOWPLANE_ADMIN
  authenticator:
    type: KeyPair
    privateKeySecretRef:
      name: snowflake-credentials
      key: private-key
  # Namespace isolation
  allowedNamespaces:
    - infra
    - team-analytics
    - team-ml
  # Or use label selectors for dynamic environments
  allowedNamespaceSelector:
    matchLabels:
      snowplane.io/tenant: "true"
  # Snowflake scope restrictions
  allowedDatabases:
    - ANALYTICS_DB
    - ML_DB
```

```yaml
# team-analytics/schema.yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Schema
metadata:
  name: raw-data
  namespace: team-analytics
spec:
  name: RAW_DATA
  providerRef:
    name: snowflake-prod
    namespace: infra              # Cross-namespace ProviderConfig
  databaseRef:
    name: analytics-db
    namespace: infra              # Cross-namespace Database reference
```

{: .important }
> Cross-namespace references require the Snowplane controller to have RBAC access to resources in the target namespace. The default Helm chart grants cluster-wide access. For namespace-scoped RBAC, see [Scaling with Namespace Sharding](#namespace-sharding).

### Model 3: ProviderConfig per Team

Each team gets its own ProviderConfig with a dedicated Snowflake role and scoping. Provides the strongest isolation.

```
team-analytics/                     team-ml/
├── providerconfig.yaml             ├── providerconfig.yaml
├── database.yaml                   ├── database.yaml
├── schema.yaml                     ├── schema.yaml
└── table.yaml                      └── dynamic-table.yaml
```

```yaml
# team-analytics/providerconfig.yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: analytics-pc
  namespace: team-analytics
spec:
  account: myorg-myaccount
  user: ANALYTICS_SVC
  role: ANALYTICS_ROLE             # Least-privilege Snowflake role
  authenticator:
    type: KeyPair
    privateKeySecretRef:
      name: analytics-credentials
      key: private-key
  allowedNamespaces:
    - team-analytics               # Only this namespace
  allowedDatabases:
    - ANALYTICS_DB                 # Only this database
  allowedSchemas:
    - ANALYTICS_DB.RAW
    - ANALYTICS_DB.CURATED
```

---

## Isolation Mechanisms

### Namespace Binding

Control which Kubernetes namespaces can use a ProviderConfig:

```yaml
spec:
  # Static list
  allowedNamespaces:
    - team-a
    - team-b

  # Label selector (OR with static list)
  allowedNamespaceSelector:
    matchLabels:
      env: production
    matchExpressions:
      - key: team
        operator: In
        values: [analytics, ml, data-eng]
```

When both `allowedNamespaces` and `allowedNamespaceSelector` are set, a namespace is permitted if it matches **either** mechanism (OR semantics).

{: .warning }
> When `allowedNamespaceSelector` is set without `allowedNamespaces`, only namespaces matching the selector are permitted. This is a deny-by-default model.

### Database & Schema Scoping

Restrict which Snowflake databases and schemas resources may target:

```yaml
spec:
  allowedDatabases:
    - ANALYTICS_DB
    - SHARED_DB
  allowedSchemas:
    - ANALYTICS_DB.PUBLIC
    - ANALYTICS_DB.RAW
    - SHARED_DB.ANALYTICS
```

- **`allowedDatabases`**: Case-insensitive. Empty = all databases allowed.
- **`allowedSchemas`**: Supports `"SCHEMA"` (any database) or `"DATABASE.SCHEMA"` format. Empty = all schemas allowed.
- Resources violating these constraints are rejected with `DatabaseNotAllowed` or `SchemaNotAllowed` condition reasons and do not issue any Snowflake SQL.

### Cross-Namespace References

All `databaseRef`, `schemaRef`, and `providerRef` fields support an optional `namespace` field:

```yaml
spec:
  providerRef:
    name: snowflake-prod
    namespace: infra          # Omit for same-namespace
  databaseRef:
    name: shared-db
    namespace: infra          # Omit for same-namespace
  schemaRef:
    name: public-schema
    namespace: infra          # Omit for same-namespace
```

When `namespace` is omitted, the reference resolves within the same namespace as the referencing resource (backward-compatible).

Cross-namespace references preserve **readiness gating** — the controller checks that the referenced resource exists and has `Ready=True` before proceeding. This prevents opaque SQL errors when dependencies aren't ready yet.

#### Restricting Cross-Namespace References

Use `allowedRefNamespaces` on ProviderConfig to restrict which namespaces cross-namespace refs may target:

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  allowedRefNamespaces:
    - SAME          # Allow refs within the same namespace
    - infra         # Allow refs to the shared infrastructure namespace
  # ...
```

| Value | Semantics |
|:------|:----------|
| *(empty list)* | No restriction — all namespaces allowed (default) |
| `"*"` | All namespaces explicitly allowed |
| `"SAME"` | Only same-namespace references allowed |
| `"ns-a"`, `"ns-b"` | Only the listed namespaces allowed |

Violations are rejected with a `RefNamespaceNotAllowed` condition reason. No Snowflake SQL is issued.

### Snowflake Role Restriction

Restrict which Snowflake roles can be used in ProviderConfig resources:

```yaml
# Helm values
controller:
  allowedRoles: "SYSADMIN,SNOWPLANE_ADMIN,ANALYTICS_ROLE"
```

Any ProviderConfig using a role not in this list is rejected. This prevents teams from escalating to `ACCOUNTADMIN` or `SECURITYADMIN`.

Per-resource role overrides (`spec.useRole`) are validated against the same allowlist.

---

## Pre-flight Validation

The controller automatically validates that referenced Snowflake databases and schemas exist before issuing CREATE commands:

- **CR references** (`databaseRef`/`schemaRef`): Existence validated during reference resolution — the referenced CR must be `Ready=True`.
- **Raw strings** (`databaseName`/`schemaName`): The controller issues `SHOW DATABASES LIKE`/`SHOW SCHEMAS LIKE` queries to verify existence before CREATE.

This provides clear `DependencyNotReady` conditions instead of opaque SQL errors, regardless of whether teams use cross-namespace refs or raw Snowflake identifiers.

---

## GitOps Integration

### Argo CD — Namespace per Team

```
gitops-repo/
├── infra/                    # Platform team — synced by infra AppSet
│   ├── providerconfig.yaml
│   ├── database.yaml
│   └── warehouse.yaml
├── team-analytics/           # Analytics team — synced by team AppSet
│   ├── schema.yaml
│   ├── table.yaml
│   └── view.yaml
└── team-ml/                  # ML team — synced by team AppSet
    ├── schema.yaml
    └── dynamic-table.yaml
```

```yaml
# Argo CD ApplicationSet — one Application per team
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: snowplane-teams
spec:
  generators:
    - git:
        repoURL: https://github.com/myorg/gitops-repo
        directories:
          - path: 'team-*'
  template:
    metadata:
      name: 'snowplane-{{path.basename}}'
    spec:
      destination:
        namespace: '{{path.basename}}'
      source:
        repoURL: https://github.com/myorg/gitops-repo
        path: '{{path}}'
      syncPolicy:
        automated:
          prune: true
```

See the [Argo CD guide](gitops-argocd.md) for sync waves and health checks.

### Flux — Namespace per Team

```yaml
# Flux Kustomization per team
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: team-analytics
  namespace: flux-system
spec:
  sourceRef:
    kind: GitRepository
    name: gitops-repo
  path: ./team-analytics
  targetNamespace: team-analytics
  dependsOn:
    - name: infra      # Ensure infra resources exist first
```

See the [Flux guide](gitops-flux.md) for detailed setup.

---

## Policy Enforcement

### Kyverno — Require Namespace Scoping

```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: require-providerconfig-scoping
spec:
  validationFailureAction: Enforce
  rules:
    - name: require-namespace-restriction
      match:
        resources:
          kinds:
            - ProviderConfig
      validate:
        message: >-
          ProviderConfig must specify allowedNamespaces or
          allowedNamespaceSelector for multi-tenant isolation.
        anyPattern:
          - spec:
              allowedNamespaces: "?*"
          - spec:
              allowedNamespaceSelector: "?*"
```

### Kyverno — Enforce Database Scoping

```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: require-database-scoping
spec:
  validationFailureAction: Enforce
  rules:
    - name: require-allowed-databases
      match:
        resources:
          kinds:
            - ProviderConfig
      validate:
        message: "ProviderConfig must restrict allowedDatabases."
        pattern:
          spec:
            allowedDatabases: "?*"
```

### OPA/Gatekeeper — Role Restriction

See the [Production Guide](production-guide.md#opagatkeeper-constrainttemplate-for-role-restriction) for OPA/Gatekeeper examples.

---

{: .label .label-green }
## Namespace Sharding

For very large deployments (10K+ resources), run multiple Snowplane instances with non-overlapping namespace sets:

```bash
# Instance 1: Analytics teams
helm install snowplane-analytics charts/snowplane/ \
  --set watchNamespaces="team-analytics,team-bi,team-reporting" \
  --set leaderElectionID=snowplane-analytics

# Instance 2: ML teams
helm install snowplane-ml charts/snowplane/ \
  --set watchNamespaces="team-ml,team-data-science" \
  --set leaderElectionID=snowplane-ml
```

Each instance only watches its assigned namespaces, providing horizontal scaling and blast radius isolation.

{: .warning }
> Namespace sets must not overlap — two instances watching the same namespace will conflict on leader election and reconciliation.

---

## Further Reading

- [Resource Dependencies — Cross-Namespace References](resource-dependencies.md#cross-namespace-references)
- [Production Guide — Resource Scoping](production-guide.md#resource-scoping)
- [Production Guide — Security Hardening](production-guide.md#security-hardening)
- [Snowflake Role Setup](snowflake-role-setup.md)
- [kro Compositions](kro.md)
- [Architecture — Pre-flight Checks](architecture.md#pre-flight-checks)
