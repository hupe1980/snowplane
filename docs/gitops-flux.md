# GitOps with Flux

This guide describes how to manage Snowplane resources using [Flux](https://fluxcd.io/).

## Health Checks

Flux natively supports Kubernetes-standard `status.conditions` with `type: Ready`. Since all Snowplane CRDs follow this convention, health checks work automatically when you set `spec.healthChecks` on your Kustomization:

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: snowflake-databases
  namespace: flux-system
spec:
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: snowflake-infra
  path: ./databases
  prune: true
  healthChecks:
    - apiVersion: snowplane.hupe1980.github.io/v1alpha1
      kind: Database
      name: analytics-db
      namespace: snowplane-system
  timeout: 10m
```

Flux will wait for the `Ready` condition to become `True` before marking the Kustomization as ready.

## Dependency Ordering with `dependsOn`

Use Flux [Kustomization dependencies](https://fluxcd.io/flux/components/kustomize/kustomizations/#dependencies) to ensure resources are created in the correct order. Split your resources into separate Kustomizations and wire them with `dependsOn`:

```yaml
# 1. Provider configuration — no dependencies
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: snowflake-provider
  namespace: flux-system
spec:
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: snowflake-infra
  path: ./provider
  prune: true
  healthChecks:
    - apiVersion: snowplane.hupe1980.github.io/v1alpha1
      kind: ProviderConfig
      name: default
      namespace: snowplane-system
  timeout: 5m
---
# 2. Account-level resources — depends on provider
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: snowflake-databases
  namespace: flux-system
spec:
  dependsOn:
    - name: snowflake-provider
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: snowflake-infra
  path: ./databases
  prune: true
  healthChecks:
    - apiVersion: snowplane.hupe1980.github.io/v1alpha1
      kind: Database
      name: analytics-db
      namespace: snowplane-system
  timeout: 10m
---
# 3. Database-scoped resources — depends on databases
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: snowflake-schemas
  namespace: flux-system
spec:
  dependsOn:
    - name: snowflake-databases
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: snowflake-infra
  path: ./schemas
  prune: true
  healthChecks:
    - apiVersion: snowplane.hupe1980.github.io/v1alpha1
      kind: Schema
      name: raw-schema
      namespace: snowplane-system
  timeout: 10m
---
# 4. Schema-scoped resources — depends on schemas
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: snowflake-tables
  namespace: flux-system
spec:
  dependsOn:
    - name: snowflake-schemas
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: snowflake-infra
  path: ./tables
  prune: true
  healthChecks:
    - apiVersion: snowplane.hupe1980.github.io/v1alpha1
      kind: Table
      name: events-table
      namespace: snowplane-system
  timeout: 10m
---
# 5. Grants and field exports — depends on all resource types
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: snowflake-grants
  namespace: flux-system
spec:
  dependsOn:
    - name: snowflake-databases
    - name: snowflake-schemas
    - name: snowflake-tables
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: snowflake-infra
  path: ./grants
  prune: true
  timeout: 10m
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: snowflake-fieldexports
  namespace: flux-system
spec:
  dependsOn:
    - name: snowflake-databases
    - name: snowflake-schemas
    - name: snowflake-tables
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: snowflake-infra
  path: ./fieldexports
  prune: true
  timeout: 10m
```

> **Note:** Snowplane controllers already handle dependency ordering internally (e.g., Schema waits for its Database to become Ready). Flux `dependsOn` is complementary — it prevents Flux from applying manifests before their dependencies exist, avoiding transient reconciliation errors.

## GitRepository Source

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: snowflake-infra
  namespace: flux-system
spec:
  interval: 1m
  url: https://github.com/your-org/snowflake-gitops.git
  ref:
    branch: main
  secretRef:
    name: snowflake-gitops-auth  # optional: for private repos
```

## Notifications and Alerts

Configure Flux notifications to alert on Snowplane resource failures:

```yaml
apiVersion: notification.toolkit.fluxcd.io/v1beta3
kind: Alert
metadata:
  name: snowflake-alerts
  namespace: flux-system
spec:
  providerRef:
    name: slack
  eventSeverity: error
  eventSources:
    - kind: Kustomization
      name: snowflake-*
  inclusionList:
    - ".*reconciliation.*failed.*"
---
apiVersion: notification.toolkit.fluxcd.io/v1beta3
kind: Provider
metadata:
  name: slack
  namespace: flux-system
spec:
  type: slack
  channel: snowflake-ops
  secretRef:
    name: slack-webhook-url
```

## Directory Structure

A recommended repository layout for Flux-managed Snowflake infrastructure:

```
snowflake-gitops/
├── provider/
│   └── providerconfig.yaml
├── databases/
│   └── analytics.yaml
├── warehouses/
│   └── compute.yaml
├── schemas/
│   └── raw.yaml
├── tables/
│   └── events.yaml
├── roles/
│   ├── analyst.yaml
│   └── engineer.yaml
├── grants/
│   └── analyst-grants.yaml
├── fieldexports/
│   └── db-name.yaml
├── clusters/
│   └── production/
│       ├── kustomization.yaml    # Flux Kustomizations for this cluster
│       └── patches/              # Environment-specific patches
└── README.md
```

## Using FieldExport with Flux

FieldExport is particularly useful in GitOps workflows where you need to pass data between Snowplane resources and other Kubernetes workloads without hard-coding values:

```yaml
# Export the database name to a ConfigMap
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: FieldExport
metadata:
  name: db-name-export
spec:
  from:
    resource:
      kind: Database
      name: analytics-db
    path: ".status.showOutput.name"
  to:
    kind: ConfigMap
    name: app-config
    key: SNOWFLAKE_DATABASE
---
# Your application can then mount the ConfigMap
apiVersion: apps/v1
kind: Deployment
metadata:
  name: data-pipeline
spec:
  template:
    spec:
      containers:
        - name: pipeline
          envFrom:
            - configMapRef:
                name: app-config
```

This approach keeps your application configuration fully declarative and automatically synchronized with the actual Snowflake infrastructure state.
