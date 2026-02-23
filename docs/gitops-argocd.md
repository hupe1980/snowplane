# GitOps with Argo CD

This guide describes how to manage Snowplane resources using [Argo CD](https://argo-cd.readthedocs.io/).

## Health Checks

Argo CD needs custom health checks to understand when Snowplane CRDs are healthy. Add the following Lua scripts to your `argocd-cm` ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-cm
  namespace: argocd
data:
  resource.customizations.health.snowplane.hupe1980.github.io_Database: |
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

The same health check pattern works for **all** Snowplane CRDs since they all use the standard `Ready` condition. You can apply it to all types at once using a wildcard:

```yaml
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

## Sync Waves

Use Argo CD [sync waves](https://argo-cd.readthedocs.io/en/stable/user-guide/sync-waves/) to control the order of resource creation. Snowplane resources often have dependencies (e.g., Schema depends on Database).

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
# Wave 1: Account-level resources (databases, warehouses, roles)
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
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Warehouse
metadata:
  name: compute-wh
  annotations:
    argocd.argoproj.io/sync-wave: "1"
spec:
  name: COMPUTE_WH
  warehouseSize: XSMALL
  providerRef:
    name: default
---
# Wave 2: Database-scoped resources (schemas, database roles)
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
# Wave 3: Schema-scoped resources (tables, views, stages)
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
kind: Grant
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
  to:
    accountRole: ANALYST_ROLE
  providerRef:
    name: default
---
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: FieldExport
metadata:
  name: db-name-export
  annotations:
    argocd.argoproj.io/sync-wave: "4"
spec:
  from:
    resource:
      kind: Database
      name: analytics-db
    path: ".status.showOutput.name"
  to:
    kind: ConfigMap
    name: snowplane-exports
    key: database-name
```

> **Note:** Snowplane controllers already handle dependency ordering via `databaseRef` / `schemaRef` resolution (waiting for parents to become Ready). Sync waves are complementary — they prevent Argo CD from showing false sync failures while dependencies are being created.

## Ignorable Differences

Snowplane controllers manage `status`, `metadata.finalizers`, and `metadata.annotations` (tracked parameters). Tell Argo CD to ignore these:

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

## Directory Structure

A recommended repository layout for GitOps-managed Snowflake infrastructure:

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
│   │   └── kustomization.yaml     # patches for prod
│   └── staging/
│       └── kustomization.yaml     # patches for staging
└── README.md
```
