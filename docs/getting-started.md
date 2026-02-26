---
layout: default
title: Getting Started
nav_order: 2
description: "Step-by-step guide to set up Snowplane and create your first managed Snowflake resources."
---

# Getting Started
{: .fs-8 }

This guide walks you through setting up Snowplane and creating your first managed Snowflake resources.
{: .fs-5 .fw-300 }

---

## Prerequisites

- A running Kubernetes cluster (kind, minikube, or remote — see [Development Guide]({% link development.md %}) for local setup)
- Snowplane CRDs installed
- The Snowplane controller running
- A Snowflake account with the SYSADMIN role (or equivalent)

---

## Create Credentials

Choose an authentication method and create the credentials Secret:

```bash
# Option A: Key pair (recommended for production)
kubectl create secret generic snowflake-creds \
  --from-file=privateKey=/path/to/your/rsa_key.p8 \
  -n default

# Option A2: Key pair with encrypted private key (passphrase-protected)
kubectl create secret generic snowflake-creds \
  --from-file=privateKey=/path/to/your/encrypted_rsa_key.p8 \
  --from-literal=passphrase='YourPassphrase' \
  -n default

# Option B: Username/password (simpler for development)
echo -n 'YourPassword123!' > /tmp/sf-password.txt
kubectl create secret generic snowflake-creds \
  --from-file=password=/tmp/sf-password.txt \
  -n default
rm -f /tmp/sf-password.txt

# Option C: External OAuth token
echo -n '<your-oauth-access-token>' > /tmp/sf-token.txt
kubectl create secret generic snowflake-creds \
  --from-file=token=/tmp/sf-token.txt \
  -n default
rm -f /tmp/sf-token.txt
```

{: .warning }
> Always use `--from-file` instead of `--from-literal` for credentials. `--from-literal` values are visible in process listings (`/proc/*/cmdline`) and shell history.

### Workload Identity Federation (WIF)

For WIF, no Secret is needed. The controller reads an OAuth token from a projected service account token volume:

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

See the [Workload Identity Guide]({% link workload-identity.md %}) for cloud-specific setup.

---

## Create a ProviderConfig

The ProviderConfig tells Snowplane how to connect to your Snowflake account.

### Key Pair Authentication
{: .text-delta }

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
  authenticationType: KeyPair
  credentials:
    secretRef:
      name: snowflake-creds
      namespace: default
      key: privateKey
    # Optional: for encrypted PKCS#8 private keys, specify the key
    # within the same Secret that holds the passphrase.
    # passphraseKey: passphrase
```

### Username/Password Authentication
{: .text-delta }

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
{: .text-delta }

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
```

```bash
kubectl apply -f providerconfig.yaml
```

---

## Verify Connectivity

```bash
kubectl get pc default -o yaml
```

Look for these conditions:

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

### Troubleshooting
{: .text-delta }

| Reason | Description | Fix |
|:-------|:------------|:----|
| **SecretNotFound** | Referenced Secret doesn't exist | `kubectl get secret snowflake-creds -n default` |
| **InvalidConfig** | Secret is missing the expected key | `kubectl describe pc default` |
| **PingFailed** | Connection to Snowflake failed | Check account, region, network connectivity |
| **ClientCreationFailed** | Snowflake client init failure | Ensure valid PEM key (PKCS8 or PKCS1) |

```bash
# Controller logs
kubectl logs -n snowplane-system deployment/snowplane-controller-manager -f

# Events
kubectl get events --field-selector involvedObject.name=default
```

---

## Create a Database

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

```bash
kubectl apply -f database.yaml
kubectl get databases  # or: kubectl get db
```

A healthy Database:

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

To keep the Snowflake database when the CR is deleted:

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

---

## Nil-Means-Unmanaged Semantics

Optional pointer fields (`*string`, `*int32`, `*bool`) follow the **nil-means-unmanaged** convention:

- **nil (omitted):** Snowflake's server-side default is preserved
- **non-nil (set):** The controller manages it declaratively and corrects any drift

#### Reverting to Snowflake Defaults (UNSET)

Remove a previously-set field from the spec to trigger `ALTER ... UNSET`:

```yaml
# Step 1: Set retention
spec:
  name: MY_DATABASE
  dataRetentionTimeInDays: 14

# Step 2: Remove it — controller issues ALTER ... UNSET
spec:
  name: MY_DATABASE
```

---

## Conditions

Each resource reports its state through Kubernetes conditions:

| Condition | Meaning |
|:----------|:--------|
| `Ready` | Resource exists in Snowflake and is in the desired state |
| `Synced` | Last reconciliation succeeded |
| `Terminal` | Non-retryable error — requires user intervention |
| `Recoverable` | Transient error — will be retried automatically |
| `ReferencesResolved` | All cross-resource references are resolved |
| `CredentialsInvalid` | ProviderConfig credentials are invalid or expired |

---

## CRD-Level Validation (CEL Rules)

Snowplane uses **CRD-embedded CEL validation rules** at the API server level — no webhooks or cert-manager required.

**Schema Defaults:** `deletionPolicy` → `Delete`, `providerRef.name` → `"default"`, User `type` → `PERSON`.

**Immutable fields:** `spec.name`, `spec.transient`, `spec.warehouseType`, `spec.type`, `spec.databaseRef` are rejected on UPDATE.

{: .note }
> Immutable fields are enforced at two layers — CRD schema (CEL `self == oldSelf`) and reconciler-level terminal error.

---

## Create a Schema

Schemas belong to a Database. The Schema controller resolves the `databaseRef`, waiting for the parent Database to be Ready.

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Schema
metadata:
  name: my-schema
spec:
  name: PUBLIC
  databaseRef:
    name: my-database
  providerRef:
    name: default
```

```bash
kubectl get schemas
```

```
NAME        SCHEMA   DATABASE      READY   SYNCED   REFS   OWNER      AGE
my-schema   PUBLIC   MY_DATABASE   True    True     True   SYSADMIN   1m
```

### Database + Schema Together

```yaml
---
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: analytics-db
spec:
  name: ANALYTICS
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
  providerRef:
    name: default
```

The order does not matter — the Schema controller requeues until its Database is Ready.

### Cascading Deletion

Delete a Database CR even if Schema CRs still exist — the Schema controller uses cached `status.databaseName` for `DROP SCHEMA`.

{: .note }
> ProviderConfig is protected by an in-use finalizer — it cannot be deleted while any managed resource references it.

---

## Create a Warehouse

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

### Detect-Only Drift Policy

```yaml
metadata:
  annotations:
    snowplane.hupe1980.github.io/drift-policy: detect-only
```

See [Drift Detection]({% link drift-detection.md %}) for details.

---

## Create Account Roles

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
kubectl get accountroles  # or: kubectl get ar
```

---

## Use Role (USE ROLE)

All resource types support `spec.useRole` to switch the active Snowflake role before operations:

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: analytics-db
spec:
  name: ANALYTICS_DB
  useRole: DATA_ADMIN
  providerRef:
    name: default
```

{: .important }
> `spec.useRole` is **immutable** after creation. The service user must have the target role directly granted.

---

## Create Users

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: User
metadata:
  name: etl-service
spec:
  name: ETL_SERVICE_USER
  type: SERVICE
  defaultRole: ETL_ROLE
  rsaPublicKey:
    name: etl-service-keys
    key: rsa-public-key
  providerRef:
    name: default
```

---

## Grant Privileges

Three grant CRDs are available:

```yaml
# AccountRoleGrant
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: AccountRoleGrant
metadata:
  name: usage-on-analytics-db
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

{: .warning }
> `withGrantOption: true` enables re-granting. Revoking the original Grant CR does NOT cascade-revoke re-grants.

All grant spec fields are **immutable** after creation.

---

## Export Fields

FieldExport copies values from any managed resource's status into a ConfigMap or Secret:

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

---

## CREATE OR ALTER

For Database, Schema, Table, Warehouse, Task, Tag, View, FileFormat, MaskingPolicy, PasswordPolicy, NetworkRule, RowAccessPolicy, and User, `CREATE OR ALTER` is **enabled by default**. To opt out and use the legacy `CREATE IF NOT EXISTS` + `ALTER` two-step flow:

```yaml
metadata:
  annotations:
    snowplane.hupe1980.github.io/use-create-or-alter: "false"
```

{: .note }
> `CREATE OR ALTER` is a Snowflake preview feature. If Snowflake returns an unsupported-feature error, the controller automatically falls back to the two-step flow.

---

## What's Next

Snowplane manages **30 resource types** including Task, Stream, Tag, NetworkPolicy, NetworkRule, PasswordPolicy, ResourceMonitor, MaskingPolicy, RowAccessPolicy, StorageIntegration, SecurityIntegration, FileFormat, Pipe, DynamicTable, AccountRoleAssignment, and DatabaseRoleAssignment.

See `config/samples/` for example CRs of every resource type.
