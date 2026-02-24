---
layout: default
title: GitOps with Argo CD
parent: Guides
nav_order: 4
description: "Manage Snowplane resources using Argo CD with custom health checks and sync waves."
---

# GitOps with Argo CD
{: .fs-8 }

Manage Snowplane resources using [Argo CD](https://argo-cd.readthedocs.io/) with custom health checks, sync waves, and ignorable differences.
{: .fs-5 .fw-300 }

---

## Health Checks

Argo CD needs custom health checks to understand when Snowplane CRDs are healthy. Add the following to your `argocd-cm` ConfigMap — the wildcard pattern works for **all** Snowplane CRDs since they all use the standard `Ready` condition:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-cm
  namespace: argocd
data:
  resource.customizations.health.snowplane.hupe1980.github.io_*: |
    hs = {}
    if obj.status ~= nil and obj.status.conditions ~= nil then
      for _, condition in ipairs(obj.status.conditions) do
        if condition.type == "Ready" then
          if condition.status == "True" then
            hs.status = "Healthy"
            hs.message = condition.message
          else
            hs.status = "Degraded"
            hs.message = condition.message
          end
          return hs
        end
      end
    end
    hs.status = "Progressing"
    hs.message = "Waiting for reconciliation"
    return hs
```

---

## Sync Waves

Use [sync waves](https://argo-cd.readthedocs.io/en/stable/user-guide/sync-waves/) to control resource creation order:

```yaml
# Wave 0: Provider configuration
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
  annotations:
    argocd.argoproj.io/sync-wave: "0"
spec:
  account: "myaccount"
  user: "myuser"
  region: "us-east-1"
  role: "SYSADMIN"
  warehouse: "COMPUTE_WH"
  authenticationType: KeyPair
  credentials:
    secretRef:
      name: snowflake-creds
      key: privateKey
---
# Wave 1: Account-level resources
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: analytics-db
  annotations:
    argocd.argoproj.io/sync-wave: "1"
spec:
  name: ANALYTICS_DB
  providerRef:
    name: default
---
# Wave 2: Database-scoped resources
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Schema
metadata:
  name: raw-schema
  annotations:
    argocd.argoproj.io/sync-wave: "2"
spec:
  name: RAW
  databaseRef:
    name: analytics-db
  providerRef:
    name: default
---
# Wave 3: Schema-scoped resources
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Table
metadata:
  name: events-table
  annotations:
    argocd.argoproj.io/sync-wave: "3"
spec:
  name: EVENTS
  databaseRef:
    name: analytics-db
  schemaRef:
    name: raw-schema
  columns:
    - name: ID
      type: "NUMBER(38,0)"
    - name: EVENT_TYPE
      type: "VARCHAR(256)"
    - name: CREATED_AT
      type: TIMESTAMP_NTZ
  providerRef:
    name: default
---
# Wave 4: Grants and field exports
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: AccountRoleGrant
metadata:
  name: analyst-usage
  annotations:
    argocd.argoproj.io/sync-wave: "4"
spec:
  privilege: USAGE
  on:
    accountObject:
      objectType: DATABASE
      objectName: ANALYTICS_DB
  accountRole: ANALYST_ROLE
  providerRef:
    name: default
```

{: .note }
> Snowplane controllers already handle dependency ordering via `databaseRef`/`schemaRef` resolution. Sync waves are complementary — they prevent Argo CD from showing false sync failures while dependencies are being created.

---

## Ignorable Differences

Tell Argo CD to ignore controller-managed fields:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-cm
  namespace: argocd
data:
  resource.customizations.ignoreDifferences.snowplane.hupe1980.github.io_*: |
    jqPathExpressions:
      - .status
      - .metadata.finalizers
      - .metadata.annotations["snowplane.hupe1980.github.io/tracked-parameters"]
      - .metadata.annotations["snowplane.hupe1980.github.io/last-applied-spec-hash"]
```

---

## Application Example

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: snowflake-infrastructure
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/your-org/snowflake-gitops.git
    targetRevision: main
    path: environments/production
  destination:
    server: https://kubernetes.default.svc
    namespace: snowplane-system
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
    retry:
      limit: 5
      backoff:
        duration: 5s
        factor: 2
        maxDuration: 3m
```

---

## Directory Structure

Recommended repository layout:

```
snowflake-gitops/
├── base/
│   ├── kustomization.yaml
│   ├── providerconfig.yaml
│   ├── databases/
│   │   └── analytics.yaml
│   ├── schemas/
│   │   └── raw.yaml
│   ├── warehouses/
│   │   └── compute.yaml
│   ├── roles/
│   │   ├── analyst.yaml
│   │   └── engineer.yaml
│   ├── grants/
│   │   └── analyst-grants.yaml
│   └── fieldexports/
│       └── db-name.yaml
├── environments/
│   ├── production/
│   │   └── kustomization.yaml
│   └── staging/
│       └── kustomization.yaml
└── README.md
```
