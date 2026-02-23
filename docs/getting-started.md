# Getting Started

This guide walks you through setting up Snowplane and creating your first managed Snowflake resources.

## Step 1: Prerequisites

Ensure you have:

- A running Kubernetes cluster (kind, minikube, or remote — see [Development Guide](development.md) for local setup)
- Snowplane CRDs installed
- The Snowplane controller running
- A Snowflake account with the SYSADMIN role (or equivalent)

## Step 2: Create Credentials

Choose an authentication method and create the credentials Secret:

```bash
# Option A: Key pair (recommended for production)
kubectl create secret generic snowflake-creds \
  --from-file=privateKey=/path/to/your/rsa_key.p8 \
  -n default

# Option B: Username/password (simpler for development)
kubectl create secret generic snowflake-creds \
  --from-literal=password='YourPassword123!' \
  -n default

# Option C: External OAuth token (for Secret-based OAuth)
kubectl create secret generic snowflake-creds \
  --from-literal=token='<your-oauth-access-token>' \
  -n default
```

### Workload Identity Federation (WIF)

For Workload Identity Federation, no Secret is needed. Instead, the controller reads an OAuth token from a projected service account token volume. Configure your Pod to project a token:

```yaml
volumes:
  - name: snowflake-token
    projected:
      sources:
        - serviceAccountToken:
            audience: "https://your-snowflake-account.snowflakecomputing.com"
            expirationSeconds: 3600
            path: token
```

Then reference the token file path in the ProviderConfig (see below).

## Step 3: Create a ProviderConfig

The ProviderConfig tells Snowplane how to connect to your Snowflake account:

### Key Pair Authentication

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  account: "xy12345"           # Your Snowflake account identifier
  region: "us-east-1"          # Your Snowflake region (optional for orgname-accountname identifiers)
  user: "SNOWPLANE_USER"      # Snowflake username
  role: "SYSADMIN"             # Role with resource management permissions
  warehouse: "COMPUTE_WH"     # Default warehouse for DDL operations
  authenticationType: KeyPair
  credentials:
    secretRef:
      name: snowflake-creds
      namespace: default
      key: privateKey
```

### Username/Password Authentication

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  account: "xy12345"
  region: "us-east-1"
  user: "SNOWPLANE_USER"
  role: "SYSADMIN"
  warehouse: "COMPUTE_WH"
  authenticationType: UsernamePassword
  credentials:
    secretRef:
      name: snowflake-creds
      namespace: default
      key: password
```

### Workload Identity Federation (WIF)

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  account: "xy12345"
  region: "us-east-1"
  user: "SNOWPLANE_USER"
  role: "SYSADMIN"
  warehouse: "COMPUTE_WH"
  authenticationType: WorkloadIdentity
  workloadIdentity:
    audience: "https://xy12345.us-east-1.snowflakecomputing.com"
    # tokenFilePath defaults to /var/run/secrets/snowflake/token
    # provider defaults to OIDC (also supports AWS, GCP, Azure)
```

Apply it:

```bash
kubectl apply -f providerconfig.yaml
```

## Step 4: Verify Connectivity

Check the ProviderConfig status:

```bash
kubectl get pc default -o yaml
```

Look for these conditions in the status:

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      reason: Available
      message: "Snowflake connection verified"
    - type: Synced
      status: "True"
      reason: ReconcileSuccess
```

If `Ready` is `False`, check the `message` field for details on what went wrong.

## Troubleshooting

### ProviderConfig shows Ready=False

**SecretNotFound**: The referenced Secret doesn't exist or is in a different namespace.

```bash
kubectl get secret snowflake-creds -n default
```

**InvalidConfig**: The Secret is missing the expected key or the authentication type is misconfigured.

```bash
kubectl describe pc default
```

**PingFailed**: The credentials are valid but the connection to Snowflake failed. Check:
- Account identifier is correct
- Region is correct
- Network connectivity (firewall rules, VPN)
- User is not locked

**ClientCreationFailed**: The Snowflake client couldn't be initialized. For key pair auth, ensure:
- The private key is valid PEM (PKCS8 or PKCS1)
- The key is not passphrase-protected (or provide the passphrase)

### Controller Logs

```bash
kubectl logs -n snowplane-system deployment/snowplane-controller-manager -f
```

### Events

```bash
kubectl get events --field-selector involvedObject.name=default
```

## Step 5: Create a Database

Once your ProviderConfig is `Ready`, you can create Snowflake resources. Here's a simple Database:

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: my-database
spec:
  name: MY_DATABASE
  providerRef:
    name: default
```

Apply it:

```bash
kubectl apply -f database.yaml
```

Check the status:

```bash
kubectl get databases
# or using the short name
kubectl get db
```

A healthy Database shows `Ready=True` and `Synced=True`:

```
NAME          DATABASE      READY   SYNCED   OWNER      AGE
my-database   MY_DATABASE   True    True     SYSADMIN   2m
```

### Database with Full Options

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: analytics-database
spec:
  name: ANALYTICS_DB
  comment: "Analytics data warehouse managed by Snowplane"
  dataRetentionTimeInDays: 14
  maxDataExtensionTimeInDays: 28
  replaceInvalidCharacters: true
  defaultDdlCollation: "en-ci"
  storageSerializationPolicy: OPTIMIZED
  logLevel: INFO
  metricLevel: ALL
  traceLevel: ON_EVENT
  deletionPolicy: Delete
  providerRef:
    name: default
```

### Transient Database

Transient databases have no Fail-safe period, reducing storage costs:

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: staging-database
spec:
  name: STAGING_DB
  transient: true
  dataRetentionTimeInDays: 1
  providerRef:
    name: default
```

### Orphan Deletion Policy

To keep the Snowflake database when the CR is deleted, set `deletionPolicy: Orphan`:

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: production-database
spec:
  name: PRODUCTION_DB
  deletionPolicy: Orphan
  providerRef:
    name: default
```

### Nil-Means-Unmanaged Semantics

Optional pointer fields (`*string`, `*int32`, `*bool`) follow the **nil-means-unmanaged** convention:

- **nil (omitted):** The controller does not include the parameter in `CREATE` or `ALTER` statements. Snowflake's server-side default is preserved.
- **non-nil (set to a value):** The controller manages the parameter declaratively and will correct any drift back to the specified value.

For example, omitting `dataRetentionTimeInDays` leaves Snowflake's default (typically 1 day for Standard, 0 for Transient). Setting it to `14` puts it under Snowplane management.

#### Reverting to Snowflake Defaults (UNSET)

If you previously set an optional field and later remove it from the spec, the controller automatically issues an `ALTER ... UNSET` statement to revert the parameter to Snowflake's server-side default. This is tracked via `status.trackedParameters`.

For example:

```yaml
# Step 1: Set dataRetentionTimeInDays to 14
spec:
  name: MY_DATABASE
  dataRetentionTimeInDays: 14
  providerRef:
    name: default

# Step 2: Remove dataRetentionTimeInDays to revert to Snowflake default
spec:
  name: MY_DATABASE
  providerRef:
    name: default
# → Controller issues: ALTER DATABASE "MY_DATABASE" UNSET DATA_RETENTION_TIME_IN_DAYS
```

### Conditions & Troubleshooting

Each resource reports its state through Kubernetes conditions:

| Condition | Meaning |
|-----------|---------|
| `Ready` | Resource exists in Snowflake and is in the desired state |
| `Synced` | Last reconciliation succeeded |
| `Terminal` | Non-retryable error (e.g., immutable field violation) — requires user intervention |
| `Recoverable` | Transient error (e.g., Snowflake timeout, rate limit) — will be retried automatically |
| `ReferencesResolved` | All cross-resource references (e.g., `databaseRef`) are resolved |
| `CredentialsInvalid` | ProviderConfig credentials are invalid or expired |

When `Recoverable=True`, the controller is automatically retrying with exponential backoff. No user action is needed unless the condition persists.

## Operational Features

### Admission Webhooks

Snowplane includes **mutating** and **validating** admission webhooks that intercept CREATE and UPDATE requests before the reconciler runs.

#### Mutating Webhook (Defaults Injection)

The mutating webhook injects sensible defaults at admission time:

- `spec.deletionPolicy` defaults to `Delete` if unset
- `spec.providerRef.name` defaults to `"default"` if unset
- `spec.type` defaults to `PERSON` for User resources if unset

This means you can submit minimal CRs and still get correct behavior.

#### Validating Webhook

The validating webhook prevents invalid mutations with immediate API errors:

- **Spec validation**: Required fields, enum values, range bounds (e.g., `dataRetentionTimeInDays` must be 0–90)
- **Immutable field enforcement**: `spec.name`, `spec.transient`, `spec.warehouseType`, `spec.type`, `spec.databaseRef` are rejected on UPDATE
- **Multi-error aggregation**: All violations are returned at once via `errors.Join`, not just the first

Enable webhooks with the `--enable-webhooks` flag:

```bash
# In your controller Deployment
args:
  - --enable-webhooks
  - --webhook-port=9443   # default
```

Deploy the webhook configuration from `config/webhook/`:

```bash
kubectl apply -f config/webhook/service.yaml
kubectl apply -f config/webhook/mutating-webhook-configuration.yaml
kubectl apply -f config/webhook/validating-webhook-configuration.yaml
```

#### ForceNew — Opt-in Delete+Recreate

When an immutable field change is required, annotate the resource with `snowplane.hupe1980.github.io/force-new: "true"` to bypass the immutable-field rejection:

```yaml
metadata:
  annotations:
    snowplane.hupe1980.github.io/force-new: "true"
spec:
  name: NEW_NAME  # normally rejected as immutable
```

The webhook allows the change through; the reconciler detects the `force-new` annotation and bypasses its own immutable-field check, proceeding with delete+recreate. This works at both the webhook *and* reconciler level as defense-in-depth — even if webhooks are disabled, the annotation is honored. Without the annotation, the change is rejected with a clear error message at both layers.

> **Defense-in-Depth:** Immutable fields are enforced at three layers:
> 1. **CRD schema** — CEL validation rules (`x-kubernetes-validations: self == oldSelf`) reject changes at the API server level, even without webhooks installed. Requires Kubernetes 1.25+.
> 2. **Validating webhook** — Checks `ObservedGeneration > 0` to ensure the resource was reconciled before enforcing immutability. Supports the `force-new` bypass annotation.
> 3. **Reconciler** — Detects immutable field changes as a terminal error if the webhook layer is bypassed.

#### Webhook Availability & failurePolicy

By default, webhook configurations use `failurePolicy: Fail`, meaning **all CRD create/update operations are blocked** if the webhook pod is unavailable (e.g., during rollouts, node drains, or OOM kills). This is the safest setting but has availability implications:

| Scenario | `failurePolicy: Fail` | `failurePolicy: Ignore` |
|----------|----------------------|------------------------|
| Webhook pod is down | CRD mutations **blocked** | CRD mutations **allowed** (skipping validation) |
| Invalid spec submitted | Rejected immediately | Accepted — caught by reconciler later |
| Single replica (`replicaCount: 1`) | Risky during rollouts | Safe, but validation gaps possible |

**Production recommendation:** When webhooks are enabled with `failurePolicy: Fail`, set `replicaCount: 2` (or higher) to ensure at least one webhook pod is always available:

```yaml
# values.yaml
replicaCount: 2
webhooks:
  enabled: true
  failurePolicy: Fail  # default — blocks on unavailability
```

For non-production environments or clusters where brief validation gaps are acceptable:

```yaml
webhooks:
  enabled: true
  failurePolicy: Ignore  # allows mutations through during downtime
```

### Rate Limiting

The controller applies per-provider token-bucket rate limiting to all Snowflake API calls, protecting against excessive API usage:

```bash
args:
  - --rate-limit-qps=10     # Sustained requests per second (default: 10)
  - --rate-limit-burst=20   # Maximum burst size (default: 20)
```

When the rate limit is exceeded, reconcilers pause and retry with backoff. A `Recoverable=True` condition with `RateLimited` reason is set so monitoring can track throttling.

### Prometheus Metrics

The controller exposes custom Prometheus metrics on the `/metrics` endpoint:

| Metric | Type | Description |
|--------|------|-------------|
| `snowplane_reconcile_total` | Counter | Total reconciliations by resource type and result |
| `snowplane_reconcile_duration_seconds` | Histogram | Reconciliation duration by resource type |
| `snowplane_snowflake_operation_total` | Counter | Snowflake API calls by operation and result |
| `snowplane_snowflake_operation_duration_seconds` | Histogram | Snowflake API call duration by operation |
| `snowplane_managed_resources` | Gauge | Number of managed resources by type and status |
| `snowplane_client_pool_size` | Gauge | Number of cached Snowflake clients |
| `snowplane_rate_limit_waits_total` | Counter | Rate limiter wait events |
| `snowplane_adoption_total` | Counter | Resource adoption outcomes |
| `snowplane_drift_detected_total` | Counter | Drift detection events per controller |
| `snowplane_circuit_breaker_trips_total` | Counter | Circuit breaker trips per provider |
| `snowplane_circuit_breaker_state` | Gauge | Current circuit breaker state per provider |

Integrate with Prometheus using a `ServiceMonitor` or annotation-based scraping.

## What's Next

Snowplane now supports **22 managed resource types**. Beyond the resources covered in this guide, you can also manage:

- **Task** — Scheduled SQL tasks with DAG scheduling and serverless execution
- **Stream** — Change data capture on tables, views, external tables, stages, and dynamic tables
- **Tag** — Data governance tags with allowed value constraints
- **NetworkPolicy** — IP allow/block lists and network rule enforcement
- **ResourceMonitor** — Credit quota monitoring with suspend/notify triggers
- **MaskingPolicy** — Dynamic data masking for PII/PCI compliance
- **RowAccessPolicy** — Row-level security for multi-tenant access control
- **GrantOwnership** — Ownership transfer between roles

See the `config/samples/` directory for example CRs of every resource type.

## Step 6: Create a Schema

Schemas belong to a Database. The Schema controller resolves the `databaseRef` before creating the Snowflake schema, waiting for the parent Database to be Ready.

### Basic Schema

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Schema
metadata:
  name: my-schema
spec:
  name: PUBLIC
  databaseRef:
    name: my-database   # Must match a Database CR's metadata.name in the same namespace
  providerRef:
    name: default
```

Apply it:

```bash
kubectl apply -f schema.yaml
```

Check the status:

```bash
kubectl get schemas
```

A healthy Schema shows `Ready=True`, `Synced=True`, and `Refs=True`:

```
NAME        SCHEMA   DATABASE      READY   SYNCED   REFS   OWNER      AGE
my-schema   PUBLIC   MY_DATABASE   True    True     True   SYSADMIN   1m
```

### Schema with Full Options

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Schema
metadata:
  name: staging-schema
spec:
  name: STAGING
  databaseRef:
    name: analytics-database
  transient: true
  managedAccess: true
  comment: "Ephemeral staging area with managed access"
  dataRetentionTimeInDays: 1
  defaultDdlCollation: "en-ci"
  storageSerializationPolicy: OPTIMIZED
  logLevel: INFO
  metricLevel: ALL
  traceLevel: ON_EVENT
  providerRef:
    name: default
```

### Database + Schema Together

You can apply both the Database and Schema in a single file. The Schema controller automatically waits for the Database to become Ready:

```yaml
# database_with_schemas.yaml
---
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: analytics-db
spec:
  name: ANALYTICS
  comment: "Analytics database"
  providerRef:
    name: default
---
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Schema
metadata:
  name: raw-schema
spec:
  name: RAW
  databaseRef:
    name: analytics-db
  comment: "Raw data ingestion"
  providerRef:
    name: default
```

```bash
kubectl apply -f database_with_schemas.yaml
```

The order in the file does not matter — the Schema controller will requeue until its Database is Ready.

### Dependency Resolution

The Schema controller uses a `ReferenceResolver` to resolve `databaseRef`:

1. **Database not found** — Schema sets `ReferencesResolved=False` with reason `DependencyNotReady` and returns an error; controller-runtime applies exponential backoff before the next reconcile
2. **Database not Ready** — Same behavior; Schema waits for the Database to become Ready
3. **Database Ready** — Schema resolves the `fullyQualifiedName`, creates the schema in Snowflake, and sets `ReferencesResolved=True`

The Schema controller watches Database CRs. When a Database transitions to Ready, all Schemas referencing it are automatically requeued — no polling delay.

### Cascading Deletion

You can safely delete a Database CR even if dependent Schema CRs still exist. The Schema controller handles this gracefully:

- The Schema reconciler uses the cached `status.databaseName` to issue `DROP SCHEMA` when the referenced Database CR is already gone.
- If the ProviderConfig is also deleted, the Schema (and Database) controllers remove their finalizers to allow Kubernetes garbage collection, since the Snowflake connection is no longer available.

This means `kubectl delete database my-database` will not leave Schemas stuck in `Terminating` state.

> **Note:** ProviderConfig itself is protected by an in-use finalizer. It cannot be deleted while any managed resource (Database, Schema, Warehouse, User, AccountRole, DatabaseRole, AccountRoleGrant, DatabaseRoleGrant, ShareGrant, Table, View, Stage, Task, Stream, Tag, NetworkPolicy, ResourceMonitor, MaskingPolicy, RowAccessPolicy, or GrantOwnership) still references it. The controller emits an `InUse` warning event and requeues until all references are removed.

### Immutable Fields

The following Schema fields are immutable after creation:

- `spec.name` — Cannot be renamed
- `spec.databaseRef` — Cannot move a schema to a different database
- `spec.transient` — Cannot change transient status after creation

Attempting to modify an immutable field results in a `Terminal` condition.

## Step 7: Create a Warehouse

Create a Warehouse CR to manage a Snowflake virtual warehouse:

### Basic Warehouse

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Warehouse
metadata:
  name: analytics-wh
spec:
  providerRef:
    name: default
  name: ANALYTICS_WH
  warehouseSize: XSMALL
  autoSuspend: 300
  autoResume: true
```

```bash
kubectl apply -f warehouse.yaml
```

### Full-Featured Warehouse

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Warehouse
metadata:
  name: etl-wh
spec:
  providerRef:
    name: default
  name: ETL_WH
  warehouseType: STANDARD
  warehouseSize: LARGE
  minClusterCount: 1
  maxClusterCount: 3
  scalingPolicy: ECONOMY
  autoSuspend: 600
  autoResume: true
  comment: "ETL processing warehouse with auto-scaling"
  enableQueryAcceleration: true
  queryAccelerationMaxScaleFactor: 8
  maxConcurrencyLevel: 12
  statementQueuedTimeoutInSeconds: 300
  statementTimeoutInSeconds: 7200
```

### Detect-Only Drift Policy

To monitor drift without correcting it, add the detect-only annotation:

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Warehouse
metadata:
  name: monitored-wh
  annotations:
    snowplane.hupe1980.github.io/drift-policy: detect-only
spec:
  providerRef:
    name: default
  name: MONITORED_WH
  warehouseSize: MEDIUM
  autoSuspend: 300
  autoResume: true
```

With detect-only, drift is reported via the `DriftDetected` condition and events but not corrected. See [Drift Detection](drift-detection.md) for details.

### Verify Warehouse

```bash
kubectl get warehouses
# or shortname:
kubectl get wh
```

### Immutable Fields

The following Warehouse fields are immutable after creation:

- `spec.name` — Cannot be renamed
- `spec.warehouseType` — Cannot change warehouse type after creation

Attempting to modify an immutable field results in a `Terminal` condition.

## Step 8: Create an Account Role

Create an AccountRole CR to manage a Snowflake account-level role:

### Basic Account Role

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: AccountRole
metadata:
  name: data-reader
spec:
  providerRef:
    name: default
  name: DATA_READER
```

```bash
kubectl apply -f accountrole.yaml
```

### Account Role with Comment and Owner

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: AccountRole
metadata:
  name: data-admin
spec:
  providerRef:
    name: default
  name: DATA_ADMIN
  comment: "Admin role for data engineering team"
  useRole: USERADMIN
```

When `useRole` is set, the controller executes `USE ROLE` before creating the role in Snowflake, ensuring the role is owned by the specified parent role.

### Verify Account Role

```bash
kubectl get accountroles
# or shortname:
kubectl get ar
```

A healthy AccountRole shows `Ready=True` and `Synced=True`:

```
NAME          ROLE          READY   SYNCED   OWNER       AGE
data-reader   DATA_READER   True    True     SYSADMIN    1m
data-admin    DATA_ADMIN    True    True     USERADMIN   1m
```

### Immutable Fields

The following AccountRole fields are immutable after creation:

- `spec.name` — Cannot be renamed

Attempting to modify an immutable field results in a `Terminal` condition.

## Use Role (USE ROLE)

All resource types (Database, Schema, Warehouse, AccountRole, User) support an optional `spec.useRole` field. When set, the reconciler switches to the specified Snowflake role before performing CREATE, ALTER, or DROP operations. In Snowflake's DAC model, the role active at CREATE time becomes the owner of the object.

> **Important:** `spec.useRole` is **immutable after creation**. Once a resource is created, the useRole cannot be changed. Changing the role used for `USE ROLE` without transferring ownership would leave ALTER/DROP operations unable to modify the object (Snowflake's DAC model requires the owning role to modify an object).

### How It Works

1. The controller opens a dedicated database connection (pinned to a single `sql.Conn`)
2. Executes `USE ROLE "<useRole>"` on that connection
3. Performs all Snowflake operations (CREATE/ALTER/DROP) using the switched role
4. Restores the original role after the operation completes

### Example: Database with Use Role

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: analytics-db
spec:
  name: ANALYTICS_DB
  comment: "Analytics database created under DATA_ADMIN"
  useRole: DATA_ADMIN
  providerRef:
    name: default
```

The database will be created using `USE ROLE "DATA_ADMIN"`, so `DATA_ADMIN` becomes the owner in Snowflake.

## Step 9: Create Users

Snowplane manages Snowflake users with full lifecycle support, including secret-referenced passwords and RSA keys.

### Create User Credentials Secret

```bash
kubectl create secret generic user-credentials \
  --from-literal=password='SecureP@ssw0rd!' \
  -n default
```

### Basic User

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: User
metadata:
  name: my-user
  namespace: default
spec:
  providerRef:
    name: default
  name: MY_USER
```

### Full User with All Fields

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: User
metadata:
  name: data-analyst
  namespace: default
spec:
  name: DATA_ANALYST
  type: PERSON
  loginName: data_analyst
  displayName: "Data Analyst"
  email: analyst@example.com
  firstName: Data
  lastName: Analyst
  comment: "Analyst user for data team"
  defaultRole: ANALYST_ROLE
  defaultSecondaryRoles: ALL
  defaultWarehouse: ANALYTICS_WH
  defaultNamespace: ANALYTICS_DB.PUBLIC
  mustChangePassword: true
  disabled: false
  password:
    name: user-credentials
    key: password
  useRole: SECURITYADMIN
  deletionPolicy: Delete
  providerRef:
    name: default
```

### Service User with RSA Key

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: User
metadata:
  name: etl-service
  namespace: default
spec:
  name: ETL_SERVICE_USER
  type: SERVICE
  defaultRole: ETL_ROLE
  defaultWarehouse: ETL_WH
  rsaPublicKey:
    name: etl-service-keys
    key: rsa-public-key
  deletionPolicy: Delete
  providerRef:
    name: default
```

### Verify Users

```bash
kubectl get users
# or shortname:
kubectl get sfuser
```

A healthy User shows `Ready=True` and `Synced=True`:

```
NAME           USER              TYPE      READY   SYNCED   OWNER          AGE
my-user        MY_USER                     True    True     SYSADMIN       1m
data-analyst   DATA_ANALYST      PERSON    True    True     SECURITYADMIN  1m
etl-service    ETL_SERVICE_USER  SERVICE   True    True     SYSADMIN       1m
```

### Immutable Fields

The following User fields are immutable after creation:

- `spec.name` — Cannot be renamed
- `spec.type` — User type cannot be changed

Attempting to modify an immutable field results in a `Terminal` condition.

### Managed Fields & UNSET

The User controller tracks which optional fields have been actively SET in Snowflake via `status.trackedParameters`. When you remove a previously-set field from the spec (e.g., delete the `comment` field), the controller automatically issues `ALTER USER ... UNSET COMMENT` to revert to Snowflake's server-side default.

## Step 10: Grant Privileges

Grants let you declaratively manage Snowflake privilege grants. Snowplane provides three grant CRDs, one for each grantee type:

- **AccountRoleGrant** — grants privileges to account roles
- **DatabaseRoleGrant** — grants privileges to database roles
- **ShareGrant** — grants privileges to shares

### AccountRoleGrant — Basic

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: AccountRoleGrant
metadata:
  name: usage-on-analytics-db
  namespace: default
spec:
  privilege: USAGE
  on:
    accountObject:
      objectType: DATABASE
      objectName: ANALYTICS_DB
  accountRole: DATA_READER
  providerRef:
    name: default
```

Apply and verify:

```bash
kubectl apply -f accountrolegrant.yaml
kubectl get accountrolegrants
```

### AccountRoleGrant with WITH GRANT OPTION

> **Security Warning:** `withGrantOption: true` enables the grantee to re-grant this privilege to other roles, creating delegation chains. **Revoking the original Grant CR does NOT cascade-revoke re-grants made by the grantee.** Combined with `deletionPolicy: Orphan`, grant privileges persist indefinitely after CR deletion. Use only for trusted roles that require delegation capability.

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: AccountRoleGrant
metadata:
  name: select-on-table
  namespace: default
spec:
  privilege: SELECT
  on:
    schemaObject:
      objectType: TABLE
      objectName: '"MY_DB"."PUBLIC"."MY_TABLE"'
  accountRole: ANALYST
  withGrantOption: true
  providerRef:
    name: default
```

### DatabaseRoleGrant

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: DatabaseRoleGrant
metadata:
  name: usage-to-dbrole
  namespace: default
spec:
  privilege: USAGE
  on:
    accountObject:
      objectType: DATABASE
      objectName: MY_DB
  databaseRole: '"MY_DB"."DATA_ANALYST"'
  providerRef:
    name: default
```

### ShareGrant

Share grants have a flat spec with no `on` hierarchy and do not support `withGrantOption`:

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: ShareGrant
metadata:
  name: usage-to-share
  namespace: default
spec:
  privilege: USAGE
  objectType: DATABASE
  objectName: MY_DB
  share: MY_SHARE
  providerRef:
    name: default
```

### Immutable Fields

**All** grant spec fields are immutable after creation (enforced via CEL validation rules). To change any field, delete and recreate the grant CR (or use the `force-new` annotation).

For AccountRoleGrant and DatabaseRoleGrant:
- `spec.privilege`, `spec.on`, `spec.accountRole`/`spec.databaseRole`, `spec.withGrantOption`

For ShareGrant:
- `spec.privilege`, `spec.objectType`, `spec.objectName`, `spec.share`

### Deletion Behavior

With `deletionPolicy: Delete` (default), deleting the grant CR issues `REVOKE <privilege> ...` in Snowflake. With `deletionPolicy: Orphan`, the grant is left in place.

## Step 11: Export Fields to ConfigMaps or Secrets

FieldExport lets you copy values from any managed resource's status into a ConfigMap or Secret, enabling cross-resource data passing without hard-coding values.

### Export Database Name to a ConfigMap

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: FieldExport
metadata:
  name: db-name-export
spec:
  from:
    resource:
      kind: Database
      name: my-database
    path: ".status.showOutput.name"
  to:
    kind: ConfigMap
    name: snowplane-exports
    key: database-name
```

Apply and verify:

```bash
kubectl apply -f fieldexport.yaml
kubectl get fieldexports
# or using the short name
kubectl get fexp
```

A healthy FieldExport shows `Ready=True`:

```
NAME              SOURCE KIND   SOURCE NAME   TARGET KIND   TARGET NAME        READY   AGE
db-name-export    Database      my-database   ConfigMap     snowplane-exports  True    1m
```

Verify the ConfigMap was created:

```bash
kubectl get configmap snowplane-exports -o yaml
```

### Export to a Secret

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: FieldExport
metadata:
  name: user-login-export
spec:
  from:
    resource:
      kind: User
      name: my-user
    path: ".status.showOutput.loginName"
  to:
    kind: Secret
    name: snowflake-user-info
    key: login-name
```

### Using Exported Values in Applications

Mount the exported ConfigMap or Secret as environment variables in your application:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: data-pipeline
spec:
  template:
    spec:
      containers:
        - name: pipeline
          envFrom:
            - configMapRef:
                name: snowplane-exports
```

### How It Works

1. The FieldExport reconciler fetches the source resource using an unstructured client
2. Checks that the source has `Ready=True` condition (requeues with `DependencyNotReady` if not)
3. Extracts the field value using dot-notation path (e.g., `.status.showOutput.name`)
4. Writes the value to the target ConfigMap or Secret (creates it if missing)
5. Tracks the exported value by SHA-256 hash to avoid unnecessary writes
6. A finalizer ensures the exported key is removed from the target when the FieldExport CR is deleted (if the ConfigMap/Secret was created by fieldexport and becomes empty, it is deleted entirely)

## Step 12: CREATE OR ALTER (Optional)

For Database and Warehouse resources, you can opt into Snowflake's atomic `CREATE OR ALTER` statement instead of the default two-step `CREATE IF NOT EXISTS` + `ALTER` flow:

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: my-database
  annotations:
    snowplane.hupe1980.github.io/use-create-or-alter: "true"
spec:
  name: MY_DATABASE
  comment: "Managed with CREATE OR ALTER"
  providerRef:
    name: default
```

This makes create and update operations atomic. The annotation is supported on Database and Warehouse resources. On unsupported resource types, the annotation is ignored and the standard two-step flow is used.

> **Note:** `CREATE OR ALTER` is a Snowflake preview feature. Check your Snowflake account's feature availability before using it.
