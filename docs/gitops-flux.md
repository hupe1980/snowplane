---
layout: default
title: GitOps with Flux
parent: Guides
nav_order: 5
description: "Manage Snowplane resources using Flux with native health checks and dependency ordering."
---

# GitOps with Flux
{: .fs-8 }

Manage Snowplane resources using [Flux](https://fluxcd.io/) with native health checks, `dependsOn` ordering, and notifications.
{: .fs-5 .fw-300 }

---

## Health Checks

Flux natively supports `status.conditions` with `type: Ready`. Since all Snowplane CRDs follow this convention, health checks work automatically:

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

---

## Dependency Ordering

Use [Kustomization dependencies](https://fluxcd.io/flux/components/kustomize/kustomizations/#dependencies) to control creation order:

```yaml
# 1. Provider configuration
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
# 2. Account-level resources
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
# 3. Database-scoped resources
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
  timeout: 10m
---
# 4. Schema-scoped resources
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
  timeout: 10m
---
# 5. Grants and field exports
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
```

{: .note }
> Snowplane controllers already handle dependency ordering internally. Flux `dependsOn` is complementary — it prevents Flux from applying manifests before their dependencies exist.

---

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
    name: snowflake-gitops-auth
```

---

## Notifications

Configure alerts for Snowplane resource failures:

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

---

## Using FieldExport with Flux

FieldExport passes data between Snowplane resources and other Kubernetes workloads without hard-coding values:

```yaml
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

---

## Directory Structure

Recommended repository layout:

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
│       ├── kustomization.yaml
│       └── patches/
└── README.md
```
