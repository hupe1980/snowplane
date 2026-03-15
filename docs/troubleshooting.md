---
layout: default
title: Troubleshooting
nav_order: 6
description: "Condition reference, common failure scenarios, force-reconcile instructions, and debug info collection."
---

# Troubleshooting
{: .fs-8 }

Diagnose and resolve common issues with Snowplane resources.
{: .fs-5 .fw-300 }

---

## Condition Reference

Every Snowplane resource reports its state through standard Kubernetes conditions. Use `kubectl get <resource> <name> -o yaml` to inspect them.

### Condition Types

| Type | Meaning |
|:-----|:--------|
| `Ready` | Resource is provisioned and synchronized with Snowflake |
| `Synced` | Last reconciliation completed successfully |
| `ReferencesResolved` | All cross-resource references (e.g., Schema→Database) are resolved |
| `DriftDetected` | Observed Snowflake state differs from desired spec |

### Condition Reasons

| Reason | Category | Description |
|:-------|:---------|:------------|
| `Available` | Ready | Resource is healthy and in sync |
| `ReconcileSuccess` | Synced | Last reconciliation succeeded |
| `ReconcileError` | Recoverable | Transient error — controller will retry |
| `RecoverableError` | Recoverable | Non-fatal error with automatic retry |
| `Creating` | Transitional | Snowflake CREATE in progress |
| `Deleting` | Transitional | Snowflake DROP in progress |
| `DriftDetected` | Drift | External changes detected (see [Drift Detection]({% link drift-detection.md %})) |
| `DriftCorrected` | Info | Drift was automatically corrected |
| `DependencyWait` | Waiting | Waiting for a referenced resource |
| `DependencyNotReady` | Waiting | Referenced resource exists but not ready |
| `CredentialsError` | Recoverable | Secret exists but credentials are invalid |
| `SecretNotFound` | Recoverable | Credentials Secret not found |
| `ClientCreationFailed` | Recoverable | Could not create Snowflake client |
| `PingFailed` | Recoverable | Snowflake connectivity check failed |
| `RateLimited` | Recoverable | Request was rate-limited — will retry |
| `TerminalError` | **Terminal** | Non-retryable error — reconciliation stopped |
| `ImmutableField` | **Terminal** | Attempted to change an immutable field |
| `ValidationFailed` | **Terminal** | Spec validation failed |
| `InvalidConfig` | **Terminal** | ProviderConfig is invalid |
| `ResourceAlreadyExists` | **Terminal** | Snowflake resource exists and adoption is not enabled |
| `NamespaceNotAllowed` | **Terminal** | Resource is in a namespace not in watchNamespaces |
| `RoleNotAllowed` | **Terminal** | Requested useRole is not in allowedRoles |
| `DatabaseNotAllowed` | **Terminal** | Target database is not in ProviderConfig `allowedDatabases` |
| `SchemaNotAllowed` | **Terminal** | Target schema is not in ProviderConfig `allowedSchemas` |
| `Adopted` | Info | Resource was adopted from existing Snowflake object |
| `LateInitialized` | Info | Spec fields were late-initialized from observed Snowflake state during adoption |
| `OrphanedResource` | Info | Resource was deleted with orphan policy |
| `ConflictDetected` | **Terminal** | Another CR already manages this Snowflake object — reconciliation will not retry |
| `DeleteBlocked` | Blocking | DROP failed — resource stuck in deleting state |
| `InUse` | Blocking | Resource has dependents preventing deletion |
| `ReconcilePaused` | Info | `spec.paused: true` is set |

{: .warning }
> **Terminal conditions stop reconciliation.** The controller will not retry until you fix the spec and the generation changes.

---

## Common Scenarios

### 1. Resource Stuck in TerminalError

**Symptoms:** `Ready=False`, `reason=TerminalError`, no further reconciliation.

**Diagnosis:**
```bash
kubectl get database my-db -o jsonpath='{.status.conditions[?(@.type=="Ready")].message}'
```

**Common causes:**
- Invalid SQL syntax in resource spec (e.g., invalid warehouse size)
- Snowflake permissions insufficient for the requested operation
- Invalid identifier (reserved keyword, special character)

**Resolution:** Fix the spec field that caused the error. The generation change triggers a new reconciliation:
```bash
kubectl edit database my-db  # Fix the invalid field
```

---

### 2. Resource Exists — Adoption Required

**Symptoms:** `Ready=False`, `reason=ResourceAlreadyExists`.

**Diagnosis:** The Snowflake resource already exists, but the default `adoptionPolicy` is `fail-if-exists`.

**Resolution:** Enable adoption for this resource:
```yaml
spec:
  managementPolicies:
    adoptionPolicy: adopt
```

```bash
kubectl patch database my-db --type merge \
  -p '{"spec":{"managementPolicies":{"adoptionPolicy":"adopt"}}}'
```

---

### 3. Immutable Field Changed

**Symptoms:** `Ready=False`, `reason=ImmutableField`.

**Diagnosis:**
```bash
kubectl get database my-db -o jsonpath='{.status.conditions[?(@.type=="Ready")].message}'
# "immutable field changed: name"
```

**Resolution:** Immutable fields (like `name`, `database`, `transient`) cannot be changed after creation. To change them:
1. Delete the current CR.
2. Create a new CR with the desired values.

Or if you want to keep the Snowflake object:
1. Annotate with `abandon-on-delete` to prevent DROP.
2. Delete the CR.
3. Create a new CR with `adoptionPolicy: adopt`.

---

### 4. ProviderConfig Not Ready

**Symptoms:** `Synced=False`, `reason=ReconcileError`, message mentions ProviderConfig.

**Diagnosis:**
```bash
kubectl get providerconfig <name> -o yaml
```

**Common causes:**
- Credentials Secret doesn't exist or has wrong keys
- Account URL is incorrect
- Snowflake account is unreachable

**Resolution:**
```bash
# Check the Secret exists
kubectl get secret <secret-name> -n <namespace>

# Verify connectivity
kubectl logs deployment/snowplane-controller -n snowplane-system | grep "ping"
```

---

### 5. Circuit Breaker Open

**Symptoms:** All resources for a provider fail with `ErrCircuitOpen`. Metric `snowplane_circuit_breaker_state` is 1 (open).

**Diagnosis:** 5 consecutive Snowflake API failures tripped the circuit breaker.

**Timeline:**
1. **Open** → All calls rejected for 60s (initial backoff).
2. **HalfOpen** → One probe call allowed. Success → Closed; failure → Open with doubled backoff (up to 15 min).

**Resolution:**
- Check Snowflake status page for outages.
- Verify credentials haven't been rotated.
- Check network connectivity from the controller pod.
- The circuit breaker will self-heal once Snowflake is reachable.

**Monitoring:**
```promql
snowplane_circuit_breaker_state > 0
```

---

### 6. Drift Detected but Not Corrected

**Symptoms:** `DriftDetected=True` condition persists, resource not being corrected.

**Common causes:**
- `driftPolicy: detect-only` is set — this is intentional behavior.
- Drift is in immutable fields only — the controller cannot ALTER immutable fields.

**Diagnosis:**
```bash
# Check the drift policy
kubectl get database my-db -o jsonpath='{.spec.managementPolicies.driftPolicy}'

# Check which fields drifted
kubectl get database my-db -o jsonpath='{.status.conditions[?(@.type=="DriftDetected")].message}'
```

**Resolution:**
- For `detect-only` policy: change to `correct` if you want auto-fix.
- For immutable drift: manually fix in Snowflake console, or delete+recreate the resource.

---

### 7. Deletion Stuck — DeleteBlocked

**Symptoms:** Resource has `deletionTimestamp` but won't finalize. Condition: `DeleteBlocked`.

**Diagnosis:** The Snowflake DROP failed with a terminal error (e.g., dependencies exist, insufficient privileges).

**Resolution — escape hatch:**
```bash
# Skip the DROP and just remove the finalizer
kubectl annotate database my-db \
  snowplane.hupe1980.github.io/abandon-on-delete="true" --overwrite
```

The controller will skip the DROP, orphan the Snowflake resource, and remove the finalizer on the next reconciliation.

---

### 8. Credential Rotation

**Symptoms:** Resources start failing with `CredentialsError` or `PingFailed` after rotating Snowflake credentials.

**Resolution:**
1. Update the credentials Secret:
   ```bash
   kubectl create secret generic snowflake-creds \
     --from-file=privateKey=new_rsa_key.p8 \
     --dry-run=client -o yaml | kubectl apply -f -
   ```
2. The ClientFactory detects the config hash change and replaces the cached client automatically.
3. All resources for that provider will reconnect on their next reconciliation cycle.

{: .note }
> The client pool is keyed by **provider name + config hash**. When the hash changes, the old client is closed and a new one is created — no controller restart needed.
>
> Additionally, the ProviderConfig reconciler proactively evicts stale clients from the pool when it encounters errors (`SecretNotFound`, `CredentialsError`, `InvalidConfig`, `PingFailed`). This ensures that a failed connection is never cached and retried indefinitely.

---

### 9. Rate Limiting

**Symptoms:** Reconciliation slows down. Metric `snowplane_rate_limit_waits_total` increasing.

**Diagnosis:** The hierarchical rate limiter is throttling requests:
- **Per-controller**: default 10 QPS / 20 burst
- **Per-account**: default 50 QPS / 100 burst (aggregate across all controllers)

**Resolution:** Increase limits via Helm values:
```yaml
controller:
  rateLimit:
    qps: 20
    burst: 40
    accountQps: 100
    accountBurst: 200
```

---

### 10. Cross-Resource Reference Not Resolving

**Symptoms:** `ReferencesResolved=False`, `reason=RefResolutionFailed` or `DependencyNotReady`.

**Diagnosis:** A Schema depends on a Database (via `databaseRef`), but the referenced resource isn't ready.

```bash
# Check the referenced resource
kubectl get database <ref-name> -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'
```

**Resolution:**
- Ensure the referenced resource exists and is `Ready=True`.
- Check that the `databaseRef` name matches the referenced resource's `metadata.name`.
- If using cross-namespace references, ensure RBAC allows it.

---

### 11. Database or Schema Not Allowed

**Symptoms:** `Ready=False`, `reason=DatabaseNotAllowed` or `reason=SchemaNotAllowed`.

**Diagnosis:** The ProviderConfig restricts which databases or schemas may be targeted via `spec.allowedDatabases` or `spec.allowedSchemas`.

```bash
kubectl get providerconfig <name> -o jsonpath='{.spec.allowedDatabases}'
kubectl get providerconfig <name> -o jsonpath='{.spec.allowedSchemas}'
```

**Resolution:**
- Add the target database/schema to the ProviderConfig's allowed lists.
- Or use a different ProviderConfig that permits the target scope.

---

### 12. Pre-Flight Check — Database or Schema Not Found

**Symptoms:** `Ready=False`, `reason=DependencyNotReady`, message contains "database not found in Snowflake" or "schema not found in Snowflake".

**Diagnosis:** The resource uses a raw `databaseName`/`schemaName` string (not a CR reference), and the specified database or schema does not exist in the target Snowflake account.

```bash
kubectl get <resource> <name> -o jsonpath='{.status.conditions[?(@.type=="Ready")].message}'
```

**Resolution:**
- Create the Snowflake database/schema first (either manually or via a Snowplane Database/Schema CR).
- Or use `databaseRef`/`schemaRef` to reference a Snowplane-managed Database/Schema CR — the controller automatically gates on CR readiness.

---

## Force Reconcile

There is no dedicated "force reconcile" annotation. Instead, any annotation change triggers immediate reconciliation because the controller uses an `AnnotationChangedPredicate` event filter.

**Trigger immediate reconciliation:**
```bash
kubectl annotate database my-db \
  snowplane.hupe1980.github.io/reconcile-trigger="$(date +%s)" --overwrite
```

**When to use:**
- After manual Snowflake console changes you want detected immediately
- After fixing a TerminalError condition
- During debugging (the default requeue interval is 5 minutes)

---

## Prometheus Alerting Rules

Set up these alerts for proactive monitoring:

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
          summary: "High reconciliation error rate for {{ $labels.controller }}"

      - alert: SnowplaneCircuitBreakerOpen
        expr: snowplane_circuit_breaker_state > 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Circuit breaker open for provider {{ $labels.provider }}"

      - alert: SnowflakeAPILatencyHigh
        expr: |
          histogram_quantile(0.99,
            rate(snowplane_snowflake_operation_duration_seconds_bucket[5m])
          ) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "p99 Snowflake API latency exceeds 10s for {{ $labels.controller }}"
```

{: .tip }
> Use `status.lastReconcileTime` to build staleness alerts. If a resource's `lastReconcileTime` is older than 2× the requeue interval (default 5m), the controller may be stuck or paused.

See [Observability]({% link observability.md %}) for the full metrics reference.

---

## 11. Webhook Issues

When the validating admission webhook is enabled (`webhook.enabled: true`), several failure modes can occur:

### Webhook Rejecting All Requests

**Symptom:** All `kubectl apply` operations for Snowplane resources fail with `admission webhook denied the request`.

**Cause:** The webhook may be misconfigured or the controller pod is not receiving webhook traffic.

**Fix:**

```bash
# Check webhook configuration
kubectl get validatingwebhookconfiguration -l app.kubernetes.io/name=snowplane

# Check if the webhook service is reachable
kubectl get endpoints -n snowplane-system -l app.kubernetes.io/name=snowplane

# Check controller logs for webhook errors
kubectl logs deployment/snowplane -n snowplane-system | grep -i webhook
```

### Webhook Unreachable (Timeout)

**Symptom:** `kubectl apply` times out or returns `context deadline exceeded` for Snowplane resources.

**Cause:** NetworkPolicy may be blocking traffic to the webhook port (9443). If you have `networkPolicy.enabled: true`, ensure the chart version includes the webhook ingress rule.

**Fix:** Verify the NetworkPolicy allows the API server to reach port 9443:

```bash
kubectl get networkpolicy -n snowplane-system -o yaml | grep -A5 9443
```

If missing, upgrade the Helm chart or add `webhook.port` to the NetworkPolicy ingress rules.

### cert-manager Certificate Not Ready

**Symptom:** Webhook TLS handshake fails. Events show `certificate not found` or `secret not found`.

**Cause:** cert-manager is not installed, the Issuer failed to create, or the Certificate resource failed.

**Fix:**

```bash
# Check cert-manager is running
kubectl get pods -n cert-manager

# Check certificate status
kubectl get certificate -n snowplane-system
kubectl describe certificate -n snowplane-system

# Check issuer status
kubectl get issuer -n snowplane-system
```

### Stale CA Bundle

**Symptom:** Webhook returns `x509: certificate signed by unknown authority` after cert renewal.

**Cause:** When using cert-manager, the `cert-manager.io/inject-ca-from` annotation should automatically update the CA bundle. If cert-manager's CA injector is not working, the VWC's `caBundle` becomes stale.

**Fix:**

```bash
# Restart the cert-manager cainjector
kubectl rollout restart deployment cert-manager-cainjector -n cert-manager
```

### Disabling the Webhook in Emergency

If the webhook is causing cluster-wide issues, disable it without a Helm upgrade:

```bash
# Option 1: Delete the VWC directly
kubectl delete validatingwebhookconfiguration <release>-snowplane-ownership

# Option 2: Set failurePolicy to Ignore (if currently Fail)
kubectl patch validatingwebhookconfiguration <release>-snowplane-ownership \
  --type='json' -p='[{"op": "replace", "path": "/webhooks/0/failurePolicy", "value": "Ignore"}]'
```

---

## Collecting Debug Information

When filing a bug report or requesting support, collect these diagnostics:

```bash
# 1. Resource status and events
kubectl describe database my-db

# 2. Controller logs (last 200 lines)
kubectl logs deployment/snowplane-controller -n snowplane-system --tail=200

# 3. All resource conditions
kubectl get databases -o custom-columns=\
  NAME:.metadata.name,\
  READY:.status.conditions[0].status,\
  REASON:.status.conditions[0].reason,\
  MESSAGE:.status.conditions[0].message

# 4. Check last reconcile time (staleness)
kubectl get databases -o custom-columns=\
  NAME:.metadata.name,\
  LAST-RECONCILE:.status.lastReconcileTime

# 5. CRD version
kubectl get crd databases.snowplane.hupe1980.github.io -o jsonpath='{.metadata.resourceVersion}'

# 6. Controller version
kubectl get deployment snowplane-controller -n snowplane-system \
  -o jsonpath='{.spec.template.spec.containers[0].image}'

# 7. ProviderConfig status
kubectl get providerconfig -o yaml

# 8. Metrics snapshot (if port-forwarded)
curl -s localhost:8080/metrics | grep snowplane_
```

{: .note }
> For structured JSON logs, set `controller.logFormat: json` in your Helm values. This makes log aggregation and searching significantly easier in production.
