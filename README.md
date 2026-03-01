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

### 🏗️ Resource Management

| Category | Resources |
|----------|-----------|
| 🗄️ **Core Infrastructure** | Database, Schema, Warehouse |
| 📊 **Data Objects** | Table, View, Stage, StreamOnTable, StreamOnView, StreamOnExternalTable, StreamOnDirectoryTable, StreamOnDynamicTable, DynamicTable, FileFormat, Pipe |
| 🎭 **Identity & Access** | User, AccountRole, DatabaseRole, AccountRoleGrant, DatabaseRoleGrant, AccountRoleAssignment, DatabaseRoleAssignment, ShareGrant, GrantOwnership |
| ⏰ **Orchestration** | Task (DAG scheduling, serverless or warehouse-backed), Alert (condition-based monitoring & notification) |
| 🔗 **Integrations** | StorageIntegration, SecurityIntegration, NotificationIntegration |
| 🛡️ **Security & Governance** | NetworkPolicy, NetworkRule, PasswordPolicy, MaskingPolicy, RowAccessPolicy, Tag, ResourceMonitor |
| 📤 **Utilities** | FieldExport (copy status fields into ConfigMaps/Secrets) |

Every resource supports full lifecycle management (create, alter, drop), drift detection, adoption of pre-existing objects, and deletion policies. See the [API Reference](#-api-reference) below for detailed field documentation.

### 🔧 Operator Capabilities

- 🔗 **Cross-Resource References** — Schemas → Databases; Tables, Views, Stages, FileFormats, Pipes, DynamicTables, MaskingPolicies, PasswordPolicies, NetworkRules → Databases + Schemas with automatic dependency resolution and backoff
- 🔄 **Observe-Diff-Apply Reconciliation** — Only altered fields are pushed to Snowflake, minimizing API calls
- 🔍 **Drift Detection & Correction** — Field-level drift detection with structured reporting, detect-only policy option
- 🏷️ **Resource Adoption** — Adopt pre-existing Snowflake resources via `spec.managementPolicies.adoptionPolicy: adopt`
- 🔒 **Immutable Field Enforcement** — Two layers: CRD schema (CEL `self == oldSelf`), reconciler-level errors
- 🛡️ **Dangerous Grant Protection** — Blocks grants to ACCOUNTADMIN/SECURITYADMIN/ORGADMIN and dangerous privileges by default
- 🛡️ **Policy Body Validation** — Blocklist-based SQL injection prevention for MaskingPolicy and RowAccessPolicy `body` fields
- 🏷️ **Ownership Conflict Detection** — Prevents duplicate CRs from managing the same Snowflake object via label-based conflict detection
- ⚛️ **CREATE OR ALTER** — Atomic `CREATE OR ALTER` enabled by default for Database, Schema, Table, Warehouse, Task, Tag, View, FileFormat, MaskingPolicy, PasswordPolicy, NetworkRule, RowAccessPolicy & User, with graceful fallback for unsupported Snowflake editions (opt out via `spec.managementPolicies.createOrAlter: false`)
- 🗑️ **Deletion Policies** — `Delete` (drop resource) or `Orphan` (leave intact)
- 🏷️ **ForceNew Annotation** — Delete+recreate on immutable field changes

### 🔐 Security & Authentication

- 🔑 **Key Pair Authentication** — RSA key pair auth via Kubernetes Secrets (supports encrypted PKCS#8 with passphrase)
- 🔑 **Username/Password** — Password auth via Kubernetes Secrets
- 🌐 **Workload Identity Federation** — EKS IRSA, GKE WI, AKS WI via projected ServiceAccount tokens
- 🔒 **Sensitive Field Redaction** — Passwords, PEM keys, tokens `[REDACTED]` in all logs and events
- 🛡️ **CEL Validation Rules** — Immutable field enforcement, enum/range validation, dangerous-grant blocking at the CRD level
- 🔧 **CRD Schema Defaults** — Auto-defaults for `deletionPolicy`, `providerRef`, User `type` via CRD schema

### 📈 Observability & Operations

- 📊 **Custom Prometheus Metrics** — Reconciliation, Snowflake ops, client pool, rate limits, drift, circuit breaker
- 📉 **Grafana Dashboard** — Pre-built dashboard at `config/grafana/snowplane-dashboard.json`
- 🔌 **Connection Pooling** — LRU client cache with config-change rotation, configurable max size, and idle TTL eviction
- ⏱️ **Rate Limiting** — Hierarchical two-level rate limiting: per-controller + per-account aggregate token-bucket limiters (configurable QPS/burst)
- 🔌 **Circuit Breaker** — Per-provider 3-state failure isolation (closed/open/half-open)
- ☸️ **Helm Chart** — Production-ready with CRDs, RBAC, ServiceMonitor, PDB
- 📋 **Condition-based Status** — Ready, Synced, ReferencesResolved, Terminal, DriftDetected, CredentialsInvalid, LateInitialized

## 🏛️ Architecture

```
┌──────────────────────────-────────────────────────┐
│                Kubernetes Cluster                 │
│                                                   │
│  ┌────────-──────┐   ┌────────────────────────┐   │
│  │ ProviderConfig│   │   Snowflake Resource   │   │
│  │      CR       │   │     Custom Resources   │   │
│  └──────┬────────┘   └────────┬───────────────┘   │
│         │                     │                   │
│  ┌──────▼─────────────────────▼──────────────────┐│
│  │          Snowplane Controller Manager         ││
│  │                                               ││
│  │  ┌──────────────────────────────────────────┐ ││
│  │  │  🔗 Reference Resolver                   │ ││
│  │  │  Resolves databaseRef/schemaRef → FQN    │  ││
│  │  │  Waits for dependency readiness          │  ││
│  │  └──────────────────────────────────────────┘  ││
│  │                                                ││
│  │  ┌──────────────────────────────────────────┐  ││
│  │  │  Resource Controllers (see above)        │  ││
│  │  │  Observe → Diff → Apply reconciliation   │  ││
│  │  └──────────────────────────────────────────┘  ││
│  │                                                ││
│  │  ┌──────────────────┐ ┌───────────────────┐   ││
│  │  │ 📤 FieldExport    │ │ 🔍 Drift Engine  │   ││
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
| `--rate-limit-qps` | `10` | Sustained queries/sec to Snowflake per controller per provider (0 = disabled) |
| `--rate-limit-burst` | `20` | Max burst size for per-controller Snowflake API rate limiter |
| `--account-rate-limit-qps` | `50` | Aggregate queries/sec to Snowflake per account across all controllers (0 = disabled) |
| `--account-rate-limit-burst` | `100` | Max burst size for per-account aggregate rate limiter |
| `--requeue-interval` | `5m` | Drift detection re-observe interval |
| `--enable-alpha-resources` | `true` | Enable alpha-maturity controllers |
| `--disable-controllers` | `""` | Comma-separated controllers to disable (e.g. `accountrolegrant,stage,view`) |
| `--circuit-breaker-threshold` | `5` | Consecutive Snowflake failures before circuit opens |
| `--circuit-breaker-reset-timeout` | `60s` | Backoff duration before half-open probe |
| `--allowed-roles` | `""` | Comma-separated allowlist of Snowflake roles permitted in ProviderConfig (case-insensitive; empty = all allowed) |
| `--snowflake-op-timeout` | `60s` | Timeout for individual Snowflake API operations |
| `--watch-namespaces` | `""` | Comma-separated namespaces to watch (empty = all) |
| `--development` | `false` | Human-readable debug logging |

### 🏥 Health Probes

| Endpoint | Check | Description |
|----------|-------|-------------|
| `/healthz` | Ping | Returns 200 if the manager process is running |
| `/readyz` | Snowflake connectivity | Pings all cached clients; 200 when all reachable |

### 🏷️ Annotations & Spec Fields Reference

**Spec Lifecycle Policies** (`spec.managementPolicies`):

| Field | Default | Values | Description |
|-------|---------|--------|-------------|
| `adoptionPolicy` | `fail-if-exists` | `adopt` / `fail-if-exists` | Control adoption of pre-existing resources |
| `driftPolicy` | `correct` | `correct` / `detect-only` | Report drift without correcting it, or correct automatically |
| `createOrAlter` | `true` | `true` / `false` | Atomic `CREATE OR ALTER` (Database, Schema, Table, Warehouse, Task, Tag, View, FileFormat, MaskingPolicy, PasswordPolicy, NetworkRule, RowAccessPolicy, User) |

**Annotations**:

| Annotation | Default | Values | Description |
|------------|---------|--------|-------------|
| `snowplane.hupe1980.github.io/force-new` | `false` | `true` / `false` | Delete + recreate on immutable field change |
| `snowplane.hupe1980.github.io/allow-dangerous-grant` | `false` | `true` / `false` | Allow grants to system roles / dangerous privileges |
| `snowplane.hupe1980.github.io/abandon-on-delete` | `false` | `true` / `false` | Remove finalizer without dropping the Snowflake resource |

### 🏷️ CRD Labels

| Label | Values | Description |
|-------|--------|-------------|
| `snowplane.hupe1980.github.io/maturity` | `alpha` / `beta` / `stable` | CRD maturity classification |
| `snowplane.hupe1980.github.io/external-name-hash` | SHA-256 prefix | Ownership label — prevents duplicate CRs from managing the same Snowflake object |

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
| `controller.disableControllers` | `""` | Comma-separated controllers to disable |
| `controller.allowedRoles` | `""` | Comma-separated allowlist of permitted Snowflake roles |
| `controller.snowflakeOpTimeout` | `60s` | Timeout for individual Snowflake API operations |
| `rateLimit.qps` | `10` | Per-controller Snowflake API rate limit per provider |
| `rateLimit.burst` | `20` | Per-controller rate limit burst size |
| `rateLimit.accountQps` | `50` | Per-account aggregate rate limit (all controllers) |
| `rateLimit.accountBurst` | `100` | Per-account aggregate burst size |
| `circuitBreaker.threshold` | `5` | Consecutive failures before circuit opens |
| `circuitBreaker.resetTimeout` | `60s` | Backoff before half-open probe |
| `leaderElection.enabled` | `true` | Enable leader election |
| `metrics.serviceMonitor.enabled` | `false` | Create Prometheus ServiceMonitor |
| `grafana.dashboard.enabled` | `false` | Deploy Grafana dashboard ConfigMap |
| `revisionHistoryLimit` | `3` | ReplicaSet history limit |
| `watchNamespaces` | `""` | Namespaces to watch (empty = all) |
| `priorityClassName` | `""` | Pod PriorityClass name |
| `topologySpreadConstraints` | `[]` | Topology spread constraints |
| `extraEnv` | `[]` | Additional environment variables |
| `extraVolumes` | `[]` | Additional volumes |
| `extraVolumeMounts` | `[]` | Additional volume mounts |

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
| `snowplane_orphaned_resources_total` | Counter | `controller` | Orphan-policy deletions |
| `snowplane_ownership_conflicts_total` | Counter | `controller` | Ownership conflicts during adoption |
| `snowplane_adoption_total` | Counter | `controller`, `result` | Adoption outcomes |
| `snowplane_drift_detected_total` | Counter | `controller` | Drift detection events |
| `snowplane_managed_resources` | Gauge | `controller`, `state` | Resources by state (`ready`/`not_ready`/`terminal`) |
| `snowplane_account_rate_limit_waits_total` | Counter | `provider` | Per-account aggregate rate limiter wait events |
| `snowplane_circuit_breaker_trips_total` | Counter | `provider` | Circuit breaker trips |
| `snowplane_circuit_breaker_state` | Gauge | `provider` | Breaker state (0=closed, 1=open, 2=half-open) |
| `snowplane_db_max_open_conns` | Gauge | `provider` | Max open DB connections per provider |
| `snowplane_db_open_conns` | Gauge | `provider` | Current open DB connections per provider |
| `snowplane_db_in_use_conns` | Gauge | `provider` | In-use DB connections per provider |
| `snowplane_db_idle_conns` | Gauge | `provider` | Idle DB connections per provider |
| `snowplane_db_wait_count` | Gauge | `provider` | Total DB connection wait count per provider |
| `snowplane_db_wait_duration_seconds` | Gauge | `provider` | Total DB connection wait duration per provider |

### 📉 Grafana Dashboard

Import `config/grafana/snowplane-dashboard.json` via Grafana UI → Dashboards → Import. Panels include:

- 📈 Reconcile rate & error rate per controller
- ⏱️ Reconcile duration percentiles (P50 / P95 / P99)
- ❄️ Snowflake API latency & operation counts
- 🔌 Client pool size & rate limiter waits
- 🔍 Drift detection frequency per controller
- 🔌 Circuit breaker state & trip frequency

## 📖 API Reference

Complete field-level documentation for all Snowplane CRDs is available in the **[API Reference](docs/api-reference.md)**.

| Category | Resources |
|----------|-----------|
| 🔌 **Provider** | ProviderConfig |
| 🗄️ **Core Infrastructure** | Database, Schema, Warehouse |
| 📊 **Data Objects** | Table, View, Stage, StreamOnTable, StreamOnView, StreamOnExternalTable, StreamOnDirectoryTable, StreamOnDynamicTable, DynamicTable, FileFormat, Pipe |
| 🎭 **Identity & Access** | User, AccountRole, DatabaseRole, AccountRoleGrant, DatabaseRoleGrant, AccountRoleAssignment, DatabaseRoleAssignment, ShareGrant, GrantOwnership |
| ⏰ **Orchestration** | Task, Alert |
| 🔗 **Integrations** | StorageIntegration, SecurityIntegration, NotificationIntegration |
| 🛡️ **Security & Governance** | NetworkPolicy, NetworkRule, PasswordPolicy, MaskingPolicy, RowAccessPolicy, Tag, ResourceMonitor |
| 📤 **Utilities** | FieldExport |

## 🔍 Drift Detection

All controllers include field-level drift detection on each reconciliation cycle:

1. 🔎 Observe current Snowflake state
2. 📊 Compute field-level diffs between spec and observed
3. 🔔 If drift detected: set `DriftDetected` condition → emit event → correct (or report only)

**Detect-Only Policy** — report drift without correcting:

```yaml
spec:
  managementPolicies:
    driftPolicy: detect-only
```

## 🏷️ Resource Adoption

Adopt pre-existing Snowflake resources:

```yaml
spec:
  managementPolicies:
    adoptionPolicy: adopt
```

Without this policy, the reconciler returns a `Terminal` error if the resource already exists. With `adopt`, status is populated from current state and `LateInitialized` condition is set.

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
│   └── samples/            # Example CR YAML files
├── charts/snowplane/       # Helm chart
├── docs/                   # Documentation
├── hack/                   # Dev & codegen scripts
├── internal/
│   ├── circuitbreaker/     # Per-provider 3-state failure isolation
│   ├── clients/
│   │   ├── clientfactory/  # Client cache with hash-based rotation
│   │   └── snowflake/      # Snowflake SDK wrapper & SQL builder
│   ├── controller/         # All reconcilers (managed resources + FieldExport + ProviderConfig)
│   ├── drift/              # Field-level drift detection engine
│   ├── metrics/            # Custom Prometheus metrics
│   ├── provider/           # Provider config builder & client resolution
│   ├── ratelimit/          # Hierarchical per-controller + per-account rate limiter
│   ├── sfretry/            # Retry wrapper for transient Snowflake errors
│   ├── testutil/           # Shared test utilities and reconciler suite
│   ├── tracked/            # Generic tracked parameter computation via struct tags
│   └── utils/              # Conditions, finalizers, sanitizers
└── test/
    ├── e2e/                # End-to-end tests (kind + real Snowflake)
    └── integration/        # envtest integration tests
```

## 📚 Documentation

| Guide | Description |
|-------|-------------|
| 📘 [Getting Started](docs/getting-started.md) | Installation, ProviderConfig setup, first resources |
| 📖 [API Reference](docs/api-reference.md) | Complete field-level documentation for all CRDs |
| ☸️ [Helm Chart](docs/helm-chart.md) | Helm chart configuration and RBAC |
| 🏭 [Production Guide](docs/production-guide.md) | Production hardening, security, scaling, DR |
| 📊 [Observability](docs/observability.md) | Metrics, logging, events, probes, Grafana, circuit breaker |
| 🔍 [Drift Detection](docs/drift-detection.md) | Drift engine, detect-only policy, field-level reporting |
| 📋 [CRD Lifecycle](docs/crd-lifecycle.md) | Maturity classification and graduation |
| 🏗️ [Architecture](docs/architecture.md) | Reconciler state machine, adapter pattern, resilience layers |
| 🛠️ [Development](docs/development.md) | Project layout, patterns, contributing |
| 🔧 [Troubleshooting](docs/troubleshooting.md) | Condition reference, common scenarios, debug collection |
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
