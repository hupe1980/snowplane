# Observability

Snowplane provides comprehensive observability through Prometheus metrics, structured logging, Kubernetes events, health probes, and a pre-built Grafana dashboard.

## Prometheus Metrics

All metrics use the `snowplane_` namespace and are exposed on the controller's metrics endpoint (default `:8080/metrics`).

### Reconciliation

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `snowplane_reconcile_total` | Counter | `controller`, `result` | Total reconciliation attempts (`success` / `error`) |
| `snowplane_reconcile_duration_seconds` | Histogram | `controller` | Duration of each reconciliation loop |

### Snowflake API

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `snowplane_snowflake_operation_total` | Counter | `controller`, `operation`, `result` | Snowflake API calls by operation (`observe`, `create`, `alter`, `drop`, `ping`) |
| `snowplane_snowflake_operation_duration_seconds` | Histogram | `controller`, `operation` | Snowflake API call latencies |
| `snowplane_client_pool_size` | Gauge | — | Active Snowflake clients in the connection pool (O(1) LRU eviction via doubly-linked list when configurable max size is reached) |
| `snowplane_rate_limit_waits_total` | Counter | `controller` | Number of times a reconciler waited for the rate limiter |

> **Cardinality Bounds:** The `operation` label is bounded to a fixed set of values: `observe`, `create`, `alter`, `drop`, `create_or_alter`, and `ping`. The `controller` label is bounded by the number of registered controllers (currently 26). The `result` label is bounded to `success` and `error`. Total cardinality for operation metrics is at most ~312 series (26 controllers × 6 operations × 2 results), well within Prometheus best practices.

### Resource Management

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `snowplane_managed_resources` | Gauge | `controller`, `state` | Managed resources by controller and state (`ready`, `not_ready`, `terminal`) |
| `snowplane_adoption_total` | Counter | `controller`, `result` | Resource adoption outcomes (`adopted` / `rejected`) |
| `snowplane_drift_detected_total` | Counter | `controller` | Drift detection events |
| `snowplane_orphaned_resources_total` | Counter | `controller` | Orphan-policy deletions (Snowflake resource intentionally left intact) |
| `snowplane_ownership_conflicts_total` | Counter | `controller` | Ownership conflicts detected — another CR already manages the same Snowflake object |

### Circuit Breaker

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `snowplane_circuit_breaker_trips_total` | Counter | `provider` | Number of times the circuit breaker opened for a provider |
| `snowplane_circuit_breaker_state` | Gauge | `provider` | Current state per provider: 0 = closed, 1 = open, 2 = half-open |

### ProviderConfig Health

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `snowplane_providerconfig_healthy` | Gauge | `provider`, `account` | Whether a ProviderConfig is healthy (1 = connected, 0 = unhealthy). Cleaned up when the ProviderConfig is deleted. |

### Connection Pool (database/sql)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `snowplane_db_max_open_conns` | Gauge | `provider` | Maximum number of open connections allowed |
| `snowplane_db_open_conns` | Gauge | `provider` | Current number of open connections |
| `snowplane_db_in_use_conns` | Gauge | `provider` | Connections currently in use |
| `snowplane_db_idle_conns` | Gauge | `provider` | Connections currently idle |
| `snowplane_db_wait_count` | Gauge | `provider` | Total connections waited for |
| `snowplane_db_wait_duration_seconds` | Gauge | `provider` | Total time blocked waiting for connections |

These metrics are collected from `database/sql.DBStats` for each cached Snowflake client. They are recorded periodically via `ClientFactory.CollectDBStats()` and expose connection pool pressure per provider.

### Prometheus ServiceMonitor

When using the Helm chart, enable automatic Prometheus scraping:

```yaml
metrics:
  serviceMonitor:
    enabled: true
    additionalLabels:
      release: prometheus  # match your Prometheus operator's labels
    interval: 30s
```

### Example Alert Rules

```yaml
groups:
  - name: snowplane
    rules:
      - alert: SnowplaneHighErrorRate
        expr: rate(snowplane_reconcile_total{result="error"}[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High reconcile error rate for {{ $labels.controller }}"

      - alert: SnowplaneCircuitBreakerOpen
        expr: snowplane_circuit_breaker_state > 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Circuit breaker open for provider {{ $labels.provider }}"

      - alert: SnowflakeAPILatencyHigh
        expr: histogram_quantile(0.99, rate(snowplane_snowflake_operation_duration_seconds_bucket[5m])) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High Snowflake API latency for {{ $labels.operation }}"
```

## Structured Logging

Snowplane uses `logr` backed by `zap` for structured JSON logging in production.

### Log Levels

| Level | Content |
|-------|---------|
| `V(0)` (info) | Lifecycle transitions: creating, updating, deleting, drift detected, adoption |
| `V(1)` (debug) | Snowflake API calls, operation details, retry attempts, field-level diffs |

Configure with `--zap-log-level`:

```bash
--zap-log-level=info   # V(0) only (default)
--zap-log-level=debug  # V(0) + V(1)
```

### Standard Fields

Every log line includes:

- `controller` — Resource type (e.g., `database`, `schema`)
- `namespace` — CR namespace
- `name` — CR name
- `reconcileID` — Unique ID for the reconciliation loop (from controller-runtime)

### Sensitive Field Redaction

All log output is sanitized to prevent secrets from appearing in logs:

- **PEM keys** (`-----BEGIN ... -----END ...`) → `[REDACTED]`
- **Password/token fields** (`password=...`, `rsaPublicKey=...`, `token=...`) → field name `=[REDACTED]`
- **Connection strings** (`user@account.snowflakecomputing.com`) → `[connection redacted]`
- **Snowflake hostnames** → `[host redacted]`

The `ForLog()` function in `internal/utils/sanitize/` sanitizes log messages while preserving SQL statements (useful for debugging). The `ForEvent()` function additionally strips SQL and truncates to 1024 characters for Kubernetes Events.

## Kubernetes Events

Snowplane emits Kubernetes events on CRs for all significant lifecycle transitions. View them with `kubectl describe <resource>` or `kubectl get events`.

### Normal Events

| Reason | Description |
|--------|-------------|
| `Created` | Resource created in Snowflake |
| `Updated` | Resource updated (spec change applied) |
| `Deleted` / `Orphaned` | Resource dropped or orphaned in Snowflake |
| `AdoptedExisting` | Pre-existing Snowflake resource adopted |
| `DriftCorrected` | Out-of-band change automatically corrected |

### Warning Events

| Reason | Description |
|--------|-------------|
| `DriftDetected` | Out-of-band change detected (logged before correction or detect-only) |
| `ReconcileError` | Transient error during reconciliation (will be retried) |
| `TerminalError` | Non-retryable error (requires user intervention) |
| `DependencyNotReady` | A ProviderConfig or parent resource is not ready |
| `ValidationFailed` | Spec validation failure (defense-in-depth) |
| `ImmutableField` | Attempt to modify an immutable field |
| `OrphanedResource` | Resource deleted with `Orphan` deletion policy — Snowflake object left intact |
| `ConflictDetected` | Another CR already manages the same Snowflake object (ownership conflict) |
| `CreateOrAlterFallback` | `CREATE OR ALTER` not supported by the Snowflake edition; falling back to `ALTER` |

### SafeRecorder

All event messages pass through `SafeRecorder`, which wraps the standard `record.EventRecorder` and sanitizes messages via `ForEvent()` before emitting. This ensures no SQL, passwords, or connection strings appear in Kubernetes events.

## Health Probes

| Endpoint | Check | Description |
|----------|-------|-------------|
| `/healthz` | Ping | Returns 200 if the manager process is running |
| `/readyz` | Snowflake connectivity | Pings all cached Snowflake clients via `SELECT 1`; returns 200 when all are reachable |

Configure with `--health-probe-bind-address` (default `:8081`).

The readiness probe uses `ClientFactory.CheckHealth()`, which iterates all cached Snowflake connections and executes a `SELECT 1` with a 5-second timeout. If no clients are cached (e.g., before the first ProviderConfig is reconciled), the probe returns healthy.

A configurable **startup grace period** (`WithStartupGrace`, default 30 seconds) allows the readiness probe to pass during initial startup before any ProviderConfig has been reconciled and a Snowflake client cached. This prevents Kubernetes from killing the pod before it has had a chance to establish its first connection.

## Grafana Dashboard

A pre-built Grafana dashboard is available at `config/grafana/snowplane-dashboard.json`.

### Helm Chart Provisioning

The Helm chart can automatically deploy the dashboard as a ConfigMap for Grafana sidecar provisioning:

```yaml
grafana:
  dashboard:
    enabled: true
    labels:
      grafana_dashboard: "1"  # standard sidecar discovery label
```

### Panels

- **Reconcile Rate** — Success/error rate per controller
- **Reconcile Duration** — P50/P95/P99 percentiles per controller
- **Reconcile Error Rate** — Error-only rate per controller
- **Snowflake API Latency** — P50/P95/P99 per operation
- **Snowflake API Operations** — Operation counts (success/error)
- **Client Pool Size** — Cached Snowflake connections
- **Rate Limit Waits** — Throttling frequency
- **Drift Detection Frequency** — Drift events per controller
- **Adoption Outcomes** — Adopted/rejected per controller
- **Managed Resources** — Resources by controller and state
- **Circuit Breaker State** — Current state per provider
- **Circuit Breaker Trips** — Trip frequency per provider

### Manual Installation

1. Open Grafana → Dashboards → Import
2. Upload `config/grafana/snowplane-dashboard.json`
3. Select your Prometheus datasource
4. Use the `Controller` dropdown to filter by resource type

## Circuit Breaker

The circuit breaker in `internal/circuitbreaker/` provides per-provider failure isolation. If a ProviderConfig's Snowflake account is unreachable, the circuit breaker prevents cascading failures to other providers.

### States

| State | Behavior |
|-------|----------|
| **Closed** | Normal operation — all calls pass through |
| **Open** | After N consecutive failures — calls are rejected immediately with `DependencyNotReady` |
| **Half-Open** | After the reset timeout — a single probe call is allowed; success resets to closed, failure reopens |

### Configuration

The circuit breaker uses sensible defaults:

| Option | Default | Description |
|--------|---------|-------------|
| `FailureThreshold` | 5 | Consecutive failures before opening |
| `ResetTimeout` | 60s | Duration to stay open before probing |

### How It Works

1. Each reconciliation records success/failure via the circuit breaker
2. After `FailureThreshold` consecutive failures, the breaker opens for that provider
3. While open, resources referencing the failing provider get `DependencyNotReady` condition
4. After `ResetTimeout`, the breaker transitions to half-open and allows one probe
5. Successful probe resets to closed; failed probe reopens

Resources using other ProviderConfigs are completely unaffected.
