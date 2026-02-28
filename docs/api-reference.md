---
title: API Reference
nav_order: 2
---

# 📖 API Reference

Complete field-level documentation for all Snowplane CRDs. Each resource supports full lifecycle management (create, alter, drop), drift detection, adoption of pre-existing objects, and deletion policies.

> 💡 **Nil-means-unmanaged convention:** Pointer fields (`*string`, `*int32`, `*bool`) use `nil` to mean "not managed by Snowplane." When nil, the controller skips the parameter in CREATE/ALTER, leaving Snowflake's server-side default intact.

> 🏷️ **Common fields:** Every managed resource (except ProviderConfig and FieldExport) includes `spec.providerRef.name` (ProviderConfig reference), `spec.deletionPolicy` (`Delete` / `Orphan`), and `spec.useRole` (optional Snowflake role for `USE ROLE`).

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
| `spec.defaultDdlCollation` | `*string` | Default string column collation |
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
<summary>📂 <strong>Schema</strong></summary>

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
<summary>🔑 <strong>AccountRoleGrant</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.privilege` | `string` | Snowflake privilege (e.g. USAGE, SELECT) *(immutable)* |
| `spec.on` | `GrantOn` | Grant target — exactly one of `account`, `accountObject`, `schema`, `schemaObject` *(immutable)* |
| `spec.accountRole` | `string` | Account role name *(immutable, mutually exclusive with accountRoleRef)* |
| `spec.accountRoleRef` | `LocalObjectReference` | AccountRole CR reference *(immutable, mutually exclusive with accountRole)* |
| `spec.withGrantOption` | `bool` | Allow grantee to re-grant *(immutable)* |

> ⚠️ **Grant Immutability:** All spec fields are immutable after creation. Changing any field requires deleting and recreating the CR (or using the `force-new` annotation).

</details>

<details>
<summary>🔑 <strong>DatabaseRoleGrant</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.privilege` | `string` | Snowflake privilege (e.g. USAGE, SELECT) *(immutable)* |
| `spec.on` | `GrantOn` | Grant target — exactly one of `account`, `accountObject`, `schema`, `schemaObject` *(immutable)* |
| `spec.databaseRole` | `string` | Fully qualified database role name *(immutable, mutually exclusive with databaseRoleRef)* |
| `spec.databaseRoleRef` | `LocalObjectReference` | DatabaseRole CR reference *(immutable, mutually exclusive with databaseRole)* |
| `spec.withGrantOption` | `bool` | Allow grantee to re-grant *(immutable)* |

> ⚠️ **Grant Immutability:** All spec fields are immutable after creation. Changing any field requires deleting and recreating the CR (or using the `force-new` annotation).

</details>

<details>
<summary>🔑 <strong>AccountRoleAssignment</strong> (shortName: <code>ara</code>)</summary>

Assigns an account role to another role or user: `GRANT ROLE <role> TO ROLE|USER <target>`.

| Field | Type | Description |
|-------|------|-------------|
| `spec.roleName` | `string` | Account role to assign *(immutable, mutually exclusive with roleRef)* |
| `spec.roleRef` | `LocalObjectReference` | AccountRole CR reference *(immutable, mutually exclusive with roleName)* |
| `spec.toRole` | `string` | Target account role *(immutable, mutually exclusive with toRoleRef, toUser, toUserRef)* |
| `spec.toRoleRef` | `LocalObjectReference` | AccountRole CR reference for the target *(immutable)* |
| `spec.toUser` | `string` | Target user *(immutable, mutually exclusive with toUserRef, toRole, toRoleRef)* |
| `spec.toUserRef` | `LocalObjectReference` | User CR reference for the target *(immutable)* |

> ⚠️ **Assignment Immutability:** All spec fields are immutable after creation. Changing any field requires deleting and recreating the CR (or using the `force-new` annotation).

</details>

<details>
<summary>🔑 <strong>DatabaseRoleAssignment</strong> (shortName: <code>dra</code>)</summary>

Assigns a database role to an account role or another database role: `GRANT DATABASE ROLE <db>.<role> TO ROLE|DATABASE ROLE <target>`.

| Field | Type | Description |
|-------|------|-------------|
| `spec.databaseRoleName` | `string` | Fully qualified database role *(immutable, mutually exclusive with databaseRoleRef)* |
| `spec.databaseRoleRef` | `LocalObjectReference` | DatabaseRole CR reference *(immutable, mutually exclusive with databaseRoleName)* |
| `spec.toRole` | `string` | Target account role *(immutable, mutually exclusive with toRoleRef, toDatabaseRole, toDatabaseRoleRef)* |
| `spec.toRoleRef` | `LocalObjectReference` | AccountRole CR reference for the target *(immutable)* |
| `spec.toDatabaseRole` | `string` | Fully qualified target database role *(immutable, mutually exclusive with toDatabaseRoleRef, toRole, toRoleRef)* |
| `spec.toDatabaseRoleRef` | `LocalObjectReference` | DatabaseRole CR reference for the target *(immutable)* |

> ⚠️ **Assignment Immutability:** All spec fields are immutable after creation. Changing any field requires deleting and recreating the CR (or using the `force-new` annotation).

</details>

<details>
<summary>🔑 <strong>ShareGrant</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.privilege` | `string` | Snowflake privilege (e.g. USAGE, SELECT) *(immutable)* |
| `spec.objectType` | `string` | Object type (e.g. DATABASE, SCHEMA, TABLE, VIEW) *(immutable)* |
| `spec.objectName` | `string` | Fully qualified object name *(immutable)* |
| `spec.share` | `string` | Share name *(immutable)* |

> ⚠️ **Grant Immutability:** All spec fields are immutable after creation. Shares do not support WITH GRANT OPTION.

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
| `spec.defaultDdlCollation` | `*string` | Default DDL collation |
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
<summary>📦 <strong>Stage</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Stage name *(immutable)* |
| `spec.databaseRef.name` | `string` | Database CR reference *(immutable)* |
| `spec.schemaRef.name` | `string` | Schema CR reference *(immutable)* |
| `spec.url` | `*string` | External stage URL |
| `spec.storageIntegration` | `*string` | Storage integration name (requires `url`) |
| `spec.encryption` | `*StageEncryption` | Encryption settings |
| `spec.directory` | `*StageDirectoryOptions` | Directory table settings |
| `spec.fileFormat` | `*string` | File format |

</details>

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
| `spec.warehouse` | `*string` | Warehouse for task execution *(mutually exclusive with serverless size)* |
| `spec.userTaskManagedInitialWarehouseSize` | `*enum` | Serverless warehouse size: `XSMALL`…`XXLARGE` |
| `spec.after` | `[]string` | Predecessor task names for DAG scheduling |
| `spec.when` | `*string` | Boolean SQL condition for conditional execution |
| `spec.suspend` | `*bool` | Whether the task is suspended (default: `true`) |
| `spec.comment` | `*string` | Optional description |
| `spec.allowOverlappingExecution` | `*bool` | Allow concurrent graph executions |
| `spec.userTaskTimeoutMs` | `*int32` | Single-run timeout in milliseconds (0–604800000) |
| `spec.suspendTaskAfterNumFailures` | `*int32` | Auto-suspend after N consecutive failures |
| `spec.errorIntegration` | `*string` | Notification integration for errors |
| `spec.taskAutoRetryAttempts` | `*int32` | Automatic retry attempts (0–30) |

> ⏰ **DAG Scheduling:** Use `after` to chain tasks into directed acyclic graphs. Root tasks require a `schedule`; child tasks inherit it from the root.

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
| `spec.warehouse` | `*string` | Warehouse for alert execution — omit for serverless |
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
| `spec.table` | `string` | Fully qualified source table name *(immutable)* |
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
| `spec.view` | `string` | Fully qualified source view name *(immutable)* |
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
| `spec.externalTable` | `string` | Fully qualified source external table name *(immutable)* |
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
| `spec.stage` | `string` | Fully qualified source stage name *(immutable)* |
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
| `spec.dynamicTable` | `string` | Fully qualified source dynamic table name *(immutable)* |
| `spec.appendOnly` | `*bool` | Track row inserts only |
| `spec.showInitialRows` | `*bool` | Include existing rows on first consume |
| `spec.comment` | `*string` | Optional description |

> 🔄 **Change Data Capture:** Tracks DML changes on a dynamic table. Use with Tasks to build real-time data pipelines.

</details>

<details>
<summary>🔌 <strong>StorageIntegration</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Storage integration name *(immutable)* |
| `spec.type` | `enum` | Integration type: `EXTERNAL_STAGE` *(immutable, default: EXTERNAL_STAGE)* |
| `spec.enabled` | `*bool` | Whether the integration is active *(default: true)* |
| `spec.storageProvider` | `enum` | Cloud provider: `S3` / `GCS` / `AZURE` *(immutable)* |
| `spec.storageAllowedLocations` | `[]string` | Cloud storage URIs the integration can access *(min 1)* |
| `spec.storageBlockedLocations` | `[]string` | Cloud storage URIs explicitly denied access |
| `spec.storageAWSRoleARN` | `*string` | IAM role ARN Snowflake assumes for S3 *(required when S3)* |
| `spec.storageAWSExternalID` | `*string` | Optional external ID for AWS trust relationship *(auto-generated if omitted)* |
| `spec.azureTenantID` | `*string` | Azure AD tenant ID *(required when AZURE)* |
| `spec.comment` | `*string` | Optional description |
| `status.storageAWSIAMUserARN` | `string` | Snowflake IAM user ARN — needed for IAM trust policy |
| `status.storageAWSExternalID` | `string` | External ID currently in use (from DESCRIBE) |

> 🔌 **Cloud Integration:** Storage integrations configure secure access between Snowflake and cloud storage (S3, GCS, Azure Blob). Use `status.storageAWSIAMUserARN` to configure your IAM trust policy.

</details>

<details><summary>🔔 <strong>NotificationIntegration</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Notification integration name *(immutable)* |
| `spec.type` | `enum` | Integration type: `EMAIL` / `QUEUE` / `WEBHOOK` *(immutable)* |
| `spec.enabled` | `*bool` | Whether the integration is active *(default: true)* |
| `spec.email.allowedRecipients` | `[]string` | Email addresses that can receive notifications *(required for EMAIL, min 1)* |
| `spec.email.defaultRecipients` | `[]string` | Default email recipients |
| `spec.email.defaultSubject` | `*string` | Default email subject line |
| `spec.queue.notificationProvider` | `enum` | Cloud provider: `AWS_SNS` / `AWS_SQS` / `GCP_PUBSUB` / `AZURE_STORAGE_QUEUE` / `AZURE_EVENT_GRID` *(required for QUEUE)* |
| `spec.queue.direction` | `enum` | Message direction: `OUTBOUND` / `INBOUND` *(default: OUTBOUND)* |
| `spec.queue.awsSNSTopicARN` | `*string` | ARN of the SNS topic (AWS_SNS) |
| `spec.queue.awsSNSRoleARN` | `*string` | IAM role ARN for SNS access (AWS_SNS) |
| `spec.queue.awsSQSArn` | `*string` | ARN of the SQS queue (AWS_SQS) |
| `spec.queue.awsSQSRoleARN` | `*string` | IAM role ARN for SQS access (AWS_SQS) |
| `spec.queue.gcpPubSubTopicName` | `*string` | Pub/Sub topic name (GCP_PUBSUB) |
| `spec.queue.gcpPubSubSubscriptionName` | `*string` | Pub/Sub subscription name (GCP_PUBSUB) |
| `spec.queue.azureStorageQueuePrimaryURI` | `*string` | Azure Storage queue endpoint (AZURE_STORAGE_QUEUE) |
| `spec.queue.azureTenantID` | `*string` | Azure AD tenant ID (Azure providers) |
| `spec.queue.azureEventGridTopicEndpoint` | `*string` | Event Grid topic endpoint (AZURE_EVENT_GRID) |
| `spec.webhook.webhookURL` | `string` | Endpoint URL for the webhook *(required for WEBHOOK)* |
| `spec.webhook.webhookSecret` | `*string` | Secret used to sign webhook payloads |
| `spec.webhook.webhookBodyTemplate` | `*string` | Custom body template for webhook payload |
| `spec.webhook.webhookHeaders` | `map[string]string` | Custom HTTP headers for webhook requests |
| `spec.comment` | `*string` | Optional description |
| `status.describeOutput` | `map[string]string` | Key-value pairs from DESCRIBE INTEGRATION |

> 🔔 **Alerting & Events:** Notification integrations deliver alerts and events to email, cloud messaging (SNS, SQS, Pub/Sub, Azure), or webhook endpoints. Each type requires its own sub-config — CEL validation enforces this at admission time.

</details>

<details><summary>� <strong>SecurityIntegration</strong></summary>

| Field | Type | Description |
|-------|------|-------------|
| `spec.name` | `string` | Security integration name *(immutable)* |
| `spec.type` | `enum` | Integration type: `EXTERNAL_OAUTH` / `SAML2` / `SCIM` / `API_AUTHENTICATION` *(immutable)* |
| `spec.enabled` | `*bool` | Whether the integration is active |
| `spec.externalOAuth` | `*ExternalOAuthConfig` | External OAuth config (when type=EXTERNAL_OAUTH) |
| `spec.saml2` | `*SAML2Config` | SAML2 SSO config (when type=SAML2) |
| `spec.scim` | `*SCIMConfig` | SCIM provisioning config (when type=SCIM) |
| `spec.apiAuthentication` | `*APIAuthenticationConfig` | API Authentication config (when type=API_AUTHENTICATION) |
| `spec.comment` | `*string` | Optional description |
| `status.describeOutput` | `map[string]string` | Key-value pairs from DESCRIBE INTEGRATION |

> 🔐 **SSO & Identity:** Security integrations configure federated authentication (SAML2, OAuth), automated user provisioning (SCIM), and programmatic API access. Each type has its own sub-config.

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
| `spec.integration` | `*string` | Notification integration for auto-ingest *(required when autoIngest=true, immutable)* |
| `spec.awsSnsTopic` | `*string` | Amazon SNS topic ARN for S3 auto-ingest *(immutable)* |
| `spec.errorIntegration` | `*string` | Notification integration for error notifications |
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
| `spec.warehouse` | `string` | Warehouse used for refreshing |
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
<summary>🔒 <strong>PasswordPolicy</strong></summary>

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
| `spec.accountRoleRef` | `LocalObjectReference` | AccountRole CR reference *(immutable)* |
| `spec.databaseRole` | `string` | Target database role *(immutable, mutually exclusive with refs)* |
| `spec.databaseRoleRef` | `LocalObjectReference` | DatabaseRole CR reference *(immutable)* |
| `spec.currentGrantsBehavior` | `*enum` | `COPY` / `REVOKE` — how existing privileges are handled |

> ⚠️ **Ownership Immutability:** All spec fields are immutable after creation. Ownership cannot be revoked — deleting the CR leaves ownership intact (no-op on delete).

</details>
