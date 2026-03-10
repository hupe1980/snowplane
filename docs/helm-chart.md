---
layout: default
title: Helm Chart
parent: Guides
nav_order: 1
description: "Install and configure Snowplane using the production-ready Helm chart."
---

# Helm Chart
{: .fs-8 }

Snowplane includes a production-ready Helm chart at `charts/snowplane/` for installing the operator.
{: .fs-5 .fw-300 }

---

## Installation

```bash
helm install snowplane charts/snowplane/ \
  --namespace snowplane-system --create-namespace
```

## Upgrade

```bash
helm upgrade snowplane charts/snowplane/ \
  --namespace snowplane-system
```

{: .note }
> CRDs are installed automatically on first install but are not updated on upgrade (per Helm 3 CRD policy). To update CRDs, apply them manually:
> ```bash
> kubectl apply -f charts/snowplane/crds/
> ```

---

## Configuration

### Core Settings

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `replicaCount` | `1` | Controller manager replicas |
| `image.repository` | `ghcr.io/hupe1980/snowplane` | Container image |
| `image.tag` | Chart `appVersion` | Image tag |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy |

### Controller

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `controller.maxConcurrentReconciles` | `3` | Max concurrent reconciles per controller |
| `controller.requeueInterval` | `5m` | Drift detection resync interval |
| `controller.snowflakeOpTimeout` | `60s` | Timeout for individual Snowflake API operations |
| `controller.enableAlphaResources` | `true` | Enable alpha-maturity controllers |
| `controller.disableControllers` | `""` | Comma-separated list of controllers to disable |
| `controller.allowedRoles` | `""` | Comma-separated allowlist of permitted Snowflake roles (case-insensitive; empty = all allowed) |

### Rate Limiting

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `rateLimit.qps` | `10` | Per-controller sustained QPS to Snowflake per provider |
| `rateLimit.burst` | `20` | Per-controller burst size for the rate limiter |
| `rateLimit.accountQps` | `50` | Per-account aggregate QPS across all controllers |
| `rateLimit.accountBurst` | `100` | Per-account aggregate burst size |

### Circuit Breaker

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `circuitBreaker.threshold` | `5` | Number of consecutive failures before the circuit opens |
| `circuitBreaker.resetTimeout` | `60s` | Duration the circuit stays open before allowing a probe request |

### Sharding

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `sharding.enabled` | `false` | Enable hash-based controller sharding for horizontal scaling |
| `sharding.shardID` | `0` | Zero-based shard index for this instance |
| `sharding.shardCount` | `1` | Total number of shards (must be ≥ 1) |

When sharding is enabled, deploy one Helm release per shard. Leader election IDs are automatically suffixed with the shard index. See the [production guide](production-guide.md#horizontal-scaling) for deployment examples.

### Metrics & Monitoring

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `metrics.enabled` | `true` | Enable metrics endpoint |
| `metrics.bindAddress` | `:8080` | Metrics bind address |
| `metrics.containerPort` | `8080` | Container port for metrics endpoint |
| `metrics.service.enabled` | `true` | Create a Service for metrics |
| `metrics.service.port` | `8080` | Metrics service port |
| `metrics.serviceMonitor.enabled` | `false` | Create Prometheus ServiceMonitor |
| `metrics.serviceMonitor.additionalLabels` | `{}` | Extra labels for ServiceMonitor |
| `metrics.serviceMonitor.interval` | `30s` | Scrape interval |

### Health Probes

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `healthProbes.containerPort` | `8081` | Container port for health probe endpoint |

### Logging

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `logging.level` | `info` | Log level (`info`, `debug`, `error`) |
| `logging.development` | `false` | Human-readable debug output |

### High Availability

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `leaderElection.enabled` | `true` | Enable leader election |
| `leaderElectionID` | `snowplane-leader-election` | Leader election identity |
| `podDisruptionBudget.enabled` | `true` | Create a PodDisruptionBudget |
| `podDisruptionBudget.maxUnavailable` | `1` | Maximum unavailable pods during disruptions |

### Deployment

The Deployment uses an automatic rollout strategy:
- **Single replica** (`replicaCount: 1`): `Recreate` strategy — avoids two pods competing for the leader lease during upgrades.
- **Multiple replicas** (`replicaCount: 2+`): `RollingUpdate` with `maxSurge: 0` — prevents exceeding the intended replica count.

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `revisionHistoryLimit` | `3` | Number of old ReplicaSets to retain |

### Scheduling

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `nodeSelector` | `{}` | Node selector constraints |
| `tolerations` | `[]` | Pod tolerations |
| `affinity` | `{}` | Pod affinity rules |
| `topologySpreadConstraints` | `[]` | Topology spread constraints |
| `priorityClassName` | `""` | PriorityClass name |

### Webhook

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `webhook.enabled` | `false` | Enable the validating admission webhook for ownership conflict detection |
| `webhook.failurePolicy` | `Ignore` | Webhook failure policy (`Ignore` or `Fail`) |
| `webhook.timeoutSeconds` | `10` | Webhook request timeout |
| `webhook.port` | `9443` | Webhook server listen port |
| `webhook.certDir` | `/tmp/k8s-webhook-server/serving-certs` | TLS certificate directory |
| `webhook.namespaceSelector` | `{}` | Limit webhook to matching namespaces |
| `webhook.certManager.enabled` | `true` | Use cert-manager for automatic TLS certificates |
| `webhook.certManager.issuerRef` | `{}` | Custom cert-manager issuer reference (empty = self-signed) |
| `webhook.caBundle` | `""` | Manual CA bundle (only when `certManager.enabled: false`) |

{: .note }
> The webhook requires TLS certificates. When `certManager.enabled: true` (default), a self-signed Issuer and Certificate are created automatically. If you use a different certificate solution, disable cert-manager and provide `caBundle` manually.

### Extensibility

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `extraEnv` | `[]` | Additional environment variables for the controller container |
| `extraVolumes` | `[]` | Additional volumes to attach to the pod |
| `extraVolumeMounts` | `[]` | Additional volume mounts for the controller container |

### Namespace Scoping

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `watchNamespaces` | `""` | Comma-separated namespaces to watch (empty = all) |

### Grafana Dashboard

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `grafana.dashboard.enabled` | `false` | Deploy a Grafana dashboard ConfigMap |
| `grafana.dashboard.namespace` | Release namespace | Namespace for the dashboard ConfigMap |
| `grafana.dashboard.labels` | `{}` | Additional labels for sidecar discovery |
| `grafana.dashboard.annotations` | `{}` | Annotations for the dashboard ConfigMap |

### Security

| Parameter | Default | Description |
|:----------|:--------|:------------|
| `serviceAccount.create` | `true` | Create a ServiceAccount |
| `serviceAccount.annotations` | `{}` | SA annotations (e.g., for IRSA/WI) |
| `podSecurityContext.runAsNonRoot` | `true` | Run as non-root |
| `networkPolicy.enabled` | `true` | Create a NetworkPolicy restricting ingress/egress |
| `networkPolicy.restrictDNS` | `true` | Restrict DNS egress to kube-system namespace (kube-dns/CoreDNS) |
| `networkPolicy.egressCIDRs` | `[]` | Restrict HTTPS egress to specific CIDRs (e.g., Snowflake IP ranges + K8s API server). Empty = allow all |
| `networkPolicy.metricsNamespace` | `""` | Restrict metrics/health-probe ingress to pods in this namespace. Empty = allow all |
| `networkPolicy.extraEgress` | `[]` | Additional egress rules appended to the policy |
| `networkPolicy.extraIngress` | `[]` | Additional ingress rules appended to the policy |

---

## Templates

| Template | Description |
|:---------|:------------|
| `deployment.yaml` | Controller manager Deployment with all flags |
| `serviceaccount.yaml` | ServiceAccount with configurable annotations |
| `rbac.yaml` | ClusterRole, ClusterRoleBinding, viewer/editor ClusterRoles |
| `service-metrics.yaml` | Service for the metrics endpoint |
| `servicemonitor.yaml` | Prometheus ServiceMonitor (optional) |
| `pdb.yaml` | PodDisruptionBudget (optional) |
| `networkpolicy.yaml` | NetworkPolicy restricting traffic (optional) |
| `grafana-dashboard.yaml` | Grafana dashboard ConfigMap for sidecar provisioning (optional) |
| `webhook-validating.yaml` | ValidatingWebhookConfiguration for ownership conflict detection (optional) |
| `webhook-service.yaml` | Service routing to the webhook endpoint (optional) |
| `webhook-certificate.yaml` | cert-manager Issuer and Certificate for webhook TLS (optional) |

---

## RBAC

The chart creates three ClusterRoles:

**Controller ClusterRole** — Full permissions for the operator: CRDs (get/list/watch/create/update/patch/delete), status/finalizers subresources, Secrets (read-only), ConfigMaps, Events, Leases.

**Viewer ClusterRole** (`<release>-viewer`) — Read-only access to all Snowplane CRDs and their status subresources.

**Editor ClusterRole** (`<release>-editor`) — Full CRUD on all Snowplane CRDs and read access to status.

---

## CRDs

CRDs are placed in `charts/snowplane/crds/` and installed automatically on first `helm install`. After modifying API types:

```bash
just generate     # Regenerate from markers
just sync-crds    # Copy into charts/snowplane/crds/
just verify-crds  # CI check: fails if out-of-sync
```

---

## Production Deployment

```bash
helm install snowplane charts/snowplane/ \
  --namespace snowplane-system --create-namespace \
  --set replicaCount=2 \
  --set controller.maxConcurrentReconciles=5 \
  --set rateLimit.qps=20 \
  --set metrics.serviceMonitor.enabled=true \
  --set priorityClassName=system-cluster-critical \
  --set grafana.dashboard.enabled=true \
  --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="arn:aws:iam::123456789012:role/snowplane"
```

### Multi-Instance Deployment

For multiple Snowplane instances (e.g., different Snowflake accounts), use unique leader election IDs and namespace scoping:

```bash
helm install snowplane-prod charts/snowplane/ \
  --namespace snowplane-prod --create-namespace \
  --set leaderElectionID=snowplane-prod-leader \
  --set watchNamespaces="team-a,team-b"
```
