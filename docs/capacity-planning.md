---
layout: default
title: Capacity Planning
parent: Guides
nav_order: 6
description: "Resource sizing, rate limit tuning, and scaling guidance for Snowplane deployments."
---

# Capacity Planning
{: .fs-8 }

Size your Snowplane deployment based on the number of managed Snowflake resources, reconcile frequency, and Snowflake account tier.
{: .fs-5 .fw-300 }

---

## Deployment Sizes

| Size | Managed Resources | Memory Limit | CPU Limit | MaxConcurrentReconciles |
|:-----|:-----------------|:-------------|:----------|:----------------------|
| **Small** | 1–1,000 | 512Mi | 500m | 3 (default) |
| **Medium** | 1,000–10,000 | 1Gi | 1 | 5 |
| **Large** | 10,000–50,000 | 2Gi | 2 | 5–10 |
| **Very Large** | 50,000+ | 4Gi+ | 4+ | 10+ with sharding |

{: .note }
> "Managed Resources" is the total count across all 75+ CRD types in the cluster.

---

## Memory Consumption

Memory usage breaks down into three categories:

### 1. Informer Cache

The controller-runtime informer caches all watched objects in memory. Each cached object consumes **2–5 KB** depending on spec size and status.

| Total Resources | Estimated Cache | Recommended Memory |
|:---------------|:---------------|:------------------|
| 500 | ~2 MB | 512Mi |
| 5,000 | ~20 MB | 512Mi–1Gi |
| 20,000 | ~80 MB | 1Gi–2Gi |
| 50,000 | ~200 MB | 2Gi–4Gi |

### 2. Snowflake Client Pool

Each `ProviderConfig` maintains a cached Snowflake client with an underlying `database/sql` connection pool. Each client consumes **~5–10 MB** of memory including driver buffers.

| ProviderConfigs | Client Pool Memory |
|:---------------|:------------------|
| 1–3 | ~30 MB |
| 5–10 | ~50–100 MB |
| 20+ | ~200+ MB |

### 3. Go Runtime Overhead

The Go runtime, goroutine stacks, and GC metadata add **~100–200 MB** baseline overhead.

**Formula:**

$$\text{Memory} \approx 200\text{MB} + (5\text{KB} \times \text{resources}) + (10\text{MB} \times \text{providers})$$

---

## CPU Consumption

CPU usage is bursty — spikes during reconciliation, idle between cycles.

| Factor | Impact |
|:-------|:-------|
| Reconcile rate | ~0.5ms CPU per reconcile (K8s API + status patch) |
| Snowflake API calls | Blocked on I/O, minimal CPU |
| Drift detection | ~1ms per resource (field comparison + hash) |
| CRD informer sync | Periodic watch reconnections, negligible |

For most deployments, **500m CPU** is sufficient. Increase to **1–2 cores** for 10,000+ resources with high churn rates.

---

## Rate Limit Tuning

Snowplane uses a two-tier rate limiter to prevent overwhelming the Snowflake API.

### Per-Controller Rate Limit

Controls how fast each CRD controller (e.g., Database, Schema, Table) can issue Snowflake API calls.

```yaml
rateLimit:
  qps: 10    # Queries per second (default)
  burst: 20  # Burst capacity (default)
```

### Per-Account Rate Limit

Aggregate Snowflake API budget shared across all controllers using the same Snowflake account (ProviderConfig).

```yaml
rateLimit:
  accountQps: 50     # Total QPS per account (default)
  accountBurst: 100   # Total burst per account (default)
```

### Tuning by Snowflake Account Tier

| Snowflake Tier | Recommended `accountQps` | Recommended `accountBurst` | Per-Controller `qps` |
|:---------------|:------------------------|:--------------------------|:--------------------|
| Standard | 30 | 60 | 10 |
| Enterprise | 50 | 100 | 15 |
| Business Critical | 100 | 200 | 20 |

{: .warning }
> Set `accountQps` to **50–80%** of your Snowflake account's actual API rate limit to leave headroom for non-operator traffic (dashboards, queries, other automation).

### Monitoring Rate Limits

Watch for rate limit pressure via Prometheus:

```promql
# Per-controller rate limit waits (should be near zero)
rate(snowplane_rate_limit_waits_total[5m])

# Per-account rate limit waits (indicates global pressure)
rate(snowplane_account_rate_limit_waits_total[5m])
```

If rate limit waits are sustained, either increase the rate limit or reduce `MaxConcurrentReconciles`.

---

## MaxConcurrentReconciles

Controls how many objects of each CRD type are reconciled in parallel. Each controller gets its own worker pool.

```yaml
controller:
  maxConcurrentReconciles: 3  # default
```

| Value | Use Case |
|:------|:---------|
| 1 | Lowest API pressure, slowest convergence |
| 3 | Default — good balance for most deployments |
| 5 | Medium deployments, faster convergence |
| 10 | Large deployments with generous rate limits |
| 20+ | Very large deployments with sharding |

**Total concurrent Snowflake API calls** = `maxConcurrentReconciles` × number of active controllers (~68). Most reconciles complete quickly (observe + no-op), so actual concurrency is much lower than the theoretical maximum.

---

## Reconcile Timing

### Sync Period

The controller-runtime sync period (default: **5 minutes**) determines how often resources are re-reconciled even without changes. This drives drift detection frequency.

### Reconcile Timeout

Each reconciliation has a **5-minute overall timeout** (configurable). Individual Snowflake API calls have a **60-second timeout**. Snowflake sessions enforce a **300-second STATEMENT_TIMEOUT_IN_SECONDS** server-side.

| Timeout | Default | Purpose |
|:--------|:--------|:--------|
| Reconcile timeout | 5 min | Total time for one reconcile cycle |
| Operation timeout | 60s | Per Snowflake API call |
| Statement timeout | 300s | Snowflake server-side query timeout |

---

## Scaling Strategies

### Vertical Scaling

For most deployments, increase memory and MaxConcurrentReconciles:

```yaml
resources:
  limits:
    memory: 1Gi
    cpu: 1
  requests:
    memory: 512Mi
    cpu: 250m

controller:
  maxConcurrentReconciles: 5
```

### Namespace Sharding

Run multiple Snowplane instances, each watching a subset of namespaces:

```bash
# Instance 1: production namespaces
helm install snowplane-prod charts/snowplane/ \
  --set watchNamespaces="prod-team-a,prod-team-b" \
  --set leaderElectionID=snowplane-prod

# Instance 2: staging namespaces
helm install snowplane-staging charts/snowplane/ \
  --set watchNamespaces="staging-team-a,staging-team-b" \
  --set leaderElectionID=snowplane-staging
```

### Hash-Based Sharding

For very large deployments (50,000+ resources), use deterministic hash-based sharding:

```bash
for i in 0 1 2; do
  helm install snowplane-shard-$i charts/snowplane/ \
    --set sharding.enabled=true \
    --set sharding.shardID=$i \
    --set sharding.shardCount=3
done
```

Each shard owns a deterministic subset of resources based on FNV-1a hash of `namespace/name`. Leader election IDs are automatically made shard-specific, so no manual `leaderElectionID` override is needed.

---

## Monitoring Dashboard

Use these PromQL queries to monitor resource consumption:

```promql
# Memory usage trend
container_memory_working_set_bytes{container="snowplane"}

# Reconcile throughput
sum(rate(snowplane_reconcile_total[5m])) by (controller)

# Reconcile error rate
sum(rate(snowplane_reconcile_total{result="error"}[5m])) by (controller)

# Snowflake API latency P99
histogram_quantile(0.99, rate(snowplane_snowflake_operation_duration_seconds_bucket[5m]))

# Active connections per provider
snowplane_db_in_use_connections

# Circuit breaker state (0=closed, 1=open, 2=half-open)
snowplane_circuit_breaker_state
```

---

## Capacity Planning Checklist

- [ ] Count total managed resources across all CRD types
- [ ] Count number of ProviderConfigs (Snowflake accounts)
- [ ] Identify Snowflake account tier for rate limit tuning
- [ ] Calculate memory using the formula above
- [ ] Set MaxConcurrentReconciles based on deployment size
- [ ] Configure rate limits to 50–80% of Snowflake API limits
- [ ] Enable ServiceMonitor and Grafana dashboard for monitoring
- [ ] Set appropriate resource requests and limits in Helm values
- [ ] Consider sharding for 50,000+ resources

---

## Further Reading

- [Production Guide]({% link production-guide.md %}) — Security hardening and deployment checklist
- [Observability]({% link observability.md %}) — Metrics reference and alerting
- [Helm Chart]({% link helm-chart.md %}) — Full Helm values reference
