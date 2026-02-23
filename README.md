<p align="center">
  <h1 align="center">❄️✈️ Snowplane</h1>
  <p align="center">
    <strong>Kubernetes-native control plane for Snowflake</strong>
  </p>
  <p align="center">
    <a href="https://github.com/hupe1980/snowplane/actions/workflows/ci.yml"><img src="https://github.com/hupe1980/snowplane/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <a href="https://goreportcard.com/report/github.com/hupe1980/snowplane"><img src="https://goreportcard.com/badge/github.com/hupe1980/snowplane" alt="Go Report Card"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License"></a>
  </p>
</p>

---

**Snowplane** manages Snowflake resources declaratively as Kubernetes custom resources — similar in spirit to [AWS ACK](https://aws-controllers-k8s.github.io/community/) and [Crossplane](https://crossplane.io). Define your Snowflake infrastructure as CRDs and let the operator reconcile them.

## ✨ Features

### 🏗️ Resource Management (21 CRDs)

| Category | Resources |
|----------|-----------|
| 🗄️ **Core Infrastructure** | Database, Schema, Warehouse |
| 📊 **Data Objects** | Table, View, Stage, Stream |
| 🎭 **Identity & Access** | User, AccountRole, DatabaseRole, AccountRoleGrant, DatabaseRoleGrant, ShareGrant, GrantOwnership |
| ⏰ **Orchestration** | Task (DAG scheduling, serverless or warehouse-backed) |
| 🛡️ **Security & Governance** | NetworkPolicy, MaskingPolicy, RowAccessPolicy, Tag, ResourceMonitor |
| 📤 **Utilities** | FieldExport (copy status fields into ConfigMaps/Secrets) |

Every resource supports full lifecycle management (create, alter, drop), drift detection, adoption of pre-existing objects, and deletion policies. See the [API Reference](#-api-reference) below for detailed field documentation.

### 🔧 Operator Capabilities

- 🔗 **Cross-Resource References** — Schemas → Databases; Tables, Views, Stages → Databases + Schemas with automatic dependency resolution and backoff
- 🔄 **Observe-Diff-Apply Reconciliation** — Only altered fields are pushed to Snowflake, minimizing API calls
- 🔍 **Drift Detection & Correction** — Field-level drift detection with structured reporting, detect-only policy option
- 🏷️ **Resource Adoption** — Adopt pre-existing Snowflake resources via `adoption-policy: adopt` annotation
- 🔒 **Immutable Field Enforcement** — Three layers: CRD schema (CEL `self == oldSelf`), validating webhooks, reconciler-level errors
- 🛡️ **Dangerous Grant Protection** — Blocks grants to ACCOUNTADMIN/SECURITYADMIN/ORGADMIN and dangerous privileges by default
- ⚛️ **CREATE OR ALTER** — Opt-in atomic `CREATE OR ALTER` for Database & Warehouse via annotation
- 🗑️ **Deletion Policies** — `Delete` (drop resource) or `Orphan` (leave intact)
- 🏷️ **ForceNew Annotation** — Delete+recreate on immutable field changes

### 🔐 Security & Authentication

- 🔑 **Key Pair Authentication** — RSA key pair auth via Kubernetes Secrets
- 🔑 **Username/Password** — Password auth via Kubernetes Secrets
- 🌐 **Workload Identity Federation** — EKS IRSA, GKE WI, AKS WI via projected ServiceAccount tokens
- 🔒 **Sensitive Field Redaction** — Passwords, PEM keys, tokens `[REDACTED]` in all logs and events
- 🛡️ **Validating Webhooks** — Immutable field enforcement, enum/range validation, dangerous-grant blocking
- 🔧 **Mutating Webhooks** — Auto-defaults for `deletionPolicy`, `providerRef`, User `type`

### 📈 Observability & Operations

- 📊 **Custom Prometheus Metrics** — Reconciliation, Snowflake ops, client pool, rate limits, drift, circuit breaker
- 📉 **Grafana Dashboard** — Pre-built dashboard at `config/grafana/snowplane-dashboard.json`
- 🔌 **Connection Pooling** — LRU client cache with config-change rotation and configurable max size
- ⏱️ **Rate Limiting** — Per-provider token-bucket rate limiter (configurable QPS/burst)
- 🔌 **Circuit Breaker** — Per-provider 3-state failure isolation (closed/open/half-open)
- ☸️ **Helm Chart** — Production-ready with CRDs, RBAC, ServiceMonitor, PDB
- 📋 **Condition-based Status** — Ready, Synced, ReferencesResolved, Terminal, DriftDetected, CredentialsInvalid, LateInitialized

## 🏛️ Architecture

```
┌──────────────────────────────────────────────────┐
│                Kubernetes Cluster                 │
│                                                   │
│  ┌──────────────┐   ┌────────────────────────┐   │
│  │ ProviderConfig│   │   Snowflake Resource   │   │
│  │      CR       │   │     Custom Resources   │   │
│  └──────┬────────┘   └────────┬───────────────┘   │
│         │                     │                    │
│  ┌──────▼─────────────────────▼──────────────────┐│
│  │          Snowplane Controller Manager          ││
│  │                                                ││
│  │  ┌──────────────────────────────────────────┐  ││
│  │  │  🔗 Reference Resolver                  │  ││
│  │  │  Resolves databaseRef/schemaRef → FQN    │  ││
│  │  │  Waits for dependency readiness          │  ││
│  │  └──────────────────────────────────────────┘  ││
│  │                                                ││
│  │  ┌──────────────────────────────────────────┐  ││
│  │  │  21 Resource Controllers (see above)     │  ││
│  │  │  Observe → Diff → Apply reconciliation   │  ││
│  │  └──────────────────────────────────────────┘  ││
│  │                                                ││
│  │  ┌──────────────────┐ ┌───────────────────┐   ││
│  │  │ 📤 FieldExport  │ │ 🔍 Drift Engine  │   ││
│  │  └──────────────────┘ └───────────────────┘   ││
│  └────────────────────┬───────────────────────────┘│
│                       │                            │
└───────────────────────┼────────────────────────────┘
                        │
               ┌────────▼────────┐
               │   ❄️ Snowflake  │
               │     Account     │
               └─────────────────┘
```

## 🚀 Quick Start

### 📋 Prerequisites

- Go 1.25+
- A Kubernetes cluster (kind, minikube, or remote)
- A Snowflake account with appropriate permissions

### 📦 Installation

```bash
# Clone the repository
git clone https://github.com/hupe1980/snowplane.git
cd snowplane

# Install CRDs
kubectl apply -f config/crd/bases/

# Build and run locally (outside cluster)
just build
./bin/manager
```

### 🔧 Configure a Provider

Create a Secret with your Snowflake credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: snowflake-credentials
  namespace: snowplane-system
type: Opaque
stringData:
  password: "your-snowflake-password"
```

Create a ProviderConfig:

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  account: "your-account"
  region: "us-east-1"
  user: "your-user"
  role: "SYSADMIN"
  warehouse: "COMPUTE_WH"
  authenticationType: UsernamePassword
  credentials:
    secretRef:
      name: snowflake-credentials
      namespace: snowplane-system
      key: password
```

### 🎉 Create Your First Resource

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: my-database
spec:
  name: MY_DATABASE
  comment: "Managed by Snowplane"
  providerRef:
    name: default
  deletionPolicy: Delete
```

```bash
kubectl apply -f database.yaml
kubectl get databases
```

## ⚙️ Configuration

### 🎛️ Controller Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--metrics-bind-address` | `:8080` | Metrics endpoint bind address |
| `--health-probe-bind-address` | `:8081` | Health/readiness probe bind address |
| `--leader-elect` | `false` | Enable leader election for HA deployments |
| `--leader-election-id` | `snowplane-leader-election` | Leader election identity |
| `--max-concurrent-reconciles` | `3` | Max concurrent reconciles per controller |
| `--rate-limit-qps` | `10` | Sustained queries/sec to Snowflake per provider (0 = disabled) |
| `--rate-limit-burst` | `20` | Max burst size for Snowflake API rate limiter |
| `--enable-webhooks` | `false` | Enable admission webhooks |
| `--webhook-port` | `9443` | Webhook server port |
| `--requeue-interval` | `5m` | Drift detection re-observe interval |
| `--enable-alpha-resources` | `true` | Enable alpha-maturity controllers |
| `--disable-controllers` | `""` | Comma-separated controllers to disable (e.g. `accountrolegrant,stage,view`) |
| `--watch-namespaces` | `""` | Comma-separated namespaces to watch (empty = all) |
| `--development` | `false` | Human-readable debug logging |

### 🏥 Health Probes

| Endpoint | Check | Description |
|----------|-------|-------------|
| `/healthz` | Ping | Returns 200 if the manager process is running |
| `/readyz` | Snowflake connectivity | Pings all cached clients; 200 when all reachable |

### 🏷️ Annotations Reference

| Annotation | Default | Values | Description |
|------------|---------|--------|-------------|
| `snowplane.hupe1980.github.io/force-new` | `false` | `true` / `false` | Delete + recreate on immutable field change |
| `snowplane.hupe1980.github.io/adoption-policy` | `fail-if-exists` | `adopt` / `fail-if-exists` | Control adoption of pre-existing resources |
| `snowplane.hupe1980.github.io/drift-policy` | correct | `detect-only` | Report drift without correcting it |
| `snowplane.hupe1980.github.io/use-create-or-alter` | `true` | `true` / `false` | Atomic `CREATE OR ALTER` (Database, Warehouse) |
| `snowplane.hupe1980.github.io/allow-dangerous-grant` | `false` | `true` / `false` | Allow grants to system roles / dangerous privileges |

### 🏷️ CRD Labels

| Label | Values | Description |
|-------|--------|-------------|
| `snowplane.hupe1980.github.io/maturity` | `alpha` / `beta` / `stable` | CRD maturity classification |

## ☸️ Helm Chart

```bash
helm install snowplane charts/snowplane/ \
  --namespace snowplane-system --create-namespace
```

### Key Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `replicaCount` | `1` | Controller manager replicas |
| `controller.maxConcurrentReconciles` | `3` | Max concurrent reconciles per controller |
| `controller.requeueInterval` | `5m` | Drift detection interval |
| `controller.enableAlphaResources` | `true` | Enable alpha-maturity controllers |
| `rateLimit.qps` | `10` | Snowflake API rate limit per provider |
| `rateLimit.burst` | `20` | Rate limit burst size |
| `leaderElection.enabled` | `true` | Enable leader election |
| `metrics.serviceMonitor.enabled` | `false` | Create Prometheus ServiceMonitor |
| `grafana.dashboard.enabled` | `false` | Deploy Grafana dashboard ConfigMap |
| `webhooks.enabled` | `false` | Enable admission webhooks |
| `watchNamespaces` | `""` | Namespaces to watch (empty = all) |
| `priorityClassName` | `""` | Pod PriorityClass name |
| `topologySpreadConstraints` | `[]` | Topology spread constraints |

CRDs are automatically installed from `charts/snowplane/crds/` on first install.

## 📊 Observability

### Prometheus Metrics

All metrics use the `snowplane_` namespace.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `snowplane_reconcile_total` | Counter | `controller`, `result` | Reconciliation attempts |
| `snowplane_reconcile_duration_seconds` | Histogram | `controller` | Reconciliation loop duration |
| `snowplane_snowflake_operation_total` | Counter | `controller`, `operation`, `result` | Snowflake API call count |
| `snowplane_snowflake_operation_duration_seconds` | Histogram | `controller`, `operation` | Snowflake API call duration |
| `snowplane_client_pool_size` | Gauge | — | Active Snowflake clients in pool |
| `snowplane_rate_limit_waits_total` | Counter | `controller` | Rate limiter wait events |
| `snowplane_adoption_total` | Counter | `controller`, `result` | Adoption outcomes |
| `snowplane_drift_detected_total` | Counter | `controller` | Drift detection events |
| `snowplane_managed_resources` | Gauge | `controller`, `state` | Resources by state (`ready`/`not_ready`/`terminal`) |
| `snowplane_circuit_breaker_trips_total` | Counter | `provider` | Circuit breaker trips |
| `snowplane_circuit_breaker_state` | Gauge | `provider` | Breaker state (0=closed, 1=open, 2=half-open) |

### 📉 Grafana Dashboard

Import `config/grafana/snowplane-dashboard.json` via Grafana UI → Dashboards → Import. Panels include:

- 📈 Reconcile rate & error rate per controller
- ⏱️ Reconcile duration percentiles (P50 / P95 / P99)
- ❄️ Snowflake API latency & operation counts
- 🔌 Client pool size & rate limiter waits
- 🔍 Drift detection frequency per controller
- 🔌 Circuit breaker state & trip frequency

## 📖 API Reference

<details>
<summary>🔌 <strong>ProviderConfig</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.account` | `string` | Snowflake account identifier |
| `spec.region` | `string` | Snowflake cloud region |
| `spec.user` | `string` | Snowflake username |
| `spec.role` | `string` | Snowflake role to assume |
| `spec.warehouse` | `string` | Default warehouse |
| `spec.authenticationType` | `enum` | `KeyPair` / `UsernamePassword` / `WorkloadIdentity` |
| `spec.credentials.secretRef` | `SecretKeyReference` | Reference to credentials Secret |
| `spec.workloadIdentity.audience` | `string` | OIDC audience for WIF |
| `spec.workloadIdentity.tokenFilePath` | `string` | Path to projected SA token file |
| `spec.workloadIdentity.provider` | `enum` | `OIDC` (default) / `AWS` / `GCP` / `Azure` |

</details>

<details>
<summary>🗄️ <strong>Database</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Snowflake database name *(immutable)* |
| `spec.comment` | `*string` | Optional description |
| `spec.dataRetentionTimeInDays` | `*int32` | Time Travel retention (0–90 days) |
| `spec.maxDataExtensionTimeInDays` | `*int32` | Max data extension days (0–90) |
| `spec.transient` | `bool` | Transient database — no Fail-safe *(immutable)* |
| `spec.catalog` | `*string` | Iceberg catalog integration name |
| `spec.externalVolume` | `*string` | External volume for Iceberg tables |
| `spec.replaceInvalidCharacters` | `*bool` | Replace invalid UTF-8 characters |
| `spec.defaultDdlCollation` | `*string` | Default string column collation |
| `spec.storageSerializationPolicy` | `*enum` | `COMPATIBLE` / `OPTIMIZED` |
| `spec.logLevel` | `*enum` | `TRACE` / `DEBUG` / `INFO` / `WARN` / `ERROR` / `FATAL` / `OFF` |
| `spec.metricLevel` | `*enum` | `NONE` / `ALL` |
| `spec.traceLevel` | `*enum` | `ALWAYS` / `ON_EVENT` / `OFF` |
| `spec.useRole` | `*string` | Snowflake role activated via USE ROLE before SQL operations |
| `spec.deletionPolicy` | `enum` | `Delete` (default) / `Orphan` |
| `spec.providerRef.name` | `string` | Name of the ProviderConfig to use |

> 💡 **Nil-means-unmanaged:** Pointer fields (`*string`, `*int32`, `*bool`) use nil to mean "not managed by Snowplane." When nil, the controller skips the parameter in CREATE/ALTER, leaving Snowflake's server-side default intact.

</details>

<details>
<summary>📂 <strong>Schema</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Snowflake schema name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.comment` | `*string` | Optional description |
| `spec.dataRetentionTimeInDays` | `*int32` | Time Travel retention (0–90 days) |
| `spec.maxDataExtensionTimeInDays` | `*int32` | Max data extension days (0–90) |
| `spec.transient` | `bool` | Transient schema *(immutable)* |
| `spec.managedAccess` | `bool` | Managed access schema |
| `spec.storageSerializationPolicy` | `*enum` | `COMPATIBLE` / `OPTIMIZED` |
| `spec.logLevel` / `metricLevel` / `traceLevel` | `*enum` | Same as Database |
| `spec.useRole` | `*string` | Snowflake role for USE ROLE |

> 🔗 **Cross-Resource References:** `databaseRef` points to a Database CR in the same namespace. The controller verifies the Database is Ready before proceeding. If not Ready, the Schema enters `DependencyNotReady` and auto-requeues.

</details>

<details>
<summary>⚡ <strong>Warehouse</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Snowflake warehouse name *(immutable)* |
| `spec.warehouseType` | `*enum` | `STANDARD` / `SNOWPARK-OPTIMIZED` |
| `spec.warehouseSize` | `*enum` | `XSMALL` … `6XLARGE` |
| `spec.minClusterCount` / `maxClusterCount` | `*int32` | Multi-cluster warehouse scaling (1–10) |
| `spec.scalingPolicy` | `*enum` | `STANDARD` / `ECONOMY` |
| `spec.autoSuspend` | `*int32` | Auto-suspend timeout (seconds) |
| `spec.autoResume` | `*bool` | Auto-resume on query |
| `spec.initiallySuspended` | `bool` | Create in suspended state |
| `spec.resourceMonitor` | `*string` | Resource monitor name |
| `spec.enableQueryAcceleration` | `*bool` | Query acceleration |
| `spec.queryAccelerationMaxScaleFactor` | `*int32` | Max scale factor (0–100) |
| `spec.maxConcurrencyLevel` | `*int32` | Max concurrent queries (1–32) |
| `spec.resourceConstraint` | `*enum` | `MEMORY_1X` etc. |

</details>

<details>
<summary>🎭 <strong>AccountRole</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Snowflake role name *(immutable)* |
| `spec.comment` | `*string` | Optional description |
| `spec.useRole` | `*string` | Snowflake role for USE ROLE |

</details>

<details>
<summary>🎭 <strong>DatabaseRole</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Database role name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.comment` | `*string` | Optional description |

</details>

<details>
<summary>👤 <strong>User</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Snowflake user name *(immutable)* |
| `spec.type` | `*enum` | `PERSON` / `SERVICE` / `LEGACY_SERVICE` *(immutable)* |
| `spec.loginName` | `*string` | Login name (defaults to user name) |
| `spec.displayName` | `*string` | Display name |
| `spec.email` | `*string` | Email address |
| `spec.password` | `*SecretKeyReference` | Secret reference for password |
| `spec.rsaPublicKey` | `*SecretKeyReference` | Secret reference for RSA public key |
| `spec.rsaPublicKey2` | `*SecretKeyReference` | Second RSA key (for rotation) |
| `spec.defaultRole` | `*string` | Default role on login |
| `spec.defaultWarehouse` | `*string` | Default warehouse |
| `spec.disabled` | `*bool` | Whether the user is disabled |

> 🔐 **Secret-Referenced Credentials:** `password`, `rsaPublicKey`, and `rsaPublicKey2` use `SecretKeyReference` to point to Kubernetes Secrets. Sensitive data is never stored in the CR.

</details>

<details>
<summary>🔑 <strong>AccountRoleGrant</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.privilege` | `string` | Snowflake privilege (e.g. USAGE, SELECT) *(immutable)* |
| `spec.on` | `GrantOn` | Grant target — exactly one of `account`, `accountObject`, `schema`, `schemaObject` *(immutable)* |
| `spec.accountRole` | `string` | Account role name *(immutable, mutually exclusive with accountRoleRef)* |
| `spec.accountRoleRef` | `LocalObjectReference` | AccountRole CR reference *(immutable, mutually exclusive with accountRole)* |
| `spec.withGrantOption` | `bool` | Allow grantee to re-grant *(immutable)* |

> ⚠️ **Grant Immutability:** All spec fields are immutable after creation. Changing any field requires deleting and recreating the CR (or using the `force-new` annotation).

</details>

<details>
<summary>🔑 <strong>DatabaseRoleGrant</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.privilege` | `string` | Snowflake privilege (e.g. USAGE, SELECT) *(immutable)* |
| `spec.on` | `GrantOn` | Grant target — exactly one of `account`, `accountObject`, `schema`, `schemaObject` *(immutable)* |
| `spec.databaseRole` | `string` | Fully qualified database role name *(immutable, mutually exclusive with databaseRoleRef)* |
| `spec.databaseRoleRef` | `LocalObjectReference` | DatabaseRole CR reference *(immutable, mutually exclusive with databaseRole)* |
| `spec.withGrantOption` | `bool` | Allow grantee to re-grant *(immutable)* |

> ⚠️ **Grant Immutability:** All spec fields are immutable after creation. Changing any field requires deleting and recreating the CR (or using the `force-new` annotation).

</details>

<details>
<summary>🔑 <strong>ShareGrant</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.privilege` | `string` | Snowflake privilege (e.g. USAGE, SELECT) *(immutable)* |
| `spec.objectType` | `string` | Object type (e.g. DATABASE, SCHEMA, TABLE, VIEW) *(immutable)* |
| `spec.objectName` | `string` | Fully qualified object name *(immutable)* |
| `spec.share` | `string` | Share name *(immutable)* |

> ⚠️ **Grant Immutability:** All spec fields are immutable after creation. Shares do not support WITH GRANT OPTION.

</details>

<details>
<summary>📊 <strong>Table</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Table name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.columns` | `[]ColumnDefinition` | Column definitions (name, type, nullable, default, comment) |
| `spec.transient` | `bool` | Transient table *(immutable)* |
| `spec.dataRetentionTimeInDays` | `*int32` | Time Travel retention (0–90) |
| `spec.clusterBy` | `[]string` | Clustering key expressions |
| `spec.changeTracking` | `*bool` | Enable change tracking |
| `spec.enableSchemaEvolution` | `*bool` | Enable schema evolution |

</details>

<details>
<summary>👁️ <strong>View</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | View name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.statement` | `string` | SQL SELECT statement (triggers CREATE OR REPLACE on change) |
| `spec.secure` | `bool` | Enable SECURE VIEW |
| `spec.changeTracking` | `*bool` | Enable change tracking |

> ⚠️ **Security:** The `statement` field is executed verbatim as SQL. Ensure RBAC restricts View CR access to trusted principals.

</details>

<details>
<summary>📦 <strong>Stage</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Stage name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.url` | `*string` | External stage URL |
| `spec.storageIntegration` | `*string` | Storage integration name (requires `url`) |
| `spec.encryption` | `*StageEncryption` | Encryption settings |
| `spec.directory` | `*StageDirectoryOptions` | Directory table settings |
| `spec.fileFormat` | `*string` | File format |

</details>

<details>
<summary>📤 <strong>FieldExport</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.from.resource.kind` | `string` | Source resource kind (e.g. `Database`) |
| `spec.from.resource.name` | `string` | Source resource name |
| `spec.from.path` | `string` | Dot-notation path (e.g. `.status.showOutput.name`) |
| `spec.to.kind` | `enum` | `ConfigMap` / `Secret` |
| `spec.to.name` | `string` | Target name |
| `spec.to.key` | `string` | Key within the target data |

> 📤 **Cross-Resource Data Passing:** FieldExport reads status fields and writes them to ConfigMaps/Secrets. The exported value is tracked by SHA-256 hash to avoid unnecessary writes. On deletion, exported keys are cleaned up.

</details>

<details>
<summary>⏰ <strong>Task</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Task name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.sqlStatement` | `string` | SQL code executed when the task runs |
| `spec.schedule` | `*string` | Cron or interval schedule (e.g. `5 MINUTES`) |
| `spec.warehouse` | `*string` | Warehouse for task execution *(mutually exclusive with serverless size)* |
| `spec.userTaskManagedInitialWarehouseSize` | `*enum` | Serverless warehouse size: `XSMALL`…`XXLARGE` |
| `spec.after` | `[]string` | Predecessor task names for DAG scheduling |
| `spec.when` | `*string` | Boolean SQL condition for conditional execution |
| `spec.suspend` | `*bool` | Whether the task is suspended (default: `true`) |
| `spec.comment` | `*string` | Optional description |
| `spec.allowOverlappingExecution` | `*bool` | Allow concurrent graph executions |
| `spec.userTaskTimeoutMs` | `*int32` | Single-run timeout in milliseconds (0–604800000) |
| `spec.suspendTaskAfterNumFailures` | `*int32` | Auto-suspend after N consecutive failures |
| `spec.errorIntegration` | `*string` | Notification integration for errors |
| `spec.taskAutoRetryAttempts` | `*int32` | Automatic retry attempts (0–30) |

> ⏰ **DAG Scheduling:** Use `after` to chain tasks into directed acyclic graphs. Root tasks require a `schedule`; child tasks inherit it from the root.

</details>

<details>
<summary>🔄 <strong>Stream</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Stream name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.sourceType` | `enum` | Source type: `TABLE` / `VIEW` / `EXTERNAL_TABLE` / `STAGE` / `DYNAMIC_TABLE` *(immutable)* |
| `spec.sourceName` | `string` | Fully qualified source object name *(immutable)* |
| `spec.appendOnly` | `*bool` | Track row inserts only (TABLE/VIEW) |
| `spec.insertOnly` | `*bool` | Track inserts only (EXTERNAL_TABLE) |
| `spec.showInitialRows` | `*bool` | Include existing rows on first consume |
| `spec.comment` | `*string` | Optional description |

> 🔄 **Change Data Capture:** Streams track DML changes on source objects. Use with Tasks to build real-time data pipelines.

</details>

<details>
<summary>🏷️ <strong>Tag</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Tag name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.allowedValues` | `[]string` | Valid values when assigning the tag (max 5000) |
| `spec.comment` | `*string` | Optional description |

> 🏷️ **Data Governance:** Tags enable metadata classification for compliance, cost allocation, and access control. Assign tags to databases, schemas, tables, columns, and other objects.

</details>

<details>
<summary>🛡️ <strong>NetworkPolicy</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Network policy name *(immutable)* |
| `spec.allowedIPList` | `[]string` | IPv4 addresses or CIDR ranges allowed access |
| `spec.blockedIPList` | `[]string` | IPv4 addresses or CIDR ranges denied access |
| `spec.allowedNetworkRuleList` | `[]string` | Network rules that allow access |
| `spec.blockedNetworkRuleList` | `[]string` | Network rules that deny access |
| `spec.comment` | `*string` | Optional description |

> 🛡️ **Security Perimeter:** Network policies control which IP addresses and network rules can access your Snowflake account.

</details>

<details>
<summary>📈 <strong>ResourceMonitor</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Resource monitor name *(immutable)* |
| `spec.creditQuota` | `*int32` | Credits allocated per frequency interval |
| `spec.frequency` | `*enum` | Reset interval: `MONTHLY` / `DAILY` / `WEEKLY` / `YEARLY` / `NEVER` |
| `spec.startTimestamp` | `*string` | When monitoring begins (use `IMMEDIATELY` for now) |
| `spec.endTimestamp` | `*string` | When the monitor suspends assigned warehouses |
| `spec.notifyUsers` | `[]string` | Users to receive email notifications |
| `spec.triggers` | `[]Trigger` | Threshold triggers with actions (`SUSPEND` / `SUSPEND_IMMEDIATE` / `NOTIFY`) |

> 📈 **Cost Management:** Resource monitors prevent runaway credit usage by suspending warehouses or sending notifications when credit thresholds are reached.

</details>

<details>
<summary>🎭 <strong>MaskingPolicy</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Masking policy name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.signature` | `[]Argument` | Column arguments (name + type) *(immutable)* |
| `spec.body` | `string` | SQL expression that transforms the data |
| `spec.exemptOtherPolicies` | `*bool` | Whether other policies can reference a masked column |
| `spec.comment` | `*string` | Optional description |

> 🎭 **PII/PCI Compliance:** Masking policies dynamically mask sensitive data at query time based on the executing role.

</details>

<details>
<summary>🔐 <strong>RowAccessPolicy</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Row access policy name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.signature` | `[]Argument` | Row arguments (name + type) *(immutable)* |
| `spec.body` | `string` | SQL expression returning BOOLEAN for row visibility |
| `spec.comment` | `*string` | Optional description |

> 🔐 **Row-Level Security:** Row access policies filter rows at query time based on the executing role, enabling multi-tenant data isolation.

</details>

<details>
<summary>🔑 <strong>GrantOwnership</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.objectType` | `string` | Object type (e.g. DATABASE, TABLE, SCHEMA) *(immutable)* |
| `spec.objectName` | `string` | Fully qualified object name *(immutable)* |
| `spec.accountRole` | `string` | Target account role *(immutable, mutually exclusive with refs)* |
| `spec.accountRoleRef` | `LocalObjectReference` | AccountRole CR reference *(immutable)* |
| `spec.databaseRole` | `string` | Target database role *(immutable, mutually exclusive with refs)* |
| `spec.databaseRoleRef` | `LocalObjectReference` | DatabaseRole CR reference *(immutable)* |
| `spec.currentGrantsBehavior` | `*enum` | `COPY` / `REVOKE` — how existing privileges are handled |

> ⚠️ **Ownership Immutability:** All spec fields are immutable after creation. Ownership cannot be revoked — deleting the CR leaves ownership intact (no-op on delete).

</details>

## 🔍 Drift Detection

All controllers include field-level drift detection on each reconciliation cycle:

1. 🔎 Observe current Snowflake state
2. 📊 Compute field-level diffs between spec and observed
3. 🔔 If drift detected: set `DriftDetected` condition → emit event → correct (or report only)

**Detect-Only Policy** — annotate to report drift without correcting:

```yaml
metadata:
  annotations:
    snowplane.hupe1980.github.io/drift-policy: detect-only
```

## 🏷️ Resource Adoption

Adopt pre-existing Snowflake resources:

```yaml
metadata:
  annotations:
    snowplane.hupe1980.github.io/adoption-policy: adopt
```

Without this annotation, the reconciler returns a `Terminal` error if the resource already exists. With `adopt`, status is populated from current state and `LateInitialized` condition is set.

## 🤝 Coexistence with Non-Managed Resources

Snowplane is designed for safe coexistence with manually-managed or Terraform-managed resources:

- 🔍 **Read-only observation** — `SHOW` queries never mutate state
- 🔒 **Finalizer-gated writes** — CREATE/ALTER/DROP only for finalized resources
- 🚫 **No implicit takeover** — Existing resources produce `Terminal` errors unless adoption is enabled
- 🏷️ **Orphan deletion policy** — Remove CRD without dropping the Snowflake resource

## 📋 Status Conditions

| Condition | Description |
|-----------|-------------|
| ✅ `Ready` | Resource exists and matches desired state |
| 🔄 `Synced` | Successful reconciliation completed |
| 🔗 `ReferencesResolved` | Cross-resource references resolved and Ready |
| 🔍 `DriftDetected` | Out-of-band changes detected |
| ❌ `Terminal` | Non-retryable error (e.g. invalid identifier) |
| ♻️ `Recoverable` | Transient error — retried automatically |
| 🔑 `CredentialsInvalid` | Credentials invalid or missing (ProviderConfig) |
| 📥 `LateInitialized` | Status populated from existing resource during adoption |

## 🔄 Terraform State Import

The `cmd/tfimport` tool converts Terraform state files to Snowplane CRD manifests:

```bash
go run ./cmd/tfimport -state terraform.tfstate -provider default -namespace snowflake > manifests.yaml
kubectl apply -f manifests.yaml
```

**Supported resources:**

| Terraform Resource | Snowplane CRD |
|--------------------|---------------|
| `snowflake_database` | Database |
| `snowflake_schema` | Schema |
| `snowflake_warehouse` | Warehouse |
| `snowflake_user` | User |
| `snowflake_account_role` / `snowflake_role` | AccountRole |
| `snowflake_database_role` | DatabaseRole |
| `snowflake_grant_privileges_to_account_role` | AccountRoleGrant |
| `snowflake_grant_privileges_to_database_role` | DatabaseRoleGrant |
| `snowflake_grant_privileges_to_share` | ShareGrant |
| `snowflake_table` | Table |
| `snowflake_view` | View |
| `snowflake_stage` | Stage |

Generated manifests use `deletionPolicy: Orphan`. Sensitive fields are skipped and must be configured manually.

## 📂 Project Structure

```
.
├── api/v1alpha1/           # CRD type definitions & validation
├── cmd/
│   ├── manager/            # Controller manager entrypoint
│   └── tfimport/           # Terraform state → Snowplane converter
├── config/
│   ├── crd/bases/          # CRD YAML manifests
│   ├── grafana/            # Grafana dashboard JSON
│   ├── manager/            # Deployment, PDB, NetworkPolicy
│   ├── rbac/               # RBAC roles and bindings
│   ├── samples/            # Example CR YAML files
│   └── webhook/            # Webhook configurations
├── charts/snowplane/       # Helm chart
├── docs/                   # Documentation
├── hack/                   # Dev & codegen scripts
├── internal/
│   ├── clients/
│   │   ├── clientfactory/  # Client cache with hash-based rotation
│   │   └── snowflake/      # Snowflake SDK wrapper & SQL builder
│   ├── controller/         # All reconcilers (20 managed resources + FieldExport + ProviderConfig)
│   ├── drift/              # Field-level drift detection engine
│   ├── metrics/            # Custom Prometheus metrics
│   ├── provider/           # Provider config builder & client resolution
│   ├── ratelimit/          # Per-provider token-bucket rate limiter
│   ├── sfretry/            # Retry wrapper for transient Snowflake errors
│   ├── webhook/            # Admission webhook handlers
│   └── utils/              # Conditions, finalizers, sanitizers
└── test/
    ├── e2e/                # End-to-end tests (kind + real Snowflake)
    └── integration/        # envtest integration tests
```

## 📚 Documentation

| Guide | Description |
|-------|-------------|
| 📘 [Getting Started](docs/getting-started.md) | Installation, ProviderConfig setup, first resources |
| ☸️ [Helm Chart](docs/helm-chart.md) | Helm chart configuration and RBAC |
| 📊 [Observability](docs/observability.md) | Metrics, logging, events, probes, Grafana, circuit breaker |
| 🔍 [Drift Detection](docs/drift-detection.md) | Drift engine, detect-only policy, field-level reporting |
| 📋 [CRD Lifecycle](docs/crd-lifecycle.md) | Maturity classification and graduation |
| 🛠️ [Development](docs/development.md) | Architecture, project layout, contributing |
| 🌐 [Workload Identity](docs/workload-identity.md) | External OAuth and Workload Identity Federation |
| 🔐 [Snowflake Role Setup](docs/snowflake-role-setup.md) | Required Snowflake roles and permissions |
| 🚀 [GitOps with Argo CD](docs/gitops-argocd.md) | Health checks, sync waves, ArgoCD setup |
| 🚀 [GitOps with Flux](docs/gitops-flux.md) | Kustomization dependencies, health checks, Flux setup |
| 🧩 [Adding a Resource](docs/adding-a-resource.md) | Guide for adding new Snowflake resource types |

## 🤝 Contributing

1. 🍴 Fork the repository
2. 🌿 Create a feature branch (`git checkout -b feature/amazing-feature`)
3. ✅ Run tests (`just test`)
4. 🔍 Run linter (`just lint`)
5. 💾 Commit your changes (`git commit -m 'Add amazing feature'`)
6. 📤 Push to the branch (`git push origin feature/amazing-feature`)
7. 🔀 Open a Pull Request

## 📄 License

This project is licensed under the Apache License 2.0 — see the [LICENSE](LICENSE) file for details.
