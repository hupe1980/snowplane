---
layout: default
title: Production Guide
parent: Guides
nav_order: 2
description: "Production hardening checklist and deployment best practices for Snowplane."
---

# Production Guide
{: .fs-8 }

Best practices for deploying Snowplane in production environments.
{: .fs-5 .fw-300 }

---

## Deployment Checklist

### Required

- [ ] **Use Helm** — Helm is the only supported production deployment path. Raw kustomize manifests lack RBAC guards present in the chart.
- [ ] **NetworkPolicy enabled** — Enabled by default (`networkPolicy.enabled: true`). Verify your CNI supports NetworkPolicy enforcement (Calico, Cilium, etc.).
- [ ] **Key pair authentication** — Use RSA key pair auth, not username/password. See [Getting Started]({% link getting-started.md %}).
- [ ] **Dedicated Snowflake role** — Create a least-privilege role (e.g., `SNOWPLANE_ADMIN`). See [Snowflake Role Setup]({% link snowflake-role-setup.md %}).
- [ ] **CRD installation** — Apply CRDs before Helm install. Helm 3 installs CRDs on first install but does not upgrade them.

### Recommended

- [ ] **Multiple replicas** — Run `replicaCount: 2` with leader election (enabled by default).
- [ ] **Pod topology** — Set `topologySpreadConstraints` or `affinity` for multi-AZ resilience.
- [ ] **Priority class** — Set `priorityClassName: system-cluster-critical` to prevent eviction.
- [ ] **ServiceMonitor** — Enable `metrics.serviceMonitor.enabled: true` for Prometheus scraping.
- [ ] **Grafana dashboard** — Enable `grafana.dashboard.enabled: true` for operational visibility.
- [ ] **Role allowlist** — Set `controller.allowedRoles` to restrict which Snowflake roles can be used in ProviderConfig resources.
- [ ] **Namespace scoping** — Use `watchNamespaces` to limit the controller's blast radius in multi-tenant clusters.

---

## Recommended Helm Values

```yaml
replicaCount: 2
priorityClassName: system-cluster-critical

controller:
  maxConcurrentReconciles: 5
  allowedRoles: "SYSADMIN,SNOWPLANE_ADMIN"

rateLimit:
  qps: 20
  burst: 40
  accountQps: 50
  accountBurst: 100

metrics:
  serviceMonitor:
    enabled: true

grafana:
  dashboard:
    enabled: true

networkPolicy:
  enabled: true
  restrictDNS: true
  # Restrict egress to Snowflake IP ranges + K8s API server
  # egressCIDRs:
  #   - 52.23.40.0/24    # Snowflake US East
  #   - 10.0.0.1/32      # K8s API server

topologySpreadConstraints:
  - maxSkew: 1
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: ScheduleAnyway
    labelSelector:
      matchLabels:
        app.kubernetes.io/name: snowplane
```

---

## Security Hardening

### NetworkPolicy

The NetworkPolicy is enabled by default and restricts:

| Direction | Rule | Default |
|:----------|:-----|:--------|
| **Egress** | DNS (port 53 UDP/TCP) | Restricted to kube-system namespace |
| **Egress** | HTTPS + K8s API (443, 6443) | All destinations (or `egressCIDRs` if set) |
| **Ingress** | Metrics + health probes | All sources (or `metricsNamespace` if set) |

For maximum security, configure `egressCIDRs` with your Snowflake account's IP ranges and K8s API server endpoint:

```yaml
networkPolicy:
  egressCIDRs:
    - 52.23.40.0/24      # Snowflake region IPs
    - 10.0.0.1/32        # K8s API server ClusterIP
  metricsNamespace: monitoring  # Only Prometheus can scrape
  extraEgress:
    - to:
        - ipBlock:
            cidr: 104.18.0.0/16  # OCSP responders
      ports:
        - port: 80
          protocol: TCP
```

### RBAC

The Helm chart provides granular RBAC toggles:

| Toggle | Default | Purpose |
|:-------|:--------|:--------|
| `rbac.secrets.read` | `true` | Read Secrets for credentials |
| `rbac.secrets.write` | `false` | Write Secrets (required for FieldExport to Secrets) |
| `rbac.configMaps.write` | `false` | Write ConfigMaps (required for FieldExport to ConfigMaps) |

Only enable `secrets.write` and `configMaps.write` if you use FieldExport.

### Role Allowlist

Restrict which Snowflake roles can be used in ProviderConfig resources:

```yaml
controller:
  allowedRoles: "SYSADMIN,USERADMIN,SNOWPLANE_ADMIN"
```

ProviderConfigs specifying a role not in the allowlist will be rejected with `Ready=False, reason=RoleNotAllowed`.

### Policy Enforcement

For organizational policy enforcement beyond what the controller provides, deploy [OPA/Gatekeeper](https://open-policy-agent.github.io/gatekeeper/) or [Kyverno](https://kyverno.io/) policies.

#### Kyverno Example: Block ACCOUNTADMIN in ProviderConfig

```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: block-accountadmin
spec:
  validationFailureAction: Enforce
  rules:
    - name: deny-accountadmin-role
      match:
        resources:
          kinds:
            - ProviderConfig
      validate:
        message: "ACCOUNTADMIN is not allowed in ProviderConfig. Use a least-privilege role."
        pattern:
          spec:
            role: "!ACCOUNTADMIN"
```

#### Kyverno Example: Require allowedNamespaces on ProviderConfig

```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: require-namespace-scoping
spec:
  validationFailureAction: Enforce
  rules:
    - name: require-allowed-namespaces
      match:
        resources:
          kinds:
            - ProviderConfig
      validate:
        message: "ProviderConfig must specify allowedNamespaces for multi-tenant isolation."
        pattern:
          spec:
            allowedNamespaces: "?*"
```

#### Kyverno Example: Block dangerous grants without annotation

```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: block-dangerous-grants
spec:
  validationFailureAction: Enforce
  rules:
    - name: deny-ownership-grants
      match:
        resources:
          kinds:
            - AccountRoleGrant
            - DatabaseRoleGrant
      validate:
        message: "OWNERSHIP grants are prohibited. Use specific privileges instead."
        deny:
          conditions:
            all:
              - key: "{{ request.object.spec.privilege }}"
                operator: Equals
                value: "OWNERSHIP"
```

#### OPA/Gatekeeper: ConstraintTemplate for role restriction

```yaml
apiVersion: templates.gatekeeper.sh/v1
kind: ConstraintTemplate
metadata:
  name: snowplaneroledenylist
spec:
  crd:
    spec:
      names:
        kind: SnowplaneRoleDenylist
      validation:
        openAPIV3Schema:
          type: object
          properties:
            deniedRoles:
              type: array
              items:
                type: string
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package snowplaneroledenylist
        violation[{"msg": msg}] {
          role := upper(input.review.object.spec.role)
          denied := upper(input.parameters.deniedRoles[_])
          role == denied
          msg := sprintf("Role '%v' is denied by policy", [role])
        }
---
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: SnowplaneRoleDenylist
metadata:
  name: block-admin-roles
spec:
  match:
    kinds:
      - apiGroups: ["snowplane.hupe1980.github.io"]
        kinds: ["ProviderConfig"]
  parameters:
    deniedRoles:
      - ACCOUNTADMIN
      - SECURITYADMIN
      - ORGADMIN
```

---

## Observability

### Metrics

Enable the ServiceMonitor for Prometheus scraping:

```yaml
metrics:
  serviceMonitor:
    enabled: true
    interval: 30s
    additionalLabels:
      release: prometheus  # Match your Prometheus operator selector
```

Key metrics to monitor:

| Metric | Alert Condition |
|:-------|:---------------|
| `snowplane_reconcile_errors_total` | Rate > 0 sustained for 5m |
| `snowplane_resource_health` | Value = 0 (unhealthy) for 10m |
| `snowplane_circuit_breaker_state` | Value = 2 (open) for 5m |
| `snowplane_account_rate_limit_waits_total` | Rate increasing — consider tuning `accountQps` |

### Grafana Dashboard

Enable the pre-built dashboard:

```yaml
grafana:
  dashboard:
    enabled: true
    labels:
      grafana_dashboard: "1"  # Match your Grafana sidecar selector
```

### Logging

For production, keep the default `info` level. Enable `debug` only for troubleshooting:

```yaml
logging:
  level: info       # info | debug | error
  development: false  # true = human-readable, false = JSON
```

---

## Scaling

### Horizontal Scaling

Snowplane supports horizontal scaling via leader election. The active leader handles all reconciliation; standby replicas provide fast failover.

For very large deployments (10K+ managed resources), consider namespace-based sharding:

```bash
# Instance 1: teams A and B
helm install snowplane-ab charts/snowplane/ \
  --set watchNamespaces="team-a,team-b" \
  --set leaderElectionID=snowplane-ab

# Instance 2: teams C and D
helm install snowplane-cd charts/snowplane/ \
  --set watchNamespaces="team-c,team-d" \
  --set leaderElectionID=snowplane-cd
```

### Rate Limiting

Tune rate limits based on your Snowflake account tier:

| Snowflake Tier | Recommended `accountQps` | Recommended `accountBurst` |
|:---------------|:------------------------|:--------------------------|
| Standard | 30 | 60 |
| Enterprise | 50 | 100 |
| Business Critical | 100 | 200 |

Per-controller limits (`rateLimit.qps`) ensure fairness — a noisy controller cannot starve others within the account budget.

---

## Upgrade Procedure

1. **Update CRDs first** — Helm does not upgrade CRDs automatically:
   ```bash
   kubectl apply --server-side -f charts/snowplane/crds/
   ```

2. **Upgrade the release:**
   ```bash
   helm upgrade snowplane charts/snowplane/ \
     --namespace snowplane-system \
     --reuse-values
   ```

3. **Verify health:**
   ```bash
   kubectl -n snowplane-system rollout status deployment/snowplane
   kubectl get providerconfig -o wide
   ```

---

## Disaster Recovery

### CRD State Backup

Back up all Snowplane custom resources:

```bash
for crd in $(kubectl get crd -o name | grep snowplane); do
  kind=$(echo "$crd" | sed 's|customresourcedefinition.apiextensions.k8s.io/||; s|\.snowplane.*||')
  kubectl get "$kind" --all-namespaces -o yaml > "backup-${kind}.yaml"
done
```

### Restore

Apply the backed-up resources. The controller will reconcile and adopt existing Snowflake objects:

```bash
for f in backup-*.yaml; do
  kubectl apply -f "$f"
done
```

{: .warning }
> Set `spec.managementPolicies.adoptionPolicy: adopt` on resources if the Snowflake objects already exist to prevent creation errors.
