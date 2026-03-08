---
title: API Reference
nav_order: 2
---

# 📖 API Reference

Complete field-level documentation for all Snowplane CRDs. Each resource supports full lifecycle management (create, alter, drop), drift detection, adoption of pre-existing objects, and deletion policies.

> 💡 **Nil-means-unmanaged convention:** Pointer fields (`*string`, `*int32`, `*bool`) use `nil` to mean "not managed by Snowplane." When nil, the controller skips the parameter in CREATE/ALTER, leaving Snowflake's server-side default intact.

> 🏷️ **Common fields:** Every managed resource (except ProviderConfig and FieldExport) embeds `CommonSpec` which provides:
> - `spec.providerRef.name` — ProviderConfig reference (default: `"default"`)
> - `spec.providerRef.namespace` — Optional cross-namespace ProviderConfig reference
> - `spec.deletionPolicy` — `Delete` (default) / `Orphan`
> - `spec.useRole` — Optional Snowflake role activated via `USE ROLE` before SQL operations
> - `spec.paused` — Suspend reconciliation (`bool`, default: `false`)
> - `spec.managementPolicies.adoptionPolicy` — `fail-if-exists` (default) / `adopt` — controls whether pre-existing Snowflake objects are adopted
> - `spec.managementPolicies.driftPolicy` — `correct` (default) / `detect-only` — controls whether detected drift is auto-corrected
> - `spec.managementPolicies.createOrAlter` — Use CREATE OR ALTER flow (`*bool`, default: `true`)

> 🔗 **Cross-namespace references:** All `ObjectReference` fields (e.g., `databaseRef`, `schemaRef`, `accountRoleRef`) support an optional `namespace` field. When omitted, the reference resolves within the same namespace as the referencing resource. When set, the reference resolves in the specified namespace — enabling platform teams to manage shared infrastructure (Databases, Warehouses, Roles) in a central namespace while project teams reference them from their own namespaces.

> 🏷️ **Annotations:** `snowplane.hupe1980.github.io/force-destroy` enables CASCADE DROP for databases and schemas. `snowplane.hupe1980.github.io/force-new` triggers delete-and-recreate for immutable field changes. `snowplane.hupe1980.github.io/late-initialized` is set to `"true"` after adoption, indicating spec fields were populated from observed state.

> 🔄 **Late-initialization:** When `adoptionPolicy: adopt` is used, nil spec fields are automatically populated from the existing Snowflake resource state (ShowOutput, DescribeOutput, Parameters). This ensures the adopted CR's spec accurately represents the managed state. Supports 33 adapters: Database, SecondaryDatabase, SharedDatabase, Schema, Warehouse, User, Task, PasswordPolicy, Table, Alert, DynamicTable, Sequence, View, Tag, AccountRole, DatabaseRole, StorageIntegrationAWS, StorageIntegrationGCS, StorageIntegrationAzure, ExternalVolume, CortexSearchService, GitRepository, Streamlit, InternalStage, ExternalStage, Pipe, NetworkPolicy, EmailNotificationIntegration, QueueNotificationIntegration, WebhookNotificationIntegration, ResourceMonitor, APIIntegration, ComputePool, Service.

> ⏱️ **LastReconcileTime:** Every resource's `status.lastReconcileTime` is stamped on each successful reconcile (create, update, adoption, and post-crash recovery) via `finalizeSpec()`. Use this for SLO dashboards, staleness alerting, and diagnosing whether reconciliation is running for a specific resource.

> ✈️ **Pre-flight validation:** For all database/schema-scoped resources (38 types), the controller automatically verifies that the referenced Snowflake database and schema exist before issuing CREATE. When using `databaseRef`/`schemaRef`, existence is validated via CR readiness. When using raw `databaseName`/`schemaName` strings, the controller issues `SHOW DATABASES LIKE`/`SHOW SCHEMAS LIKE` queries. Non-existent targets set `Ready=False` with reason `DependencyNotReady`.

---

<details>
<summary>🔌 <strong>ProviderConfig</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.account` | `string` | Snowflake account identifier |
| `spec.region` | `string` | Snowflake cloud region |
| `spec.user` | `string` | Snowflake username |
| `spec.role` | `string` | Snowflake role to assume |
| `spec.warehouse` | `string` | Default warehouse |
| `spec.authenticationType` | `enum` | `KeyPair` / `UsernamePassword` / `WorkloadIdentity` |
| `spec.credentials.secretRef` | `SecretKeyReference` | Reference to credentials Secret |
| `spec.credentials.passphraseKey` | `string` | Key within the same Secret that holds the passphrase for encrypted PKCS#8 private keys (KeyPair only) |
| `spec.workloadIdentity.audience` | `string` | OIDC audience for WIF |
| `spec.workloadIdentity.tokenFilePath` | `string` | Path to projected SA token file |
| `spec.workloadIdentity.provider` | `enum` | `OIDC` (default) / `AWS` / `GCP` / `Azure` |
| `spec.allowedNamespaces` | `[]string` | Static list of namespaces that may reference this ProviderConfig. Empty = all allowed. Contains `"*"` = all allowed. |
| `spec.allowedNamespaceSelector` | `*LabelSelector` | Label selector matching namespaces that may reference this ProviderConfig. Used as OR with `allowedNamespaces`. |
| `spec.allowedDatabases` | `[]string` | Restrict which Snowflake databases may be targeted by resources using this ProviderConfig. Empty = all allowed. Case-insensitive. |
| `spec.allowedSchemas` | `[]string` | Restrict which Snowflake schemas may be targeted. Supports `"SCHEMA"` or `"DATABASE.SCHEMA"` format. Empty = all allowed. |
| `spec.allowedRefNamespaces` | `[]string` | Restrict which Kubernetes namespaces cross-namespace refs may target. Empty = unrestricted. `"*"` = all allowed. `"SAME"` = same-namespace only. Specific names for explicit allow. |

</details>

<details>
<summary>🗄️ <strong>Database</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Snowflake database name *(immutable)* |
| `spec.comment` | `*string` | Optional description |
| `spec.dataRetentionTimeInDays` | `*int32` | Time Travel retention (0–90 days) |
| `spec.maxDataExtensionTimeInDays` | `*int32` | Max data extension days (0–90) |
| `spec.transient` | `bool` | Transient database — no Fail-safe *(immutable)* |
| `spec.catalog` | `*string` | Iceberg catalog integration name |
| `spec.externalVolume` | `*string` | External volume for Iceberg tables |
| `spec.replaceInvalidCharacters` | `*bool` | Replace invalid UTF-8 characters |
| `spec.defaultDDLCollation` | `*string` | Default string column collation |
| `spec.storageSerializationPolicy` | `*enum` | `COMPATIBLE` / `OPTIMIZED` |
| `spec.logLevel` | `*enum` | `TRACE` / `DEBUG` / `INFO` / `WARN` / `ERROR` / `FATAL` / `OFF` |
| `spec.metricLevel` | `*enum` | `NONE` / `ALL` |
| `spec.traceLevel` | `*enum` | `ALWAYS` / `ON_EVENT` / `OFF` |
| `spec.useRole` | `*string` | Snowflake role activated via USE ROLE before SQL operations |
| `spec.deletionPolicy` | `enum` | `Delete` (default) / `Orphan` |
| `spec.providerRef.name` | `string` | Name of the ProviderConfig to use |

> 💡 **Nil-means-unmanaged:** Pointer fields (`*string`, `*int32`, `*bool`) use nil to mean "not managed by Snowplane." When nil, the controller skips the parameter in CREATE/ALTER, leaving Snowflake's server-side default intact.

</details>

<details>
<summary>�️ <strong>SecondaryDatabase</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Snowflake database name *(immutable, 1–255 chars)* |
| `spec.asReplicaOf` | `string` | Fully qualified source database: `<org>.<account>.<db>` *(immutable)* |
| `spec.comment` | `*string` | Optional description |
| `spec.dataRetentionTimeInDays` | `*int32` | Time Travel retention (0–90 days) |
| `spec.maxDataExtensionTimeInDays` | `*int32` | Max data extension days (0–90) |

> 🔄 **Replication:** Secondary databases are read-only replicas of a primary database in another account. Use `asReplicaOf` with the format `org.account.database_name`. Data retention settings can be configured independently of the primary.

</details>

<details>
<summary>🗄️ <strong>SharedDatabase</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Snowflake database name *(immutable, 1–255 chars)* |
| `spec.fromShare` | `string` | Source share: `<account>.<share>` or `<org>.<account>.<share>` *(immutable)* |
| `spec.comment` | `*string` | Optional description |
| `spec.externalVolume` | `*string` | External volume for Iceberg table storage |
| `spec.catalog` | `*string` | Apache Iceberg catalog integration name |
| `spec.defaultDDLCollation` | `*string` | Default string column collation |
| `spec.replaceInvalidCharacters` | `*bool` | Replace invalid UTF-8 characters |
| `spec.storageSerializationPolicy` | `*enum` | `COMPATIBLE` / `OPTIMIZED` |
| `spec.logLevel` | `*enum` | `TRACE` / `DEBUG` / `INFO` / `WARN` / `ERROR` / `FATAL` / `OFF` |
| `spec.traceLevel` | `*enum` | `ALWAYS` / `ON_EVENT` / `OFF` |

> 🤝 **Data Sharing:** Shared databases are created from a Snowflake share provided by another account. Use `fromShare` with the provider account and share name. Supports the same Iceberg, collation, and logging parameters as standard databases.

</details>

<details>
<summary>�📂 <strong>Schema</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Snowflake schema name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.comment` | `*string` | Optional description |
| `spec.dataRetentionTimeInDays` | `*int32` | Time Travel retention (0–90 days) |
| `spec.maxDataExtensionTimeInDays` | `*int32` | Max data extension days (0–90) |
| `spec.transient` | `bool` | Transient schema *(immutable)* |
| `spec.managedAccess` | `bool` | Managed access schema |
| `spec.storageSerializationPolicy` | `*enum` | `COMPATIBLE` / `OPTIMIZED` |
| `spec.logLevel` / `metricLevel` / `traceLevel` | `*enum` | Same as Database |
| `spec.useRole` | `*string` | Snowflake role for USE ROLE |

> 🔗 **Cross-Resource References:** `databaseRef` points to a Database CR in the same namespace. The controller verifies the Database is Ready before proceeding. If not Ready, the Schema enters `DependencyNotReady` and auto-requeues.

</details>

<details>
<summary>⚡ <strong>Warehouse</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Snowflake warehouse name *(immutable)* |
| `spec.warehouseType` | `*enum` | `STANDARD` / `SNOWPARK-OPTIMIZED` |
| `spec.warehouseSize` | `*enum` | `XSMALL` … `6XLARGE` |
| `spec.minClusterCount` / `maxClusterCount` | `*int32` | Multi-cluster warehouse scaling (1–10) |
| `spec.scalingPolicy` | `*enum` | `STANDARD` / `ECONOMY` |
| `spec.autoSuspend` | `*int32` | Auto-suspend timeout (seconds) |
| `spec.autoResume` | `*bool` | Auto-resume on query |
| `spec.initiallySuspended` | `bool` | Create in suspended state |
| `spec.resourceMonitor` | `*string` | Resource monitor name |
| `spec.enableQueryAcceleration` | `*bool` | Query acceleration |
| `spec.queryAccelerationMaxScaleFactor` | `*int32` | Max scale factor (0–100) |
| `spec.maxConcurrencyLevel` | `*int32` | Max concurrent queries (1–32) |
| `spec.resourceConstraint` | `*enum` | `MEMORY_1X` etc. |

</details>

<details>
<summary>🎭 <strong>AccountRole</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Snowflake role name *(immutable)* |
| `spec.comment` | `*string` | Optional description |
| `spec.useRole` | `*string` | Snowflake role for USE ROLE |

</details>

<details>
<summary>🎭 <strong>DatabaseRole</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Database role name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.comment` | `*string` | Optional description |

</details>

<details>
<summary>👤 <strong>User</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Snowflake user name *(immutable)* |
| `spec.type` | `*enum` | `PERSON` / `SERVICE` / `LEGACY_SERVICE` *(immutable)* |
| `spec.loginName` | `*string` | Login name (defaults to user name) |
| `spec.displayName` | `*string` | Display name |
| `spec.email` | `*string` | Email address |
| `spec.firstName` | `*string` | First name |
| `spec.lastName` | `*string` | Last name |
| `spec.middleName` | `*string` | Middle name |
| `spec.comment` | `*string` | User description |
| `spec.password` | `*SecretKeyReference` | Secret reference for password |
| `spec.rsaPublicKey` | `*SecretKeyReference` | Secret reference for RSA public key |
| `spec.rsaPublicKey2` | `*SecretKeyReference` | Second RSA key (for rotation) |
| `spec.defaultRole` | `*string` | Default role on login |
| `spec.defaultSecondaryRoles` | `*string` | Secondary roles on login (`ALL`) |
| `spec.defaultWarehouse` | `*string` | Default warehouse |
| `spec.defaultNamespace` | `*string` | Default database.schema namespace |
| `spec.mustChangePassword` | `*bool` | Force password change on next login |
| `spec.disabled` | `*bool` | Whether the user is disabled |
| `spec.daysToExpiry` | `*int32` | Days until credentials expire (≥ 0) |
| `spec.minsToUnlock` | `*int32` | Minutes until locked account auto-unlocks (≥ 0) |
| `spec.minsToBypassMFA` | `*int32` | Minutes to temporarily bypass MFA (≥ 0) |
| `spec.networkPolicy` | `*string` | User-level network policy override |
| `spec.disableMFA` | `*bool` | Disable multi-factor authentication |

> 🔐 **Secret-Referenced Credentials:** `password`, `rsaPublicKey`, and `rsaPublicKey2` use `SecretKeyReference` to point to Kubernetes Secrets. Sensitive data is never stored in the CR.

</details>

<details>
<summary>🔑 <strong>GrantPrivilegesToAccountRole</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.privilege` | `string` | Snowflake privilege (e.g. USAGE, SELECT) *(immutable, mutually exclusive with allPrivileges)* |
| `spec.allPrivileges` | `bool` | Grant all privileges on the target *(immutable, mutually exclusive with privilege)* |
| `spec.on` | `GrantOn` | Grant target — exactly one of `account`, `accountObject`, `schema`, `schemaObject` *(immutable)* |
| `spec.accountRole` | `string` | Account role name *(immutable, mutually exclusive with accountRoleRef)* |
| `spec.accountRoleRef` | `ObjectReference` | AccountRole CR reference *(immutable, mutually exclusive with accountRole)* |
| `spec.withGrantOption` | `bool` | Allow grantee to re-grant *(immutable)* |

**GrantOn** — exactly one field must be set:

| Field | Type | Description |
|-------|------|-------------|
| `account` | `bool` | Global account-level privilege (`ON ACCOUNT`) |
| `accountObject` | `GrantOnAccountObject` | Account-level object (DATABASE, WAREHOUSE, etc.) |
| `schema` | `GrantOnSchema` | Schema or all/future schemas in a database |
| `schemaObject` | `GrantOnSchemaObject` | Schema object, or all/future schema objects of a type |

> ⚠️ **Grant Immutability:** All spec fields are immutable after creation. Changing any field requires deleting and recreating the CR (or using the `force-new` annotation).

> 💡 **Grant DeletionPolicy:** With `deletionPolicy: Delete` (default), deleting the CR issues `REVOKE <privilege>`. With `deletionPolicy: Orphan`, the grant **persists in Snowflake** after the CR is deleted — the privilege remains active. Use `Orphan` when you want to remove the CR from Kubernetes without revoking the Snowflake privilege.

</details>

<details>
<summary>🔑 <strong>GrantPrivilegesToDatabaseRole</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.privilege` | `string` | Snowflake privilege (e.g. USAGE, SELECT) *(immutable, mutually exclusive with allPrivileges)* |
| `spec.allPrivileges` | `bool` | Grant all privileges on the target *(immutable, mutually exclusive with privilege)* |
| `spec.on` | `GrantPrivilegesToDatabaseRoleOn` | Grant target — exactly one of `database`, `schema`, `schemaObject` *(immutable)* |
| `spec.databaseRole` | `string` | Fully qualified database role name *(immutable, mutually exclusive with databaseRoleRef)* |
| `spec.databaseRoleRef` | `ObjectReference` | DatabaseRole CR reference *(immutable, mutually exclusive with databaseRole)* |
| `spec.withGrantOption` | `bool` | Allow grantee to re-grant *(immutable)* |

**GrantPrivilegesToDatabaseRoleOn** — exactly one field must be set:

| Field | Type | Description |
|-------|------|-------------|
| `database` | `*string` | Grant on the database itself (`ON DATABASE <name>`) |
| `schema` | `GrantOnSchema` | Schema or all/future schemas |
| `schemaObject` | `GrantOnSchemaObject` | Schema object, or all/future schema objects |

> ⚠️ **Grant Immutability:** All spec fields are immutable after creation. Changing any field requires deleting and recreating the CR (or using the `force-new` annotation).

</details>

<details>
<summary>🔑 <strong>AccountRoleAssignment</strong> (shortName: <code>ara</code>)</summary>

Assigns an account role to another role or user: `GRANT ROLE <role> TO ROLE|USER <target>`.

| Field | Type | Description |
|-------|------|-------------|
| `spec.roleName` | `string` | Account role to assign *(immutable, mutually exclusive with roleRef)* |
| `spec.roleRef` | `ObjectReference` | AccountRole CR reference *(immutable, mutually exclusive with roleName)* |
| `spec.toRole` | `string` | Target account role *(immutable, mutually exclusive with toRoleRef, toUser, toUserRef)* |
| `spec.toRoleRef` | `ObjectReference` | AccountRole CR reference for the target *(immutable)* |
| `spec.toUser` | `string` | Target user *(immutable, mutually exclusive with toUserRef, toRole, toRoleRef)* |
| `spec.toUserRef` | `ObjectReference` | User CR reference for the target *(immutable)* |

> ⚠️ **Assignment Immutability:** All spec fields are immutable after creation. Changing any field requires deleting and recreating the CR (or using the `force-new` annotation).

</details>

<details>
<summary>🔑 <strong>DatabaseRoleAssignment</strong> (shortName: <code>dra</code>)</summary>

Assigns a database role to an account role or another database role: `GRANT DATABASE ROLE <db>.<role> TO ROLE|DATABASE ROLE <target>`.

| Field | Type | Description |
|-------|------|-------------|
| `spec.databaseRoleName` | `string` | Fully qualified database role *(immutable, mutually exclusive with databaseRoleRef)* |
| `spec.databaseRoleRef` | `ObjectReference` | DatabaseRole CR reference *(immutable, mutually exclusive with databaseRoleName)* |
| `spec.toRole` | `string` | Target account role *(immutable, mutually exclusive with toRoleRef, toDatabaseRole, toDatabaseRoleRef)* |
| `spec.toRoleRef` | `ObjectReference` | AccountRole CR reference for the target *(immutable)* |
| `spec.toDatabaseRole` | `string` | Fully qualified target database role *(immutable, mutually exclusive with toDatabaseRoleRef, toRole, toRoleRef)* |
| `spec.toDatabaseRoleRef` | `ObjectReference` | DatabaseRole CR reference for the target *(immutable)* |

> ⚠️ **Assignment Immutability:** All spec fields are immutable after creation. Changing any field requires deleting and recreating the CR (or using the `force-new` annotation).

</details>

<details>
<summary>🔑 <strong>GrantPrivilegesToShare</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.privilege` | `string` | Snowflake privilege (USAGE, SELECT, REFERENCE_USAGE, READ, EVOLVE SCHEMA) *(immutable, required)* |
| `spec.on` | `GrantPrivilegesToShareOn` | Grant target — exactly one field must be set *(immutable)* |
| `spec.share` | `string` | Share name *(immutable, required)* |

**GrantPrivilegesToShareOn** — exactly one field must be set:

| Field | Type | Description |
|-------|------|-------------|
| `database` | `*string` | Grant on a database (`ON DATABASE <name>`) |
| `schema` | `*string` | Grant on a schema (`ON SCHEMA <fqn>`) |
| `table` | `*string` | Grant on a table (`ON TABLE <fqn>`) |
| `allTablesInSchema` | `*string` | Grant on all tables in a schema (`ON ALL TABLES IN SCHEMA <fqn>`) |
| `functionName` | `*string` | Grant on a function (`ON FUNCTION <fqn>`) |
| `tag` | `*string` | Grant on a tag (`ON TAG <fqn>`) |
| `view` | `*string` | Grant on a view (`ON VIEW <fqn>`) |

> ⚠️ **Grant Immutability:** All spec fields are immutable after creation. Shares do not support `allPrivileges` or `withGrantOption`.

</details>

<details>
<summary>📊 <strong>Table</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Table name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.columns` | `[]ColumnDefinition` | Column definitions (name, type, nullable, default, comment). Column drift is detected and corrected via ADD/DROP/ALTER COLUMN. |
| `spec.constraints` | `[]TableConstraint` | Table constraints (PRIMARY KEY, UNIQUE, FOREIGN KEY) *(immutable)* |
| `spec.transient` | `bool` | Transient table *(immutable)* |
| `spec.dataRetentionTimeInDays` | `*int32` | Time Travel retention (0–90) |
| `spec.maxDataExtensionTimeInDays` | `*int32` | Max data extension time (0–90) |
| `spec.clusterBy` | `[]string` | Clustering key expressions |
| `spec.changeTracking` | `*bool` | Enable change tracking |
| `spec.enableSchemaEvolution` | `*bool` | Enable schema evolution |
| `spec.defaultDDLCollation` | `*string` | Default DDL collation |
| `spec.comment` | `*string` | Table comment |

</details>

<details>
<summary>👁️ <strong>View</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | View name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.statement` | `string` | SQL SELECT statement (triggers CREATE OR REPLACE on change) |
| `spec.secure` | `bool` | Enable SECURE VIEW |
| `spec.changeTracking` | `*bool` | Enable change tracking |

> ⚠️ **Security:** The `statement` field is executed verbatim as SQL. Ensure RBAC restricts View CR access to trusted principals.

</details>

<details>
<summary>� <strong>MaterializedView</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Materialized view name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.statement` | `string` | SQL SELECT statement *(immutable)* |
| `spec.secure` | `bool` | Enable SECURE MATERIALIZED VIEW |
| `spec.comment` | `*string` | Comment for the materialized view |
| `spec.clusterBy` | `[]string` | Cluster-by expressions *(immutable)* |

> ⚠️ **Enterprise Edition:** Materialized views require Snowflake Enterprise Edition or higher.
>
> ⚠️ **No CREATE OR ALTER:** Materialized views do not support CREATE OR ALTER. Immutable field changes require delete/recreate (use `force-new` annotation).

</details>

> **Note:** The unified `Stage` CRD has been removed. Use `InternalStage` and `ExternalStage` instead.

<details>
<summary>📤 <strong>FieldExport</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.from.resource.kind` | `string` | Source resource kind (e.g. `Database`) |
| `spec.from.resource.name` | `string` | Source resource name |
| `spec.from.path` | `string` | Dot-notation path (e.g. `.status.showOutput.name`) |
| `spec.to.kind` | `enum` | `ConfigMap` / `Secret` |
| `spec.to.name` | `string` | Target name |
| `spec.to.key` | `string` | Key within the target data |

> 📤 **Cross-Resource Data Passing:** FieldExport reads status fields and writes them to ConfigMaps/Secrets. The exported value is tracked by SHA-256 hash to avoid unnecessary writes. On deletion, exported keys are cleaned up.
>
> 🔒 **Same-Namespace Security:** FieldExport is restricted to resources in the same namespace — the source resource, target ConfigMap/Secret, and the FieldExport itself must all reside in the same namespace. This prevents cross-namespace privilege escalation (following the ACK model).
>
> 🛡️ **Sensitive Field Guard:** Status fields containing sensitive information (OAuth credentials, PII, key fingerprints, AWS ARNs, endpoint URLs) cannot be exported to a ConfigMap — they must target a Secret. The guard covers 20 CRD kinds including all Secret, API Auth Integration, ExternalOAuthIntegration, SAML2Integration, User, StorageIntegrationAWS, StorageIntegrationGCS, StorageIntegrationAzure, ExternalStage, and Pipe types. See `SensitiveStatusPaths` in `api/v1alpha1/sensitive_paths.go` for the full registry.

</details>

<details>
<summary>🔧 <strong>SQLStatement</strong> (shortName: <code>sqlstmt</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.execute` | `string` | **Required.** SQL to run on create (multi-statement supported via `;`) |
| `spec.revert` | `string` | SQL to run on delete (optional, multi-statement) |
| `spec.observe` | `string` | SQL to run periodically to check state (optional) |
| `spec.observeExpect` | `[]object` | Column/value pairs the observe query must match |
| `spec.observeExpect[].column` | `string` | Column name (case-insensitive match) |
| `spec.observeExpect[].value` | `string` | Expected value (case-insensitive match) |
| `spec.idempotent` | `bool` | Informational: indicates whether execute SQL is safe to run multiple times (default false) |
| `spec.dangerousAllowDestructive` | `bool` | Required when `execute` contains DROP/TRUNCATE/DELETE/REMOVE (revert is exempt) |
| `spec.useRole` | `string` | Snowflake role to USE before execution *(immutable)* |
| `status.executeHash` | `string` | SHA-256 hash of the execute SQL at creation time |
| `status.observeResult.rowCount` | `int32` | Number of rows from last observe query |
| `status.observeResult.matched` | `bool` | Whether all expectations were satisfied |

> 🔧 **Escape-Hatch Resource:** SQLStatement executes arbitrary SQL that doesn't map to any first-class Snowflake CRD. Typical use cases: GRANT statements, ALTER ACCOUNT, session parameters, or any DDL/DML not yet supported by a dedicated resource type.
>
> 🔒 **Safety Guards:** `spec.execute` is immutable after creation — changing it requires the `force-new` annotation. Destructive keywords (DROP, TRUNCATE, DELETE, REMOVE) are blocked unless `dangerousAllowDestructive` is explicitly set. The controller is disabled by default (`--enable-sql-statement=false`).
>
> 📡 **Observe & Drift:** When `observe` SQL is provided, the controller runs it periodically and checks expectations. A mismatch sets the `DriftDetected` condition. Without observe SQL, the resource reports as "not yet observable" and relies on hash tracking.

</details>

<details>
<summary>⏰ <strong>Task</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Task name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.sqlStatement` | `string` | SQL code executed when the task runs |
| `spec.schedule` | `*string` | Cron or interval schedule (e.g. `5 MINUTES`) |
| `spec.warehouseRef.name` | `*string` | Warehouse CR reference *(mutually exclusive with warehouseName and serverless size)* |
| `spec.warehouseName` | `*string` | Warehouse Snowflake name *(mutually exclusive with warehouseRef and serverless size)* |
| `spec.userTaskManagedInitialWarehouseSize` | `*enum` | Serverless warehouse size: `XSMALL`…`XXLARGE` |
| `spec.after` | `[]TaskPredecessor` | Predecessor tasks for DAG scheduling — each entry has `ref.name` or `name` (XOR) |
| `spec.when` | `*string` | Boolean SQL condition for conditional execution |
| `spec.suspend` | `*bool` | Whether the task is suspended (default: `true`) |
| `spec.comment` | `*string` | Optional description |
| `spec.allowOverlappingExecution` | `*bool` | Allow concurrent graph executions |
| `spec.userTaskTimeoutMs` | `*int32` | Single-run timeout in milliseconds (0–604800000) |
| `spec.suspendTaskAfterNumFailures` | `*int32` | Auto-suspend after N consecutive failures |
| `spec.errorIntegrationRef.name` | `*string` | NotificationIntegration CR reference for errors *(XOR with errorIntegrationName)* |
| `spec.errorIntegrationName` | `*string` | NotificationIntegration Snowflake name for errors *(XOR with errorIntegrationRef)* |
| `spec.successIntegrationRef.name` | `*string` | NotificationIntegration CR reference for success *(XOR with successIntegrationName)* |
| `spec.successIntegrationName` | `*string` | NotificationIntegration Snowflake name for success *(XOR with successIntegrationRef)* |
| `spec.finalizeRef.name` | `*string` | Finalizer Task CR reference *(XOR with finalizeName)* |
| `spec.finalizeName` | `*string` | Finalizer Task Snowflake name *(XOR with finalizeRef)* |
| `spec.taskAutoRetryAttempts` | `*int32` | Automatic retry attempts (0–30) |

> ⏰ **DAG Scheduling:** Use `after` to chain tasks into directed acyclic graphs. Each predecessor entry accepts either a `ref.name` (CR reference) or a `name` (Snowflake name) — exactly one must be set. Root tasks require a `schedule`; child tasks inherit it from the root.

> ⚠️ **Security:** The `sqlStatement`, `when`, and `config` fields are embedded into SQL statements. Ensure RBAC restricts Task CR access to trusted principals.

</details>

<details>
<summary>� <strong>Alert</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Alert name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.condition` | `string` | SQL condition query (evaluated inside `IF(EXISTS(...))`) |
| `spec.action` | `string` | SQL statement executed when condition is true |
| `spec.schedule` | `*string` | Cron or interval schedule (e.g. `10 MINUTE`) — omit for streaming alerts |
| `spec.warehouseRef.name` | `*string` | Warehouse CR reference *(XOR with warehouseName)* — omit for serverless |
| `spec.warehouseName` | `*string` | Warehouse Snowflake name *(XOR with warehouseRef)* — omit for serverless |
| `spec.suspend` | `*bool` | Whether the alert is suspended (default: `true`) |
| `spec.comment` | `*string` | Optional description |

> 🔔 **Monitoring:** Alerts periodically evaluate a condition query and execute an action when the condition returns rows. Use for monitoring data quality, anomaly detection, or triggering notification procedures.

> ⚠️ **Security:** The `condition` and `action` fields are embedded into SQL statements. Ensure RBAC restricts Alert CR access to trusted principals.

</details>

<details>
<summary>🔄 <strong>StreamOnTable</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Stream name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.tableRef.name` | `string` | Source Table CR reference *(XOR with tableName, immutable)* |
| `spec.tableName` | `string` | Source table Snowflake name *(XOR with tableRef, immutable)* |
| `spec.appendOnly` | `*bool` | Track row inserts only |
| `spec.showInitialRows` | `*bool` | Include existing rows on first consume |
| `spec.comment` | `*string` | Optional description |

> 🔄 **Change Data Capture:** Tracks DML changes on a table. Use with Tasks to build real-time data pipelines.

</details>

<details>
<summary>🔄 <strong>StreamOnView</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Stream name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.viewRef.name` | `string` | Source View CR reference *(XOR with viewName, immutable)* |
| `spec.viewName` | `string` | Source view Snowflake name *(XOR with viewRef, immutable)* |
| `spec.appendOnly` | `*bool` | Track row inserts only |
| `spec.showInitialRows` | `*bool` | Include existing rows on first consume |
| `spec.comment` | `*string` | Optional description |

> 🔄 **Change Data Capture:** Tracks DML changes on a view. Use with Tasks to build real-time data pipelines.

</details>

<details>
<summary>🔄 <strong>StreamOnExternalTable</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Stream name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.externalTableRef.name` | `string` | Source ExternalTable CR reference *(XOR with externalTableName, immutable)* |
| `spec.externalTableName` | `string` | Source external table Snowflake name *(XOR with externalTableRef, immutable)* |
| `spec.insertOnly` | `*bool` | Track inserts only (the only mode for external tables) |
| `spec.comment` | `*string` | Optional description |

> 🔄 **Change Data Capture:** Tracks inserts on an external table.

</details>

<details>
<summary>🔄 <strong>StreamOnDirectoryTable</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Stream name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.stageRef.name` | `string` | Source Stage CR reference *(XOR with stageName, immutable)* |
| `spec.stageName` | `string` | Source stage Snowflake name *(XOR with stageRef, immutable)* |
| `spec.comment` | `*string` | Optional description |

> 🔄 **Change Data Capture:** Tracks file changes on a stage's directory table.

</details>

<details>
<summary>🔄 <strong>StreamOnDynamicTable</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Stream name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.dynamicTableRef.name` | `string` | Source DynamicTable CR reference *(XOR with dynamicTableName, immutable)* |
| `spec.dynamicTableName` | `string` | Source dynamic table Snowflake name *(XOR with dynamicTableRef, immutable)* |
| `spec.appendOnly` | `*bool` | Track row inserts only |
| `spec.showInitialRows` | `*bool` | Include existing rows on first consume |
| `spec.comment` | `*string` | Optional description |

> 🔄 **Change Data Capture:** Tracks DML changes on a dynamic table. Use with Tasks to build real-time data pipelines.

</details>

> **Note:** The unified `StorageIntegration` CRD has been removed. Use `StorageIntegrationAWS`, `StorageIntegrationGCS`, and `StorageIntegrationAzure` instead.

<details>
<summary>📦 <strong>ExternalVolume</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | External volume name *(immutable)* |
| `spec.storageLocations` | `[]StorageLocation` | Cloud storage locations *(min 1)* |
| `spec.storageLocations[].name` | `string` | Location name *(unique within volume)* |
| `spec.storageLocations[].storageProvider` | `enum` | `S3` / `S3GOV` / `GCS` / `AZURE` |
| `spec.storageLocations[].storageBaseURL` | `string` | Base URL (e.g. `s3://bucket/path/`) |
| `spec.storageLocations[].storageAWSRoleARN` | `*string` | IAM role ARN for S3/S3GOV access |
| `spec.storageLocations[].storageAWSExternalID` | `*string` | External ID for S3/S3GOV trust policy |
| `spec.storageLocations[].encryptionType` | `*enum` | `AWS_SSE_S3` / `AWS_SSE_KMS` / `GCS_SSE_KMS` / `NONE` |
| `spec.storageLocations[].encryptionKMSKeyID` | `*string` | KMS key ID for encrypted storage |
| `spec.storageLocations[].azureTenantID` | `*string` | Azure tenant ID for AZURE locations |
| `spec.allowWrites` | `*bool` | Whether Snowflake can write *(default: true)* |
| `spec.comment` | `*string` | Optional description |

> 📦 **Iceberg Table Support:** External volumes provide the storage backend for Apache Iceberg tables in Snowflake. A single external volume can span multiple cloud providers with polymorphic storage locations. The controller reconciles storage locations via `ALTER EXTERNAL VOLUME ADD/REMOVE STORAGE_LOCATION` and tracks drift on `ALLOW_WRITES`, `COMMENT`, and storage location membership.

</details>

<details>
<summary>🔍 <strong>CortexSearchService</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Cortex Search Service name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.warehouseRef.name` | `string` | Warehouse CR reference |
| `spec.on` | `string` | Search column name *(immutable)* |
| `spec.attributes` | `[]string` | Attribute columns for filtering *(immutable)* |
| `spec.targetLag` | `string` | Target freshness lag (e.g. `1 hour`) |
| `spec.query` | `string` | Source data query *(immutable)* |
| `spec.embeddingModel` | `*string` | Embedding model override *(immutable)* |
| `spec.refreshMode` | `*enum` | `FULL` / `INCREMENTAL` *(immutable)* |
| `spec.initialize` | `*enum` | `ON_CREATE` / `ON_SCHEDULE` *(immutable)* |
| `spec.fullIndexBuildIntervalDays` | `*int32` | Days between full index rebuilds |
| `spec.comment` | `*string` | Optional description |
| `status.showOutput` | `object` | Last SHOW CORTEX SEARCH SERVICES output |
| `status.describeOutput` | `object` | Last DESCRIBE output (embedding model, indexing/serving state) |

> 🔍 **Cortex AI Search:** Cortex Search Services enable semantic and hybrid search over Snowflake data powered by LLM embeddings. The `on` column is the primary text field for search, with optional `attributes` for metadata filtering. Many fields are immutable after creation — to change the search column, query, or embedding model, delete and recreate the service. The controller tracks drift on mutable fields (`targetLag`, `warehouse`, `comment`) and reports indexing/serving state via `status.describeOutput`.

</details>

<details>
<summary>📂 <strong>GitRepository</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Git repository name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.origin` | `string` | HTTPS URL of the Git remote *(immutable)* |
| `spec.apiIntegration` | `string` | API integration for Git authentication |
| `spec.gitCredentials` | `*string` | Secret FQN for Git credentials |
| `spec.comment` | `*string` | Optional description |
| `status.showOutput` | `object` | Last SHOW GIT REPOSITORIES output |

> 📂 **Git Version Control:** Git Repositories integrate Snowflake with external Git providers for version-controlled code deployment (stored procedures, Streamlit apps, notebooks). The `origin` URL is immutable — to point at a different repository, delete and recreate the resource. The controller tracks drift on mutable fields (`apiIntegration`, `gitCredentials`, `comment`) and supports FETCH for repository synchronization.

</details>

<details>
<summary>🎨 <strong>Streamlit</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Streamlit app name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.warehouseRef.name` | `string` | Warehouse CR reference |
| `spec.from` | `*string` | Stage source path *(create-only)* |
| `spec.mainFile` | `*string` | Main Python file *(default: streamlit_app.py)* |
| `spec.comment` | `*string` | Optional description |
| `spec.title` | `*string` | Display title |
| `spec.externalAccessIntegrations` | `[]string` | External access integrations |
| `status.showOutput` | `object` | Last SHOW STREAMLITS output |
| `status.describeOutput` | `object` | Last DESCRIBE output (mainFile, packages, URLs) |

> 🎨 **Snowflake Apps:** Streamlit apps are Snowflake-native interactive Python applications. The `from` field specifies a stage source path at creation time only. The controller uses DESCRIBE output to compare `mainFile` values and tracks drift on mutable fields (`queryWarehouse`, `comment`, `title`). Warehouse is resolved via `warehouseRef` for Kubernetes-native dependency tracking.

</details>

> **Note:** The unified `NotificationIntegration` CRD has been removed. Use `EmailNotificationIntegration`, `QueueNotificationIntegration`, and `WebhookNotificationIntegration` instead.

> **Note:** The unified `SecurityIntegration` CRD has been removed. Use `ExternalOAuthIntegration`, `SAML2Integration`, and `SCIMIntegration` instead.

<details>
<summary>🔌 <strong>APIIntegration</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | API integration name *(immutable)* |
| `spec.apiProvider` | `enum` | Cloud API provider *(immutable)*: `aws_api_gateway` / `aws_private_api_gateway` / `aws_gov_api_gateway` / `aws_gov_private_api_gateway` / `azure_api_management` / `google_api_gateway` / `git_https_api` |
| `spec.enabled` | `*bool` | Whether the integration is active *(default: true)* |
| `spec.apiAllowedPrefixes` | `[]string` | URL prefixes the integration can access *(min 1)* |
| `spec.apiBlockedPrefixes` | `[]string` | URL prefixes explicitly denied access |
| `spec.apiAwsRoleArn` | `*string` | IAM role ARN Snowflake assumes for AWS API Gateway |
| `spec.azureTenantId` | `*string` | Azure AD tenant ID *(required for azure_api_management, immutable)* |
| `spec.azureAdApplicationId` | `*string` | Azure AD application ID *(required for azure_api_management)* |
| `spec.googleAudience` | `*string` | Google audience for API Gateway *(required for google_api_gateway, immutable)* |
| `spec.apiKey` | `*string` | API key for authentication |
| `spec.comment` | `*string` | Optional description |
| `status.describeOutput` | `map[string]string` | Key-value pairs from DESCRIBE INTEGRATION |

> 🔌 **External APIs:** API integrations allow Snowflake external functions and services to securely call cloud APIs (AWS API Gateway, Azure API Management, Google API Gateway). Use `status.describeOutput` to retrieve Snowflake-generated values like `API_AWS_IAM_USER_ARN` for IAM trust policy configuration.

</details>

<details>
<summary>�📄 <strong>FileFormat</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | File format name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.type` | `enum` | Format type: `CSV` / `JSON` / `AVRO` / `ORC` / `PARQUET` / `XML` *(immutable)* |
| `spec.fieldDelimiter` | `*string` | Field separator character (CSV only) |
| `spec.recordDelimiter` | `*string` | Record separator character (CSV only) |
| `spec.skipHeader` | `*int32` | Lines to skip at start (CSV only) |
| `spec.fieldOptionallyEnclosedBy` | `*string` | Character to enclose strings (CSV only) |
| `spec.nullIf` | `[]string` | Strings representing NULL (CSV/JSON) |
| `spec.errorOnColumnCountMismatch` | `*bool` | Abort on column count mismatch (CSV only) |
| `spec.compression` | `*string` | Compression: `AUTO` / `GZIP` / `BZ2` / `BROTLI` / `ZSTD` / `DEFLATE` / `RAW_DEFLATE` / `NONE` |
| `spec.stripOuterArray` | `*bool` | Remove outer brackets from JSON arrays (JSON only) |
| `spec.stripNullValues` | `*bool` | Remove key-value pairs with null values (JSON only) |
| `spec.trimSpace` | `*bool` | Remove leading/trailing whitespace |
| `spec.comment` | `*string` | Optional description |

> 📄 **Data Loading:** File formats define parsing rules for structured and semi-structured data. Reuse across stages, pipes, and COPY INTO statements.

</details>

<details>
<summary>🚰 <strong>Pipe</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Pipe name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.copyStatement` | `string` | COPY INTO statement defining pipe behavior *(immutable)* |
| `spec.autoIngest` | `*bool` | Enable automatic data loading on new files *(immutable)* |
| `spec.integrationRef.name` | `*string` | NotificationIntegration CR reference for auto-ingest *(XOR with integrationName, immutable)* |
| `spec.integrationName` | `*string` | NotificationIntegration Snowflake name for auto-ingest *(XOR with integrationRef, immutable)* |
| `spec.awsSnsTopic` | `*string` | Amazon SNS topic ARN for S3 auto-ingest *(immutable)* |
| `spec.errorIntegrationRef.name` | `*string` | NotificationIntegration CR reference for errors *(XOR with errorIntegrationName)* |
| `spec.errorIntegrationName` | `*string` | NotificationIntegration Snowflake name for errors *(XOR with errorIntegrationRef)* |
| `spec.comment` | `*string` | Optional description |
| `status.notificationChannel` | `string` | Cloud notification channel (e.g. SQS ARN) — configure cloud event notifications |

> 🚰 **Continuous Ingestion:** Pipes enable serverless, continuous data loading from cloud storage. Use `autoIngest` with a notification integration for fully automated pipelines.

</details>

<details>
<summary>⚡ <strong>DynamicTable</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Dynamic table name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.query` | `string` | SQL query defining the dynamic table content *(immutable)* |
| `spec.targetLag` | `string` | Maximum acceptable staleness (e.g. `"1 minute"`, `"DOWNSTREAM"`) |
| `spec.warehouseRef.name` | `string` | Warehouse CR reference *(XOR with warehouseName)* |
| `spec.warehouseName` | `string` | Warehouse Snowflake name *(XOR with warehouseRef)* |
| `spec.refreshMode` | `*enum` | Refresh strategy: `AUTO` / `FULL` / `INCREMENTAL` *(immutable)* |
| `spec.initialize` | `*enum` | Initial data population: `ON_CREATE` / `ON_SCHEDULE` *(immutable)* |
| `spec.transient` | `bool` | Transient dynamic table — no Fail-safe *(immutable, default: false)* |
| `spec.clusterBy` | `[]string` | Clustering key expressions |
| `spec.dataRetentionTimeInDays` | `*int32` | Time Travel retention (0–90 days) |
| `spec.comment` | `*string` | Optional description |

> ⚡ **Declarative Pipelines:** Dynamic tables automatically refresh based on upstream changes. Use `targetLag` to control freshness and `DOWNSTREAM` for chained pipeline dependencies.

</details>

<details>
<summary>🏷️ <strong>Tag</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Tag name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.allowedValues` | `[]string` | Valid values when assigning the tag (max 5000) |
| `spec.comment` | `*string` | Optional description |

> 🏷️ **Data Governance:** Tags enable metadata classification for compliance, cost allocation, and access control. Assign tags to databases, schemas, tables, columns, and other objects.

</details>

<details>
<summary>🛡️ <strong>NetworkPolicy</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Network policy name *(immutable)* |
| `spec.allowedIPList` | `[]string` | IPv4 addresses or CIDR ranges allowed access |
| `spec.blockedIPList` | `[]string` | IPv4 addresses or CIDR ranges denied access |
| `spec.allowedNetworkRuleList` | `[]string` | Network rules that allow access |
| `spec.blockedNetworkRuleList` | `[]string` | Network rules that deny access |
| `spec.comment` | `*string` | Optional description |

> 🛡️ **Security Perimeter:** Network policies control which IP addresses and network rules can access your Snowflake account.

</details>

<details>
<summary>🌐 <strong>NetworkRule</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Network rule name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.type` | `enum` | Rule type: `IPV4` / `AWSVPCEID` / `AZURELINKID` / `GCPPSCID` / `HOST_PORT` / `PRIVATE_HOST_PORT` *(immutable)* |
| `spec.mode` | `enum` | Rule mode: `INGRESS` / `INTERNAL_STAGE` / `EGRESS` *(immutable)* |
| `spec.valueList` | `[]string` | Network identifiers (IPs, CIDR ranges, VPC endpoint IDs, etc.) |
| `spec.comment` | `*string` | Optional description |

> 🌐 **Network Identifiers:** Network rules define groups of network identifiers that can be used in network policies and security integrations. Type and mode are immutable after creation.

</details>

<details>
<summary>�️ <strong>AuthenticationPolicy</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Authentication policy name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, mutually exclusive with databaseName)* |
| `spec.databaseName` | `*string` | Snowflake database identifier *(immutable, mutually exclusive with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, mutually exclusive with schemaName)* |
| `spec.schemaName` | `*string` | Snowflake schema identifier *(immutable, mutually exclusive with schemaRef)* |
| `spec.authenticationMethods` | `[]string` | Allowed authentication methods (e.g. `PASSWORD`, `SAML`, `OAUTH`, `KEYPAIR`, `PROGRAMMATIC_ACCESS_TOKEN`, `WORKLOAD_IDENTITY`) |
| `spec.clientTypes` | `[]string` | Allowed client types (e.g. `SNOWFLAKE_UI`, `DRIVERS`, `SNOWFLAKE_CLI`, `SNOWSQL`) |
| `spec.securityIntegrations` | `[]string` | Security integration names allowed with this policy |
| `spec.mfaEnrollment` | `*enum` | MFA enrollment: `REQUIRED` / `REQUIRED_PASSWORD_ONLY` / `OPTIONAL` |
| `spec.mfaPolicy.allowedMethods` | `[]string` | MFA methods allowed (e.g. `TOTP`) |
| `spec.mfaPolicy.enforceMfaOnExternalAuthentication` | `*enum` | MFA for external auth: `OPTIONAL` / `REQUIRED` |
| `spec.patPolicy.defaultExpiryInDays` | `*int32` | Default PAT expiry in days |
| `spec.patPolicy.maxExpiryInDays` | `*int32` | Maximum PAT expiry in days |
| `spec.patPolicy.networkPolicyEvaluation` | `*enum` | PAT network policy eval: `OPTIONAL` / `REQUIRED` |
| `spec.patPolicy.requireRoleRestrictionForServiceUsers` | `*bool` | Require role restriction for service users |
| `spec.workloadIdentityPolicy.allowedProviders` | `[]string` | Identity providers (e.g. `AWS`, `AZURE`, `GCP`) |
| `spec.workloadIdentityPolicy.allowedAwsAccounts` | `[]string` | AWS account IDs for workload identity |
| `spec.workloadIdentityPolicy.allowedAzureIssuers` | `[]string` | Azure issuers for workload identity |
| `spec.workloadIdentityPolicy.allowedOidcIssuers` | `[]string` | OIDC issuers for workload identity |
| `spec.comment` | `*string` | Optional description |

> 🛡️ **Authentication Security:** Authentication policies control how users authenticate to Snowflake. They define allowed methods, client types, MFA requirements, PAT settings, and workload identity providers. Supports `CREATE OR ALTER` for atomic updates. Assign to users or accounts for comprehensive authentication governance.

</details>

<details>
<summary>�🔒 <strong>PasswordPolicy</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Password policy name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.passwordMinLength` | `*int32` | Minimum password length (8–256) |
| `spec.passwordMaxLength` | `*int32` | Maximum password length (8–256) |
| `spec.passwordMinUpperCaseChars` | `*int32` | Minimum uppercase characters |
| `spec.passwordMinLowerCaseChars` | `*int32` | Minimum lowercase characters |
| `spec.passwordMinNumericChars` | `*int32` | Minimum numeric characters |
| `spec.passwordMinSpecialChars` | `*int32` | Minimum special characters |
| `spec.passwordMinAgeDays` | `*int32` | Days before password can be changed |
| `spec.passwordMaxAgeDays` | `*int32` | Days before password must be changed |
| `spec.passwordMaxRetries` | `*int32` | Login attempts before lockout |
| `spec.passwordLockoutTimeMins` | `*int32` | Lockout duration in minutes |
| `spec.passwordHistory` | `*int32` | Number of previous passwords to remember |
| `spec.comment` | `*string` | Optional description |

> 🔒 **Password Compliance:** Password policies enforce authentication strength requirements across your Snowflake account. Assign to users for compliance.

</details>

<details>
<summary>📈 <strong>ResourceMonitor</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Resource monitor name *(immutable)* |
| `spec.creditQuota` | `*int32` | Credits allocated per frequency interval |
| `spec.frequency` | `*enum` | Reset interval: `MONTHLY` / `DAILY` / `WEEKLY` / `YEARLY` / `NEVER` |
| `spec.startTimestamp` | `*string` | When monitoring begins (use `IMMEDIATELY` for now) |
| `spec.endTimestamp` | `*string` | When the monitor suspends assigned warehouses |
| `spec.notifyUsers` | `[]string` | Users to receive email notifications |
| `spec.triggers` | `[]Trigger` | Threshold triggers with actions (`SUSPEND` / `SUSPEND_IMMEDIATE` / `NOTIFY`) |

> 📈 **Cost Management:** Resource monitors prevent runaway credit usage by suspending warehouses or sending notifications when credit thresholds are reached.

</details>

<details>
<summary>🎭 <strong>MaskingPolicy</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Masking policy name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.signature` | `[]Argument` | Column arguments (name + type) *(immutable)* |
| `spec.body` | `string` | SQL expression that transforms the data |
| `spec.exemptOtherPolicies` | `*bool` | Whether other policies can reference a masked column |
| `spec.comment` | `*string` | Optional description |

> 🎭 **PII/PCI Compliance:** Masking policies dynamically mask sensitive data at query time based on the executing role.

</details>

<details>
<summary>🔐 <strong>RowAccessPolicy</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Row access policy name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.signature` | `[]Argument` | Row arguments (name + type) *(immutable)* |
| `spec.body` | `string` | SQL expression returning BOOLEAN for row visibility |
| `spec.comment` | `*string` | Optional description |

> 🔐 **Row-Level Security:** Row access policies filter rows at query time based on the executing role, enabling multi-tenant data isolation.

</details>

<details>
<summary>🔑 <strong>GrantOwnership</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.objectType` | `string` | Object type (e.g. DATABASE, TABLE, SCHEMA) *(immutable)* |
| `spec.objectName` | `string` | Fully qualified object name *(immutable)* |
| `spec.accountRole` | `string` | Target account role *(immutable, mutually exclusive with refs)* |
| `spec.accountRoleRef` | `ObjectReference` | AccountRole CR reference *(immutable)* |
| `spec.databaseRole` | `string` | Target database role *(immutable, mutually exclusive with refs)* |
| `spec.databaseRoleRef` | `ObjectReference` | DatabaseRole CR reference *(immutable)* |
| `spec.currentGrantsBehavior` | `*enum` | `COPY` / `REVOKE` — how existing privileges are handled |

> ⚠️ **Ownership Immutability:** All spec fields are immutable after creation. Ownership cannot be revoked — deleting the CR leaves ownership intact (no-op on delete).

</details>

---

## Data Sharing

<details>
<summary>🤝 <strong>Share</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Share name *(immutable)* |
| `spec.comment` | `*string` | Optional description |
| `spec.accounts` | `[]string` | Consumer accounts with access (fully qualified: `ORG.ACCOUNT`) |

> 🤝 **Outbound Sharing:** Shares expose Snowflake databases/schemas to consumer accounts in other Snowflake organizations. The controller manages the account list via `ALTER SHARE ... ADD ACCOUNTS` / `REMOVE ACCOUNTS` and detects drift by comparing the observed `To` field from `SHOW SHARES` output. Account identifiers are normalized to uppercase for comparison.

</details>

---

## UDFs & Stored Procedures

<details>
<summary>☕ <strong>FunctionJava</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Function name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.arguments` | `[]CallableArgument` | Function arguments (name, type, defaultValue) *(immutable)* |
| `spec.returns` | `string` | Return type *(immutable)* |
| `spec.handler` | `string` | Fully qualified Java handler method |
| `spec.runtimeVersion` | `string` | Java runtime version (e.g. `"11"`, `"17"`) |
| `spec.snowparkPackage` | `string` | Snowpark package spec |
| `spec.body` | `*string` | Inline Java source code |
| `spec.packages` | `[]string` | Additional packages |
| `spec.imports` | `[]string` | Stage file paths to import |
| `spec.targetPath` | `*string` | Stage location for compiled artifacts |
| `spec.externalAccessIntegrations` | `[]string` | External access integration names |
| `spec.secrets` | `[]SecretBinding` | Snowflake secret bindings (`secretName` + `variableName`) |
| `spec.nullInputBehavior` | `*enum` | `CALLED ON NULL INPUT` / `RETURNS NULL ON NULL INPUT` / `STRICT` |
| `spec.volatility` | `*enum` | `VOLATILE` / `IMMUTABLE` |
| `spec.secure` | `bool` | Mark as secure function |
| `spec.comment` | `*string` | Optional description |

</details>

<details>
<summary>📜 <strong>FunctionJavascript</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Function name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.arguments` | `[]CallableArgument` | Function arguments (name, type, defaultValue) *(immutable)* |
| `spec.returns` | `string` | Return type *(immutable)* |
| `spec.body` | `string` | JavaScript function body (AS clause) |
| `spec.nullInputBehavior` | `*enum` | `CALLED ON NULL INPUT` / `RETURNS NULL ON NULL INPUT` / `STRICT` |
| `spec.volatility` | `*enum` | `VOLATILE` / `IMMUTABLE` |
| `spec.secure` | `bool` | Mark as secure function |
| `spec.comment` | `*string` | Optional description |

</details>

<details>
<summary>🐍 <strong>FunctionPython</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Function name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.arguments` | `[]CallableArgument` | Function arguments (name, type, defaultValue) *(immutable)* |
| `spec.returns` | `string` | Return type *(immutable)* |
| `spec.handler` | `string` | Python handler function |
| `spec.runtimeVersion` | `string` | Python runtime version (e.g. `"3.8"`, `"3.11"`) |
| `spec.snowparkPackage` | `string` | Snowpark package spec |
| `spec.body` | `*string` | Inline Python source code |
| `spec.packages` | `[]string` | Additional packages (e.g. `"numpy"`) |
| `spec.imports` | `[]string` | Stage file paths to import |
| `spec.externalAccessIntegrations` | `[]string` | External access integration names |
| `spec.secrets` | `[]SecretBinding` | Snowflake secret bindings (`secretName` + `variableName`) |
| `spec.nullInputBehavior` | `*enum` | `CALLED ON NULL INPUT` / `RETURNS NULL ON NULL INPUT` / `STRICT` |
| `spec.volatility` | `*enum` | `VOLATILE` / `IMMUTABLE` |
| `spec.secure` | `bool` | Mark as secure function |
| `spec.comment` | `*string` | Optional description |

</details>

<details>
<summary>⚙️ <strong>FunctionScala</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Function name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.arguments` | `[]CallableArgument` | Function arguments (name, type, defaultValue) *(immutable)* |
| `spec.returns` | `string` | Return type *(immutable)* |
| `spec.handler` | `string` | Fully qualified Scala handler method |
| `spec.runtimeVersion` | `string` | Scala runtime version (e.g. `"2.12"`) |
| `spec.snowparkPackage` | `string` | Snowpark package spec |
| `spec.body` | `*string` | Inline Scala source code |
| `spec.packages` | `[]string` | Additional packages |
| `spec.imports` | `[]string` | Stage file paths to import |
| `spec.targetPath` | `*string` | Stage location for compiled artifacts |
| `spec.externalAccessIntegrations` | `[]string` | External access integration names |
| `spec.secrets` | `[]SecretBinding` | Snowflake secret bindings (`secretName` + `variableName`) |
| `spec.nullInputBehavior` | `*enum` | `CALLED ON NULL INPUT` / `RETURNS NULL ON NULL INPUT` / `STRICT` |
| `spec.volatility` | `*enum` | `VOLATILE` / `IMMUTABLE` |
| `spec.secure` | `bool` | Mark as secure function |
| `spec.comment` | `*string` | Optional description |

</details>

<details>
<summary>🗃️ <strong>FunctionSQL</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Function name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.arguments` | `[]CallableArgument` | Function arguments (name, type, defaultValue) *(immutable)* |
| `spec.returns` | `string` | Return type *(immutable)* |
| `spec.body` | `string` | SQL function body (AS clause) |
| `spec.nullInputBehavior` | `*enum` | `CALLED ON NULL INPUT` / `RETURNS NULL ON NULL INPUT` / `STRICT` |
| `spec.volatility` | `*enum` | `VOLATILE` / `IMMUTABLE` |
| `spec.secure` | `bool` | Mark as secure function |
| `spec.comment` | `*string` | Optional description |

</details>

> 💡 **Function Language Support:** Functions are split by handler language. Java and Scala support `targetPath` for compiled artifacts. Python supports `packages` for PyPI dependencies. JavaScript and SQL embed the body directly. All languages support `nullInputBehavior`, `volatility`, `secure`, and `comment`.

> ⚠️ **Security:** The `body` field is embedded into SQL statements. Ensure RBAC restricts Function CR access to trusted principals.

<details>
<summary>☕ <strong>ProcedureJava</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Procedure name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.arguments` | `[]CallableArgument` | Procedure arguments (name, type, defaultValue) *(immutable)* |
| `spec.returns` | `string` | Return type *(immutable)* |
| `spec.handler` | `string` | Fully qualified Java handler method |
| `spec.runtimeVersion` | `string` | Java runtime version (e.g. `"11"`, `"17"`) |
| `spec.snowparkPackage` | `string` | Snowpark package spec |
| `spec.body` | `*string` | Inline Java source code |
| `spec.packages` | `[]string` | Additional packages |
| `spec.imports` | `[]string` | Stage file paths to import |
| `spec.targetPath` | `*string` | Stage location for compiled artifacts |
| `spec.externalAccessIntegrations` | `[]string` | External access integration names |
| `spec.secrets` | `[]SecretBinding` | Snowflake secret bindings |
| `spec.executeAs` | `*enum` | `OWNER` / `CALLER` |
| `spec.nullInputBehavior` | `*enum` | `CALLED ON NULL INPUT` / `RETURNS NULL ON NULL INPUT` / `STRICT` |
| `spec.secure` | `bool` | Mark as secure procedure |
| `spec.comment` | `*string` | Optional description |

</details>

<details>
<summary>📜 <strong>ProcedureJavascript</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Procedure name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.arguments` | `[]CallableArgument` | Procedure arguments (name, type, defaultValue) *(immutable)* |
| `spec.returns` | `string` | Return type *(immutable)* |
| `spec.body` | `string` | JavaScript procedure body (AS clause) |
| `spec.executeAs` | `*enum` | `OWNER` / `CALLER` |
| `spec.nullInputBehavior` | `*enum` | `CALLED ON NULL INPUT` / `RETURNS NULL ON NULL INPUT` / `STRICT` |
| `spec.secure` | `bool` | Mark as secure procedure |
| `spec.comment` | `*string` | Optional description |

</details>

<details>
<summary>🐍 <strong>ProcedurePython</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Procedure name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.arguments` | `[]CallableArgument` | Procedure arguments (name, type, defaultValue) *(immutable)* |
| `spec.returns` | `string` | Return type *(immutable)* |
| `spec.handler` | `string` | Python handler function |
| `spec.runtimeVersion` | `string` | Python runtime version (e.g. `"3.8"`, `"3.11"`) |
| `spec.snowparkPackage` | `string` | Snowpark package spec |
| `spec.body` | `*string` | Inline Python source code |
| `spec.packages` | `[]string` | Additional packages |
| `spec.imports` | `[]string` | Stage file paths to import |
| `spec.externalAccessIntegrations` | `[]string` | External access integration names |
| `spec.secrets` | `[]SecretBinding` | Snowflake secret bindings |
| `spec.executeAs` | `*enum` | `OWNER` / `CALLER` |
| `spec.nullInputBehavior` | `*enum` | `CALLED ON NULL INPUT` / `RETURNS NULL ON NULL INPUT` / `STRICT` |
| `spec.secure` | `bool` | Mark as secure procedure |
| `spec.comment` | `*string` | Optional description |

</details>

<details>
<summary>⚙️ <strong>ProcedureScala</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Procedure name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.arguments` | `[]CallableArgument` | Procedure arguments (name, type, defaultValue) *(immutable)* |
| `spec.returns` | `string` | Return type *(immutable)* |
| `spec.handler` | `string` | Fully qualified Scala handler method |
| `spec.runtimeVersion` | `string` | Scala runtime version (e.g. `"2.12"`) |
| `spec.snowparkPackage` | `string` | Snowpark package spec |
| `spec.body` | `*string` | Inline Scala source code |
| `spec.packages` | `[]string` | Additional packages |
| `spec.imports` | `[]string` | Stage file paths to import |
| `spec.targetPath` | `*string` | Stage location for compiled artifacts |
| `spec.externalAccessIntegrations` | `[]string` | External access integration names |
| `spec.secrets` | `[]SecretBinding` | Snowflake secret bindings |
| `spec.executeAs` | `*enum` | `OWNER` / `CALLER` |
| `spec.nullInputBehavior` | `*enum` | `CALLED ON NULL INPUT` / `RETURNS NULL ON NULL INPUT` / `STRICT` |
| `spec.secure` | `bool` | Mark as secure procedure |
| `spec.comment` | `*string` | Optional description |

</details>

<details>
<summary>🗃️ <strong>ProcedureSQL</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Procedure name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.arguments` | `[]CallableArgument` | Procedure arguments (name, type, defaultValue) *(immutable)* |
| `spec.returns` | `string` | Return type *(immutable)* |
| `spec.body` | `string` | SQL procedure body (AS clause) |
| `spec.executeAs` | `*enum` | `OWNER` / `CALLER` |
| `spec.nullInputBehavior` | `*enum` | `CALLED ON NULL INPUT` / `RETURNS NULL ON NULL INPUT` / `STRICT` |
| `spec.secure` | `bool` | Mark as secure procedure |
| `spec.comment` | `*string` | Optional description |

</details>

> 💡 **Procedure vs Function:** Procedures support `executeAs` (`OWNER`/`CALLER`) but NOT `volatility`. Functions support `volatility` (`VOLATILE`/`IMMUTABLE`) but NOT `executeAs`. Both share `nullInputBehavior`, `secure`, and `comment`.

---

## External Functions

<details>
<summary>🌐 <strong>ExternalFunction</strong> (shortName: <code>extfn</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Function name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.args` | `[]ExternalFunctionArg` | Function arguments (`name`, `type`) *(immutable)* |
| `spec.returnType` | `string` | Return data type *(immutable)* |
| `spec.returnNullValues` | `*bool` | Whether the function can return NULL *(default: true)* |
| `spec.returnBehavior` | `*string` | `VOLATILE` / `IMMUTABLE` |
| `spec.apiIntegrationName` | `*string` | API integration name *(XOR with apiIntegrationRef)* |
| `spec.apiIntegrationRef.name` | `string` | APIIntegration CR reference *(XOR with apiIntegrationName)* |
| `spec.url` | `string` | HTTPS endpoint invoked by the function *(immutable)* |
| `spec.headers` | `[]ExternalFunctionHeader` | HTTP headers (`name`, `value`) sent with each request |
| `spec.maxBatchRows` | `*int32` | Maximum rows per batch request *(min: 1)* |
| `spec.compression` | `*enum` | `AUTO` / `GZIP` / `DEFLATE` / `NONE` |
| `spec.requestTranslator` | `*string` | Fully qualified request translator function name |
| `spec.responseTranslator` | `*string` | Fully qualified response translator function name |
| `spec.comment` | `*string` | Optional description |

> 🌐 **Remote Services:** External functions call external HTTPS endpoints (AWS Lambda, Azure Functions, GCP Cloud Functions) via API integrations. Use `apiIntegrationRef` to reference an APIIntegration CR and get automatic dependency tracking — the controller watches referenced APIIntegration CRs and re-reconciles when they become ready. Arguments, return type, and URL are immutable — changing them requires recreating the function (use `force-new` annotation). Argument types and return types are validated against safe SQL type patterns. The controller issues `CREATE EXTERNAL FUNCTION` and `DROP FUNCTION` (no ALTER support).

</details>

---

## API Authentication Integrations

<details>
<summary>🔐 <strong>APIAuthenticationIntegrationWithAuthorizationCodeGrant</strong> (shortName: <code>aaiwacg</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Integration name *(immutable)* |
| `spec.enabled` | `bool` | Whether the integration is enabled |
| `spec.oauthClientId` | `string` | OAuth client ID |
| `spec.oauthClientSecretRef` | `SecretKeyReference` | K8s Secret reference for OAuth client secret |
| `spec.oauthTokenEndpoint` | `*string` | Token endpoint URL |
| `spec.oauthAuthorizationEndpoint` | `*string` | Authorization endpoint URL |
| `spec.oauthClientAuthMethod` | `*enum` | `CLIENT_SECRET_BASIC` / `CLIENT_SECRET_POST` |
| `spec.oauthAccessTokenValidity` | `*int` | Access token lifetime (seconds) |
| `spec.oauthRefreshTokenValidity` | `*int` | Refresh token validity (seconds) |
| `spec.oauthAllowedScopes` | `[]string` | Allowed OAuth scopes |
| `spec.comment` | `*string` | Optional description |

> 🔐 **OAuth Authorization Code:** For interactive flows where a user grants consent. The integration stores the client credentials and endpoint configuration.

</details>

<details>
<summary>🔐 <strong>APIAuthenticationIntegrationWithClientCredentials</strong> (shortName: <code>aaiwcc</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Integration name *(immutable)* |
| `spec.enabled` | `bool` | Whether the integration is enabled |
| `spec.oauthClientId` | `string` | OAuth client ID |
| `spec.oauthClientSecretRef` | `SecretKeyReference` | K8s Secret reference for OAuth client secret |
| `spec.oauthTokenEndpoint` | `*string` | Token endpoint URL |
| `spec.oauthClientAuthMethod` | `*enum` | `CLIENT_SECRET_BASIC` / `CLIENT_SECRET_POST` |
| `spec.oauthAccessTokenValidity` | `*int` | Access token lifetime (seconds) |
| `spec.oauthAllowedScopes` | `[]string` | Allowed OAuth scopes |
| `spec.comment` | `*string` | Optional description |

> 🔐 **OAuth Client Credentials:** For service-to-service authentication without user interaction. Simpler than authorization code grant — no refresh token or authorization endpoint needed.

</details>

<details>
<summary>🔐 <strong>APIAuthenticationIntegrationWithJWTBearer</strong> (shortName: <code>aaiwjb</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Integration name *(immutable)* |
| `spec.enabled` | `bool` | Whether the integration is enabled |
| `spec.oauthClientId` | `string` | OAuth client ID |
| `spec.oauthClientSecretRef` | `SecretKeyReference` | K8s Secret reference for OAuth client secret |
| `spec.oauthAssertionIssuer` | `string` | Assertion issuer for JWT bearer flow |
| `spec.oauthTokenEndpoint` | `*string` | Token endpoint URL |
| `spec.oauthAuthorizationEndpoint` | `*string` | Authorization endpoint URL |
| `spec.oauthClientAuthMethod` | `*enum` | `CLIENT_SECRET_BASIC` / `CLIENT_SECRET_POST` |
| `spec.oauthAccessTokenValidity` | `*int` | Access token lifetime (seconds) |
| `spec.oauthRefreshTokenValidity` | `*int` | Refresh token validity (seconds) |
| `spec.comment` | `*string` | Optional description |

> 🔐 **JWT Bearer:** For server-side applications using signed JWTs for authentication. Requires `oauthAssertionIssuer` to identify the JWT issuer.

</details>

---

## Snowflake Secrets

<details>
<summary>🔒 <strong>SecretWithAuthorizationCodeGrant</strong> (shortName: <code>swacg</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Secret name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.apiAuthentication` | `string` | Security integration name *(immutable)* |
| `spec.oauthRefreshToken` | `string` | OAuth refresh token |
| `spec.oauthRefreshTokenExpiryTime` | `string` | Refresh token expiry timestamp (e.g. `'2025-01-06 20:00:00'`) |
| `spec.comment` | `*string` | Optional description |

</details>

<details>
<summary>🔒 <strong>SecretWithBasicAuthentication</strong> (shortName: <code>swba</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Secret name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.username` | `string` | Username for basic authentication |
| `spec.password` | `string` | Password for basic authentication |
| `spec.comment` | `*string` | Optional description |

</details>

<details>
<summary>🔒 <strong>SecretWithClientCredentials</strong> (shortName: <code>swcc</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Secret name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.apiAuthentication` | `string` | Security integration name *(immutable)* |
| `spec.oauthScopes` | `[]string` | OAuth scopes (min 1) |
| `spec.comment` | `*string` | Optional description |

</details>

<details>
<summary>🔒 <strong>SecretWithGenericString</strong> (shortName: <code>swgs</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Secret name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.secretString` | `string` | The generic string value (e.g. API token) |
| `spec.comment` | `*string` | Optional description |

</details>

> 🔒 **Snowflake Secrets:** Secrets are Snowflake-managed credential objects used by external access integrations and UDFs. They are NOT Kubernetes Secrets — they store credentials inside Snowflake for use by functions and procedures. Each variant maps to a different `TYPE` in `CREATE SECRET`.

---

## Snowpark Container Services

<details>
<summary>💻 <strong>ComputePool</strong> (shortName: <code>cp</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Compute pool name *(immutable)* |
| `spec.minNodes` | `int32` | Minimum number of nodes *(min: 1)* |
| `spec.maxNodes` | `int32` | Maximum number of nodes *(min: 1)* |
| `spec.instanceFamily` | `string` | Compute instance type (e.g. `CPU_X64_XS`, `GPU_NV_S`) *(immutable)* |
| `spec.autoResume` | `*bool` | Auto-resume when a service/job is submitted |
| `spec.autoSuspendSecs` | `*int32` | Idle seconds before auto-suspend *(min: 0)* |
| `spec.comment` | `*string` | Optional description |

> 💻 **Compute Infrastructure:** Compute pools provide the underlying compute for Snowpark Container Services. `instanceFamily` is immutable — changing it requires recreating the pool (use `force-new` annotation). The controller tracks `state` in `status.showOutput.state` and prints it via `kubectl`.

</details>

<details>
<summary>🚀 <strong>Service</strong> (shortName: <code>svc</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Service name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.computePoolName` | `*string` | Compute pool name *(immutable, XOR with computePoolRef)* |
| `spec.computePoolRef.name` | `string` | ComputePool CR reference *(immutable, XOR with computePoolName)* |
| `spec.specification` | `string` | Inline YAML/JSON service spec *(max: 64KB, XOR with specificationReference)* |
| `spec.specificationReference` | `string` | Stage path to spec file (e.g. `@db.schema.stage/spec.yaml`) *(validated, XOR with specification)* |
| `spec.minInstances` | `*int32` | Minimum compute instances *(min: 0)* |
| `spec.maxInstances` | `*int32` | Maximum compute instances *(min: 1)* |
| `spec.autoResume` | `*bool` | Auto-resume the service |
| `spec.externalAccessIntegrations` | `[]string` | Security integrations granting egress |
| `spec.comment` | `*string` | Optional description |

> 🚀 **Container Workloads:** Services run containerized applications on Snowpark compute pools. Specify containers inline (`specification`) or reference a stage file (`specificationReference` — validated against `@identifier/path` format). The compute pool is immutable — changing it requires recreating the service (use `force-new` annotation). The controller watches referenced ComputePool CRs and automatically re-reconciles when they become ready.

</details>

<details>
<summary>📦 <strong>ImageRepository</strong> (shortName: <code>imgrepo</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Image repository name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |

> 📦 **Container Images:** Image repositories store OCI container images for Snowpark Container Services. The repository URL is available in `status.showOutput.repositoryUrl` — use it to push images via `docker push`. All spec fields are immutable — the controller only supports CREATE and DROP (no ALTER).

</details>

> 💡 **SPCS Stack:** A typical Snowpark Container Services deployment requires: (1) a ComputePool for compute resources, (2) an ImageRepository for container images, and (3) a Service to run the workload. Use `computePoolRef` in the Service spec to reference a ComputePool CR and get automatic dependency tracking.

---

## Additional Schema Objects

<details>
<summary>🔢 <strong>Sequence</strong> (shortName: <code>seq</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Sequence name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.start` | `*int64` | Initial value *(immutable, default: 1)* |
| `spec.increment` | `*int64` | Step interval *(default: 1)* |
| `spec.ordering` | `*enum` | `ORDER` / `NOORDER` — note: `NOORDER→ORDER` not allowed by Snowflake |
| `spec.comment` | `*string` | Optional description |

> 🔢 **Auto-Incrementing:** Sequences generate unique numbers for use as primary keys or surrogate keys. Use `ORDER` for guaranteed ordering (slower) or `NOORDER` for higher throughput.

</details>

<details>
<summary>📋 <strong>ExternalTable</strong> (shortName: <code>exttbl</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | External table name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable, XOR with databaseName)* |
| `spec.databaseName` | `string` | Inline database name *(immutable, XOR with databaseRef)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable, XOR with schemaName)* |
| `spec.schemaName` | `string` | Inline schema name *(immutable, XOR with schemaRef)* |
| `spec.location` | `string` | External stage + path (e.g. `@MYDB.MYSCHEMA.MYSTAGE/path/`) *(immutable)* |
| `spec.fileFormat` | `string` | File format spec (e.g. `TYPE = PARQUET`) *(immutable)* |
| `spec.columns` | `[]ExternalTableColumn` | Virtual column definitions: `name`, `type`, `as` (SQL expression) *(immutable)* |
| `spec.partitionBy` | `[]string` | Partition column names *(immutable)* |
| `spec.partitionType` | `*string` | Partition type: `USER_SPECIFIED` *(immutable)* |
| `spec.pattern` | `*string` | Regex for matching filenames *(immutable)* |
| `spec.refreshOnCreate` | `*bool` | Auto-refresh on creation *(immutable)* |
| `spec.autoRefresh` | `*bool` | Auto-refresh on new data *(only mutable field)* |
| `spec.awsSnsTopic` | `*string` | SNS topic ARN for S3 auto-refresh *(immutable)* |
| `spec.tableFormat` | `*string` | Table format: `DELTA` *(immutable)* |
| `spec.integration` | `*string` | Notification integration for GCS/Azure auto-refresh *(immutable)* |
| `spec.comment` | `*string` | Optional description *(immutable)* |

> 📋 **External Data:** External tables provide read-only access to data stored in cloud storage. Most fields are immutable — only `autoRefresh` can be altered after creation.

</details>

<details>
<summary>🔗 <strong>TableConstraint</strong> (shortName: <code>tc</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Constraint name *(immutable)* |
| `spec.type` | `enum` | `PRIMARY KEY` / `UNIQUE` / `FOREIGN KEY` *(immutable)* |
| `spec.tableName` | `string` | Fully qualified table name *(immutable)* |
| `spec.columns` | `[]string` | Column names (min 1) *(immutable)* |
| `spec.foreignKeyProperties` | `*ForeignKeyProperties` | Required when type=`FOREIGN KEY` *(immutable)* |
| `spec.foreignKeyProperties.referencesTableName` | `string` | Fully qualified referenced table |
| `spec.foreignKeyProperties.referencesColumns` | `[]string` | Referenced columns (min 1) |
| `spec.foreignKeyProperties.match` | `*enum` | `FULL` / `PARTIAL` / `SIMPLE` |
| `spec.foreignKeyProperties.onUpdate` | `*enum` | `CASCADE` / `SET NULL` / `SET DEFAULT` / `RESTRICT` / `NO ACTION` |
| `spec.foreignKeyProperties.onDelete` | `*enum` | `CASCADE` / `SET NULL` / `SET DEFAULT` / `RESTRICT` / `NO ACTION` |
| `spec.properties` | `*ConstraintProperties` | Optional mutable constraint properties |
| `spec.properties.enforced` | `*bool` | Whether constraint is enforced |
| `spec.properties.deferrable` | `*bool` | Whether constraint is deferrable |
| `spec.properties.initially` | `*enum` | `DEFERRED` / `IMMEDIATE` |
| `spec.properties.rely` | `*bool` | Constraint used in query optimization |
| `spec.properties.validate` | `*bool` | Validate existing data on creation |
| `spec.comment` | `*string` | Optional description |

> 🔗 **Referential Integrity:** TableConstraint manages standalone table constraints (PRIMARY KEY, UNIQUE, FOREIGN KEY) as independent CRs. Use instead of inline `spec.constraints` in the Table CR for more granular lifecycle control.

</details>

---

## Policy & Tag Attachments

<details>
<summary>🏷️ <strong>TagAssociation</strong> (shortName: <code>ta</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.tagName` | `string` | Fully qualified tag name *(immutable, XOR with tagRef)* |
| `spec.tagRef.name` | `string` | Tag CR reference *(immutable, XOR with tagName)* |
| `spec.tagValue` | `string` | Tag value to assign *(mutable)* |
| `spec.objectType` | `enum` | Target object type *(immutable)* — `ACCOUNT` / `DATABASE` / `SCHEMA` / `TABLE` / `VIEW` / `COLUMN` / `WAREHOUSE` / `ROLE` / `USER` / `STAGE` / `STREAM` / `TASK` / `ALERT` / `PIPE` / `FUNCTION` / `PROCEDURE` / `INTEGRATION` / `NETWORK POLICY` / `DATABASE ROLE` |
| `spec.objectName` | `string` | Fully qualified Snowflake object name *(immutable)* |

> 🏷️ **Metadata Tagging:** TagAssociation applies a tag with a value to any Snowflake object. The `tagValue` is the only mutable field — changing the value issues `ALTER ... SET TAG`.

</details>

<details>
<summary>🎭 <strong>MaskingPolicyApplication</strong> (shortName: <code>mpa</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.policyName` | `string` | Fully qualified masking policy name *(immutable, XOR with policyRef)* |
| `spec.policyRef.name` | `string` | MaskingPolicy CR reference *(immutable, XOR with policyName)* |
| `spec.tableName` | `string` | Fully qualified table name *(immutable)* |
| `spec.columnName` | `string` | Column to apply policy to *(immutable)* |
| `spec.usingColumns` | `[]string` | Conditional masking policy columns *(immutable)* |

> 🎭 **Column-Level Masking:** Applies a masking policy to a specific column. All fields are immutable — to change the column or policy, delete and recreate the CR.

</details>

<details>
<summary>🛡️ <strong>NetworkPolicyAttachment</strong> (shortName: <code>npa</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.policyName` | `string` | Snowflake network policy name *(immutable, XOR with policyRef)* |
| `spec.policyRef.name` | `string` | NetworkPolicy CR reference *(immutable, XOR with policyName)* |
| `spec.targetType` | `enum` | `ACCOUNT` / `USER` *(immutable)* |
| `spec.targetName` | `string` | User name — required when targetType=`USER` *(immutable)* |

> 🛡️ **Policy Attachment:** Attaches a network policy to an account or specific user. Account-level policies affect all users; user-level policies override account-level.

</details>

<details>
<summary>🔒 <strong>PasswordPolicyAttachment</strong> (shortName: <code>ppa</code>)</summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.policyName` | `string` | Fully qualified password policy name *(immutable, XOR with policyRef)* |
| `spec.policyRef.name` | `string` | PasswordPolicy CR reference *(immutable, XOR with policyName)* |
| `spec.targetType` | `enum` | `ACCOUNT` / `USER` *(immutable)* |
| `spec.targetName` | `string` | User name — required when targetType=`USER` *(immutable)* |

> 🔒 **Policy Attachment:** Attaches a password policy to an account or specific user. Account-level policies apply to all users who don't have a user-level override.

</details>
