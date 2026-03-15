---
layout: default
title: Observability
parent: Concepts
nav_order: 2
description: "Prometheus metrics, structured logging, Kubernetes events, and Grafana dashboards."
---

# Observability
{: .fs-8 }

Comprehensive observability through Prometheus metrics, structured logging, Kubernetes events, health probes, and a pre-built Grafana dashboard.
{: .fs-5 .fw-300 }

---

## Prometheus Metrics

All metrics use the `snowplane_` namespace and are exposed on the controller's metrics endpoint (default `:8080/metrics`).

### Reconciliation

| Metric | Type | Labels | Description |
|:-------|:-----|:-------|:------------|
| `snowplane_reconcile_total` | Counter | `controller`, `result` | Total reconciliation attempts |
| `snowplane_reconcile_duration_seconds` | Histogram | `controller` | Duration of each reconciliation |

### Snowflake API

| Metric | Type | Labels | Description |
|:-------|:-----|:-------|:------------|
| `snowplane_snowflake_operation_total` | Counter | `controller`, `operation`, `result` | Snowflake API calls by operation |
| `snowplane_snowflake_operation_duration_seconds` | Histogram | `controller`, `operation` | Snowflake API call latencies |
| `snowplane_client_pool_size` | Gauge | — | Active Snowflake clients in the connection pool |
| `snowplane_rate_limit_waits_total` | Counter | `controller` | Per-controller rate limiter wait events |
| `snowplane_account_rate_limit_waits_total` | Counter | `provider` | Per-account aggregate rate limiter wait events |

{: .note }
> **Cardinality Bounds:** The `operation` label is bounded to `observe`, `create`, `alter`, `drop`, `create_or_alter`, `ping`. The `controller` label is bounded by the number of registered controllers. Total cardinality for operation metrics scales linearly with the number of controllers.

### Resource Management

| Metric | Type | Labels | Description |
|:-------|:-----|:-------|:------------|
| `snowplane_adoption_total` | Counter | `controller`, `result` | Resource adoption outcomes |
| `snowplane_drift_detected_total` | Counter | `controller` | Drift detection events |
| `snowplane_orphaned_resources_total` | Counter | `controller` | Orphan-policy deletions |
| `snowplane_ownership_conflicts_total` | Counter | `controller` | Ownership conflicts detected |

### Operational Visibility

| Metric | Type | Labels | Description |
|:-------|:-----|:-------|:------------|
| `snowplane_late_init_total` | Counter | `controller`, `result` | Late-initialization events (`modified` or `noop`) |
| `snowplane_preflight_failures_total` | Counter | `controller`, `reason` | Pre-flight check failures that delay reconciliation |
| `snowplane_snowflake_error_codes_total` | Counter | `provider`, `code` | Snowflake errors by numeric error code |
| `snowplane_sqlstatement_executions_total` | Counter | `namespace`, `name`, `operation` | SQLStatement execute/revert audit trail |

### Circuit Breaker

| Metric | Type | Labels | Description |
|:-------|:-----|:-------|:------------|
| `snowplane_circuit_breaker_trips_total` | Counter | `provider` | Circuit breaker open events |
| `snowplane_circuit_breaker_state` | Gauge | `provider` | 0 = closed, 1 = open, 2 = half-open |

### ProviderConfig Health

| Metric | Type | Labels | Description |
|:-------|:-----|:-------|:------------|
| `snowplane_providerconfig_healthy` | Gauge | `provider`, `account` | 1 = connected, 0 = unhealthy |

### Sharding

| Metric | Type | Labels | Description |
|:-------|:-----|:-------|:------------|
| `snowplane_shard_info` | Gauge | `shard_id`, `shard_count` | Shard configuration for this manager instance (always 1) |

### Connection Pool

| Metric | Type | Labels | Description |
|:-------|:-----|:-------|:------------|
| `snowplane_db_max_open_connections` | Gauge | `provider` | Maximum open connections allowed |
| `snowplane_db_open_connections` | Gauge | `provider` | Current open connections |
| `snowplane_db_in_use_connections` | Gauge | `provider` | Connections currently in use |
| `snowplane_db_idle_connections` | Gauge | `provider` | Connections currently idle |
| `snowplane_db_wait_count` | Gauge | `provider` | Cumulative connections waited for (resets on pool recreation) |
| `snowplane_db_wait_duration_seconds` | Gauge | `provider` | Cumulative time blocked waiting for connections (resets on pool recreation) |

### Webhook (controller-runtime)

When the validating admission webhook is enabled (`webhook.enabled: true`), controller-runtime exposes additional metrics:

| Metric | Type | Labels | Description |
|:-------|:-----|:-------|:------------|
| `controller_runtime_webhook_requests_total` | Counter | `webhook`, `code` | Total admission requests by HTTP status code |
| `controller_runtime_webhook_requests_in_flight` | Gauge | `webhook` | Currently processing admission requests |
| `controller_runtime_webhook_latency_seconds` | Histogram | `webhook` | Admission request processing latency |

{: .note }
> These metrics use the `controller_runtime_` prefix (not `snowplane_`) because they are emitted by the controller-runtime framework, not Snowplane directly.

---

## OpenTelemetry Tracing

Snowplane supports optional distributed tracing via OpenTelemetry (OTLP gRPC). When enabled, every reconcile operation emits spans that can be collected by any OTLP-compatible backend (Jaeger, Grafana Tempo, etc.).

### Enabling Tracing

Add the following flags to the manager deployment:

```yaml
args:
  - --enable-tracing
  - --otel-endpoint=tempo.monitoring:4317
  - --otel-sampling-ratio=0.1   # sample 10% of traces
  - --otel-insecure              # skip TLS (for in-cluster collectors)
```

| Flag | Default | Description |
|:-----|:--------|:------------|
| `--enable-tracing` | `false` | Enable OpenTelemetry tracing |
| `--otel-endpoint` | `localhost:4317` | OTLP gRPC collector endpoint |
| `--otel-sampling-ratio` | `1.0` | Trace sampling ratio (0.0–1.0) |
| `--otel-insecure` | `true` | Use insecure gRPC connection |

### Span Structure

Each reconciliation produces a parent span with child spans:

- **reconcile** — top-level span with resource type, namespace, and name attributes
  - **reconcile.create** — Snowflake CREATE operation
  - **reconcile.update** — Snowflake ALTER operation  
  - **reconcile.delete** — Snowflake DROP operation

Errors are automatically recorded on spans with `otel.status_code=ERROR`.

---

## ServiceMonitor

Enable automatic Prometheus scraping with the Helm chart:

```yaml
metrics:
  serviceMonitor:
    enabled: true
    additionalLabels:
      release: prometheus
    interval: 30s
```

---

## PrometheusRule

The Helm chart includes an optional PrometheusRule template with default alert rules for high error rates, circuit breaker trips, and Snowflake API latency:

```yaml
metrics:
  prometheusRule:
    enabled: true
    additionalLabels:
      release: prometheus
```

---

## Example Alert Rules

The following rules are shipped in the PrometheusRule template. You can also create them manually:

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

---

## Structured Logging

Snowplane uses `logr` backed by `zap` for structured JSON logging.

### Log Levels

| Level | Content |
|:------|:--------|
| `V(0)` (info) | Lifecycle transitions: creating, updating, deleting, drift detected, adoption |
| `V(1)` (debug) | Snowflake API calls, operation details, retry attempts, field-level diffs |

```bash
--zap-log-level=info   # V(0) only (default)
--zap-log-level=debug  # V(0) + V(1)
```

### Standard Fields

Every log line includes: `controller`, `namespace`, `name`, `reconcileID`.

### Sensitive Field Redaction

All log output is sanitized:
- **PEM keys** → `[REDACTED]`
- **Password/token fields** → `=[REDACTED]`
- **Connection strings** → `[connection redacted]`
- **Snowflake hostnames** → `[host redacted]`

---

## Kubernetes Events

### Normal Events

| Reason | Description |
|:-------|:------------|
| `ReconcileSuccess` | Reconciliation completed successfully (create, update, or no-op) |
| `Creating` | Resource is being created in Snowflake |
| `Deleting` | Resource is being deleted/dropped from Snowflake |
| `Adopted` | Pre-existing Snowflake resource adopted by operator |
| `DriftCorrected` | Out-of-band change detected and corrected |
| `ReconcilePaused` | Reconciliation paused via `spec.paused: true` |
| `FinalizerRemoved` | Finalizer removed from resource |
| `CredentialsRotated` | Snowflake credentials rotated successfully |
| `ForceNewActive` | Force-new annotation triggered delete+recreate |
| `CreateOrAlterFallback` | CREATE OR ALTER used as alter fallback |

### Warning Events

| Reason | Description |
|:-------|:------------|
| `DriftDetected` | Out-of-band change detected (before correction) |
| `ReconcileError` | Transient error (will be retried) |
| `RecoverableError` | Recoverable Snowflake error (will be retried with backoff) |
| `TerminalError` | Non-retryable error (reconciliation stopped, no further requeues) |
| `DependencyNotReady` | Pre-flight or pre-reconcile dependency not satisfied |
| `DependencyWait` | Waiting for a dependency to become ready |
| `ImmutableField` | Attempt to modify an immutable field |
| `ValidationFailed` | Spec validation failed |
| `ConflictDetected` | Another CR manages the same Snowflake object |
| `OrphanedResource` | Resource orphaned per orphan deletion policy |
| `DeleteBlocked` | Deletion blocked (e.g., resource still in use) |
| `InUse` | Resource is in use by dependents |
| `CredentialsError` | Snowflake credential retrieval failed |
| `SecretNotFound` | Referenced Kubernetes Secret not found |
| `InvalidConfig` | ProviderConfig configuration invalid |
| `ClientCreationFailed` | Snowflake client creation failed |
| `PingFailed` | Snowflake connection health check failed |
| `ResourceAlreadyExists` | Snowflake resource already exists (name collision) |
| `NamespaceNotAllowed` | CR namespace not in the ProviderConfig allowlist |
| `DatabaseNotAllowed` | Target database not in the ProviderConfig allowlist |
| `SchemaNotAllowed` | Target schema not in the ProviderConfig allowlist |
| `RoleNotAllowed` | Requested role not in the ProviderConfig allowlist |
| `UnsupportedAnnotation` | Unrecognized annotation on the CR |
| `RefResolutionFailed` | Cross-resource reference could not be resolved |

All event messages pass through `SafeRecorder`, which sanitizes via `ForEvent()` before emitting.

---

## Health Probes

| Endpoint | Check | Description |
|:---------|:------|:------------|
| `/healthz` | Ping | 200 if manager process is running |
| `/readyz` | Snowflake connectivity | Pings all cached clients via `SELECT 1` |

Configure with `--health-probe-bind-address` (default `:8081`).

A configurable **startup grace period** (default 30s) allows the readiness probe to pass during initial startup before any Snowflake client is cached.

The Helm deployment template includes a **startupProbe** (`httpGet /healthz:8081`, `failureThreshold: 30`, `periodSeconds: 2`) that provides a 60-second startup window before liveness takes over. This prevents the kubelet from killing the pod during slow initial Snowflake connections.

---

## Grafana Dashboard

A pre-built dashboard is available at `config/grafana/snowplane-dashboard.json`.

### Helm Chart Provisioning

```yaml
grafana:
  dashboard:
    enabled: true
    labels:
      grafana_dashboard: "1"
```

### Panels

- **Reconcile Rate** — Success/error rate per controller
- **Reconcile Duration** — P50/P95/P99 percentiles
- **Snowflake API Latency** — P50/P95/P99 per operation
- **Snowflake API Operations** — Operation counts
- **Client Pool Size** — Cached connections
- **Rate Limit Waits** — Throttling frequency
- **Drift Detection Frequency** — Events per controller
- **Adoption Outcomes** — Adopted/rejected
- **Circuit Breaker State** — Current state per provider
- **ProviderConfig Health** — Connection health per provider/account
- **Connection Pool** — Open, in-use, idle connections per provider

### Manual Installation

1. Open Grafana → Dashboards → Import
2. Upload `config/grafana/snowplane-dashboard.json`
3. Select your Prometheus datasource
4. Use the `Controller` dropdown to filter by resource type

---

## Circuit Breaker

Per-provider failure isolation in `internal/circuitbreaker/`:

| State | Behavior |
|:------|:---------|
| **Closed** | Normal operation |
| **Open** | After N failures — calls rejected with `DependencyNotReady` |
| **Half-Open** | After reset timeout — single probe allowed |

| Option | Default | Description |
|:-------|:--------|:------------|
| `FailureThreshold` | 5 | Consecutive failures before opening |
| `ResetTimeout` | 60s | Duration before probing |

Resources using other ProviderConfigs are completely unaffected.
