# Helm Chart

Snowplane includes a production-ready Helm chart at `charts/snowplane/` for installing the operator.

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

> **Note:** CRDs are installed automatically on first install but are not updated on upgrade (per Helm 3 CRD policy). To update CRDs, apply them manually:
>
> ```bash
> kubectl apply -f charts/snowplane/crds/
> ```

## Configuration

### Core Settings

| Parameter | Default | Description |
|-----------|---------|-------------|
| `replicaCount` | `1` | Controller manager replicas |
| `image.repository` | `ghcr.io/hupe1980/snowplane` | Container image |
| `image.tag` | Chart `appVersion` | Image tag |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy |

### Controller

| Parameter | Default | Description |
|-----------|---------|-------------|
| `controller.maxConcurrentReconciles` | `3` | Max concurrent reconciles per controller |
| `controller.requeueInterval` | `5m` | Drift detection resync interval |
| `controller.enableAlphaResources` | `true` | Enable alpha-maturity controllers |
| `controller.disableControllers` | `""` | Comma-separated list of controllers to disable (e.g. `"grant,view"`) |

### Rate Limiting

| Parameter | Default | Description |
|-----------|---------|-------------|
| `rateLimit.qps` | `10` | Sustained QPS to Snowflake per provider |
| `rateLimit.burst` | `20` | Burst size for the rate limiter |

### Metrics & Monitoring

| Parameter | Default | Description |
|-----------|---------|-------------|
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
|-----------|---------|-------------|
| `healthProbes.containerPort` | `8081` | Container port for health probe endpoint |

### Logging

| Parameter | Default | Description |
|-----------|---------|-------------|
| `logging.level` | `info` | Log level (`info`, `debug`, `error`) |
| `logging.development` | `false` | Human-readable debug output |

### High Availability

| Parameter | Default | Description |
|-----------|---------|-------------|
| `leaderElection.enabled` | `true` | Enable leader election |
| `leaderElectionID` | `snowplane-leader-election` | Leader election identity (override for multi-instance clusters) |
| `podDisruptionBudget.enabled` | `true` | Create a PodDisruptionBudget |
| `podDisruptionBudget.maxUnavailable` | `1` | Maximum unavailable pods during voluntary disruptions |

### Scheduling

| Parameter | Default | Description |
|-----------|---------|-------------|
| `nodeSelector` | `{}` | Node selector constraints |
| `tolerations` | `[]` | Pod tolerations |
| `affinity` | `{}` | Pod affinity rules |
| `topologySpreadConstraints` | `[]` | Topology spread constraints for zone-aware scheduling |
| `priorityClassName` | `""` | PriorityClass name (e.g., `system-cluster-critical` for production) |

### Namespace Scoping

| Parameter | Default | Description |
|-----------|---------|-------------|
| `watchNamespaces` | `""` | Comma-separated list of namespaces to watch (empty = all namespaces) |

### Grafana Dashboard

| Parameter | Default | Description |
|-----------|---------|-------------|
| `grafana.dashboard.enabled` | `false` | Deploy a Grafana dashboard ConfigMap |
| `grafana.dashboard.namespace` | Release namespace | Namespace for the dashboard ConfigMap |
| `grafana.dashboard.labels` | `{}` | Additional labels for sidecar discovery |
| `grafana.dashboard.annotations` | `{}` | Annotations for the dashboard ConfigMap |

### Security

| Parameter | Default | Description |
|-----------|---------|-------------|
| `serviceAccount.create` | `true` | Create a ServiceAccount |
| `serviceAccount.annotations` | `{}` | SA annotations (e.g., for IRSA/WI) |
| `podSecurityContext.runAsNonRoot` | `true` | Run as non-root |
| `networkPolicy.enabled` | `false` | Create a NetworkPolicy |

## Templates

The chart includes the following templates:

| Template | Description |
|----------|-------------|
| `deployment.yaml` | Controller manager Deployment with all flags wired |
| `serviceaccount.yaml` | ServiceAccount with configurable annotations |
| `rbac.yaml` | ClusterRole, ClusterRoleBinding, viewer/editor ClusterRoles |
| `service-metrics.yaml` | Service for the metrics endpoint |
| `servicemonitor.yaml` | Prometheus ServiceMonitor (optional) |
| `pdb.yaml` | PodDisruptionBudget (optional) |
| `networkpolicy.yaml` | NetworkPolicy restricting traffic (optional) |
| `grafana-dashboard.yaml` | ConfigMap with Grafana dashboard JSON for sidecar provisioning (optional) |
| `NOTES.txt` | Post-install instructions |

## RBAC

The chart creates three ClusterRoles:

### Controller ClusterRole

Full permissions required by the operator:

- **CRDs:** get/list/watch/create/update/patch/delete on all 26 Snowplane resource types
- **Status/Finalizers:** get/patch/update on status subresources; update on finalizer subresources
- **Secrets:** get/list/watch (read-only access for ProviderConfig credential resolution)
- **ConfigMaps:** get/list/watch/create/update/patch
- **Events:** create/patch
- **Leases:** get/create/update (leader election)

### Viewer ClusterRole (`<release>-viewer`)

Read-only access to all Snowplane CRDs and their status subresources.

### Editor ClusterRole (`<release>-editor`)

Full CRUD on all Snowplane CRDs and read access to status subresources.

## CRDs

CRDs are placed in `charts/snowplane/crds/` and installed automatically on first `helm install`. Per Helm 3 convention, CRDs are not modified on subsequent upgrades.

### Keeping CRDs in Sync

The canonical CRD source is `config/crd/bases/`, generated by `controller-gen` from kubebuilder markers. The Helm chart's `crds/` directory is a copy. After modifying API types:

```bash
just generate     # Regenerate from markers
just sync-crds    # Copy into charts/snowplane/crds/
just verify-crds  # CI check: fails if out-of-sync
```

For CRD-only installation:

```bash
kubectl apply -f charts/snowplane/crds/
```

## Example: Production Deployment

```bash
helm install snowplane charts/snowplane/ \
  --namespace snowplane-system --create-namespace \
  --set replicaCount=2 \
  --set controller.maxConcurrentReconciles=5 \
  --set rateLimit.qps=20 \
  --set metrics.serviceMonitor.enabled=true \
  --set networkPolicy.enabled=true \
  --set priorityClassName=system-cluster-critical \
  --set grafana.dashboard.enabled=true \
  --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="arn:aws:iam::123456789012:role/snowplane"
```

### Multi-Instance Deployment

When running multiple Snowplane instances in the same cluster (e.g., for different Snowflake accounts), use unique leader election IDs and namespace scoping:

```bash
helm install snowplane-prod charts/snowplane/ \
  --namespace snowplane-prod --create-namespace \
  --set leaderElectionID=snowplane-prod-leader \
  --set watchNamespaces="team-a,team-b"
```
