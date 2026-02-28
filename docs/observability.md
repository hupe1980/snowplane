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
| `snowplane_managed_resources` | Gauge | `controller`, `state` | Resources by controller and state |
| `snowplane_adoption_total` | Counter | `controller`, `result` | Resource adoption outcomes |
| `snowplane_drift_detected_total` | Counter | `controller` | Drift detection events |
| `snowplane_orphaned_resources_total` | Counter | `controller` | Orphan-policy deletions |
| `snowplane_ownership_conflicts_total` | Counter | `controller` | Ownership conflicts detected |

### Circuit Breaker

| Metric | Type | Labels | Description |
|:-------|:-----|:-------|:------------|
| `snowplane_circuit_breaker_trips_total` | Counter | `provider` | Circuit breaker open events |
| `snowplane_circuit_breaker_state` | Gauge | `provider` | 0 = closed, 1 = open, 2 = half-open |

### ProviderConfig Health

| Metric | Type | Labels | Description |
|:-------|:-----|:-------|:------------|
| `snowplane_providerconfig_healthy` | Gauge | `provider`, `account` | 1 = connected, 0 = unhealthy |

### Connection Pool

| Metric | Type | Labels | Description |
|:-------|:-----|:-------|:------------|
| `snowplane_db_max_open_conns` | Gauge | `provider` | Maximum open connections allowed |
| `snowplane_db_open_conns` | Gauge | `provider` | Current open connections |
| `snowplane_db_in_use_conns` | Gauge | `provider` | Connections currently in use |
| `snowplane_db_idle_conns` | Gauge | `provider` | Connections currently idle |
| `snowplane_db_wait_count` | Gauge | `provider` | Total connections waited for |
| `snowplane_db_wait_duration_seconds` | Gauge | `provider` | Time blocked waiting for connections |

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

## Example Alert Rules

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
| `Created` | Resource created in Snowflake |
| `Updated` | Resource updated (spec change applied) |
| `Deleted` / `Orphaned` | Resource dropped or orphaned |
| `AdoptedExisting` | Pre-existing resource adopted |
| `DriftCorrected` | Out-of-band change corrected |

### Warning Events

| Reason | Description |
|:-------|:------------|
| `DriftDetected` | Out-of-band change detected |
| `ReconcileError` | Transient error (will be retried) |
| `TerminalError` | Non-retryable error (reconciliation stopped, no further requeues) |
| `DependencyNotReady` | ProviderConfig or parent not ready |
| `ImmutableField` | Attempt to modify an immutable field |
| `ConflictDetected` | Another CR manages the same Snowflake object |

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
- **Managed Resources** — By controller and state
- **Circuit Breaker State** — Current state per provider

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
