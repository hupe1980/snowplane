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

- [ ] **Ownership webhook** — Enable the validating admission webhook (`webhook.enabled: true`) to reject duplicate Snowflake resource mappings at admission time. Requires cert-manager. See [Architecture — Admission Webhook]({% link architecture.md %}#admission-webhook).
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

webhook:
  enabled: true
  failurePolicy: Ignore
  certManager:
    enabled: true

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

### Ownership Webhook

The validating admission webhook intercepts `CREATE` and `UPDATE` requests for all snowplane CRDs, rejecting requests that would create duplicate Snowflake resource mappings. This complements the reconciler-level conflict detection by shifting checks left to admission time.

```yaml
webhook:
  enabled: true
  failurePolicy: Ignore  # Ignore = best-effort, Fail = strict
  certManager:
    enabled: true
```

{: .note }
> The webhook uses `failurePolicy: Ignore` by default — if the webhook pod is unavailable, requests are allowed through. Set `failurePolicy: Fail` only if you can tolerate API rejections during webhook downtime.

See [Architecture — Admission Webhook]({% link architecture.md %}#admission-webhook) for implementation details.

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
        message: "ProviderConfig must specify allowedNamespaces or allowedNamespaceSelector for multi-tenant isolation."
        pattern:
          spec:
            allowedNamespaces: "?*"
```

### Resource Scoping

ProviderConfig supports restricting which Snowflake databases and schemas resources may target:

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: team-a
spec:
  # ...credentials...
  allowedNamespaces:
    - team-a-ns
  allowedNamespaceSelector:
    matchLabels:
      team: analytics
  allowedDatabases:
    - TEAM_A_DB
    - SHARED_DB
  allowedSchemas:
    - TEAM_A_DB.PUBLIC
    - SHARED_DB.ANALYTICS
```

- **`allowedDatabases`**: Case-insensitive list. When non-empty, resources using this ProviderConfig can only target listed databases. Empty = all allowed.
- **`allowedSchemas`**: Supports `"SCHEMA"` (any database) or `"DATABASE.SCHEMA"` format. Empty = all allowed.
- **`allowedNamespaceSelector`**: Label selector matching namespaces. Used as OR with the static `allowedNamespaces` list — a namespace is permitted if it matches either.

> **Warning:** When both `allowedNamespaces` is empty and `allowedNamespaceSelector` is nil, **all namespaces** can use the ProviderConfig. This is the default behavior. For multi-tenant production deployments, always configure at least one of these fields or enforce it via a Kyverno/OPA policy (see examples above).

Resources violating these constraints are rejected with `DatabaseNotAllowed` or `SchemaNotAllowed` condition reasons.

### Pre-flight Validation

The controller automatically validates that referenced Snowflake databases and schemas exist before issuing CREATE commands. This prevents opaque SQL errors and provides clear `DependencyNotReady` conditions.

- **CR references** (`databaseRef`/`schemaRef`): Existence is validated during reference resolution — the referenced CR must be `Ready=True`.
- **Raw strings** (`databaseName`/`schemaName`): The controller issues `SHOW DATABASES LIKE`/`SHOW SCHEMAS LIKE` queries to verify existence before CREATE.

No configuration is needed — pre-flight checks run automatically for all 38 database/schema-scoped resource types.

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
            - GrantPrivilegesToAccountRole
            - GrantPrivilegesToDatabaseRole
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

## SQLStatement Security

The SQLStatement CRD is an intentional escape hatch for executing arbitrary SQL. It is gated behind `--enable-sql-statement` (disabled by default) and carries additional hardening layers.

### Statement-Type Denylist

Block specific SQL statement types at the controller level:

```yaml
# values.yaml
controller:
  enableSQLStatement: true
  sqlStatementDenylist: "DROP DATABASE,DROP SCHEMA,ALTER USER,DROP USER"
```

Matching is case-insensitive with word-boundary detection. Denied statements receive a `TerminalError` condition and are never sent to Snowflake. The `snowplane_sqlstatement_denied_total` metric counts rejections.

### Namespace Restriction (ValidatingAdmissionPolicy)

Restrict which namespaces may create SQLStatement resources using a Kubernetes 1.30+ native policy:

```bash
# Deploy the policy
kubectl apply -f config/admission/sqlstatement-namespace-restriction.yaml

# Create the allowlist ConfigMap
kubectl create configmap sqlstatement-namespace-allowlist \
  --namespace=snowplane-system \
  --from-literal=allowedNamespaces="data-eng,platform-team"
```

When the ConfigMap is missing, the policy defaults to **Deny** (fail-closed).

### RBAC Restriction

Limit SQLStatement access to platform teams only:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: sqlstatement-admin
rules:
  - apiGroups: ["snowplane.hupe1980.github.io"]
    resources: ["sqlstatements"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: sqlstatement-admin-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: sqlstatement-admin
subjects:
  - kind: Group
    name: platform-team
    apiGroup: rbac.authorization.k8s.io
```

> **Recommendation:** Do not include SQLStatement in the default `snowplane-editor` ClusterRole. Grant access only to teams that need it.

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
| `snowplane_sqlstatement_denied_total` | Any increment — investigate blocked SQL attempts |
| `snowplane_policy_body_rejections_total` | Any increment — investigate potential injection probing |

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

For very large deployments (10K+ managed resources), use hash-based controller sharding to distribute reconciliation across multiple manager instances:

```bash
# Deploy 3 sharded replicas (one Helm release per shard)
for i in 0 1 2; do
  helm install snowplane-shard-$i charts/snowplane/ \
    --set sharding.enabled=true \
    --set sharding.shardID=$i \
    --set sharding.shardCount=3
done
```

Each shard deterministically owns a subset of resources based on FNV-1a hash of `namespace/name`. When sharding is enabled, the leader election ID is automatically suffixed with the shard index (e.g. `snowplane-leader-election-shard-0`) so that each shard independently elects a leader without conflicts.

Key properties:
- **Zero coordination** — no shared state or external coordination required across shards
- **Deterministic** — the same object always maps to the same shard
- **Uniform** — FNV-1a distributes objects evenly across shards
- **Dynamic rescaling** — changing `--shard-count` rebalances on next reconcile cycle
- **Idempotent** — brief duplicate processing during rollout is harmless
- **Complete coverage** — all controller types (including FieldExport and SQLStatement) are shard-aware; only ProviderConfig remains global since every shard needs provider credentials

Alternatively, use namespace-based sharding for coarser-grained partitioning:

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

## Multi-Cluster Deployments

### Ownership Conflict Limitation

Snowplane detects ownership conflicts (two CRs managing the same Snowflake resource) using a FQN hash label on each CR. This detection works **within a single Kubernetes cluster only**. Two Snowplane installations in different clusters managing the same Snowflake account will not detect conflicts. Both controllers may issue conflicting ALTER or DROP statements, causing alternating drift corrections.

### Recommended Partitioning Strategies

To safely run Snowplane in multiple clusters, partition by one of the following boundaries:

| Strategy | Description | Best For |
|----------|-------------|----------|
| **Account partitioning** | Each cluster manages a different Snowflake account | Multi-account orgs, separate prod/dev |
| **Database partitioning** | Each cluster manages a non-overlapping set of databases via `allowedDatabases` | Shared account, team-per-cluster |
| **Namespace partitioning** | Each cluster manages resources in non-overlapping Kubernetes namespaces, each with a separate ProviderConfig scoped to specific databases | GitOps multi-cluster with Flux/Argo |
| **Read/write split** | One cluster has full management; others use `observeOnly: true` for read-only monitoring | DR/standby, audit dashboards |

Example — database partitioning across two clusters:

```yaml
# Cluster A: manages analytics databases
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  allowedDatabases: ["ANALYTICS_*", "REPORTING"]
  # ...credentials...
```

```yaml
# Cluster B: manages application databases
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  allowedDatabases: ["APP_*", "STAGING"]
  # ...credentials...
```

### Snowflake-Side Safety

For additional protection, use separate Snowflake service users per cluster with role-scoped privileges:

```sql
-- Cluster A service user: can only manage analytics databases
CREATE ROLE SNOWPLANE_CLUSTER_A;
GRANT OWNERSHIP ON DATABASE ANALYTICS_PROD TO ROLE SNOWPLANE_CLUSTER_A;
GRANT OWNERSHIP ON DATABASE REPORTING TO ROLE SNOWPLANE_CLUSTER_A;

-- Cluster B service user: can only manage app databases
CREATE ROLE SNOWPLANE_CLUSTER_B;
GRANT OWNERSHIP ON DATABASE APP_PROD TO ROLE SNOWPLANE_CLUSTER_B;
GRANT OWNERSHIP ON DATABASE STAGING TO ROLE SNOWPLANE_CLUSTER_B;
```

This ensures that even if a cluster partition is misconfigured, the Snowflake role boundary prevents cross-partition mutations.

{: .warning }
> There is no server-side locking mechanism. If two controllers target the same Snowflake object, they will both succeed and fight. Always ensure non-overlapping resource ownership across clusters.

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
> Set `spec.managementPolicies.adoptionPolicy: adopt` on resources if the Snowflake objects already exist to prevent creation errors. Nil spec fields will be late-initialized from the existing Snowflake state during adoption.
