# Drift Detection

Snowplane includes a field-level drift detection engine that automatically detects and optionally corrects out-of-band changes to Snowflake resources.

## How It Works

Every reconciliation cycle (default: every 5 minutes), each controller:

1. **Observes** the current state of the Snowflake resource via `SHOW` and `SHOW PARAMETERS` commands
2. **Compares** each managed field in the spec against the observed state using the `drift.Detector`
3. **Reports** any differences as structured `FieldChange` entries with field name, desired value, and actual value
4. **Sets** the `DriftDetected` condition with a summary of all drifted fields
5. **Emits** a Kubernetes `DriftDetected` event on the resource
6. **Corrects** the drift by issuing an `ALTER` statement with the correct values
7. **Clears** the `DriftDetected` condition and emits a `DriftCorrected` event after successful correction

## Supported Resources

Drift detection is supported on all resource types:

| Resource | Drift Detection | Detect-Only | Immutable Fields Tracked |
|----------|----------------|-------------|--------------------------|
| Database | ✅ | ✅ | `name`, `transient` |
| Schema   | ✅ | ✅ | `name`, `database`, `transient` |
| Warehouse | ✅ | ✅ | `name` |
| User     | ✅ | ✅ | `name`, `type` |
| AccountRole | ✅ | ✅ | `name` |
| DatabaseRole | ✅ | ✅ | `name`, `database` |
| Table    | ✅ | ✅ | `name`, `database`, `schema`, `transient` |
| View     | ✅ | ✅ | `name`, `database`, `schema` |
| Stage    | ✅ | ✅ | `name`, `database`, `schema`, `stageType` |
| Task     | ✅ | ✅ | `name`, `database`, `schema` |
| Stream   | ✅ | ✅ | `name`, `database`, `schema`, `sourceType`, `sourceName` |
| Tag      | ✅ | ✅ | `name`, `database`, `schema` |
| NetworkPolicy | ✅ | ✅ | `name` |
| ResourceMonitor | ✅ | ✅ | `name` |
| MaskingPolicy | ✅ | ✅ | `name`, `database`, `schema`, `signature` |
| RowAccessPolicy | ✅ | ✅ | `name`, `database`, `schema`, `signature` |

### Immutable Field Violations

When an immutable field is changed externally in Snowflake, Snowplane:

1. **Detects** the violation and emits a distinct `ImmutableField` warning event
2. **Reports** the violation via the `DriftDetected` condition with details
3. **Skips ALTER** when only immutable fields drifted (ALTER cannot fix them)
4. **Corrects mutable fields** when both immutable and mutable drift exist ("mixed drift")

Immutable violations require manual intervention — delete/recreate the CR with the correct values.

### Database Drift-Detected Fields

The Database controller detects drift on all mutable spec fields:

- `comment`, `dataRetentionTimeInDays`, `maxDataExtensionTimeInDays`
- `defaultDdlCollation`, `replaceInvalidCharacters`
- `catalog`, `externalVolume`, `storageSerializationPolicy`
- `logLevel`, `metricLevel`, `traceLevel`

### Schema Drift-Detected Fields

The Schema controller detects drift on all mutable spec fields:

- `comment`, `managedAccess`
- `dataRetentionTimeInDays`, `maxDataExtensionTimeInDays`
- `defaultDdlCollation`, `replaceInvalidCharacters`
- `storageSerializationPolicy`, `logLevel`, `metricLevel`, `traceLevel`

### Warehouse Drift-Detected Fields

The Warehouse controller detects drift on all mutable spec fields:

- `comment`, `warehouseSize`, `autoSuspend`, `autoResume`
- `minClusterCount`, `maxClusterCount`, `scalingPolicy`
- `resourceMonitor`, `enableQueryAcceleration`, `queryAccelerationMaxScaleFactor`
- `maxConcurrencyLevel`, `statementQueuedTimeoutInSeconds`, `statementTimeoutInSeconds`
- `resourceConstraint`

The observed state is captured in `status.showOutput` (from `SHOW WAREHOUSES`) and `status.trackedParameters` (from `SHOW PARAMETERS`).

### AccountRole Drift-Detected Fields

The AccountRole controller detects drift on:

- `comment`

The observed state is captured in `status.showOutput` (from `SHOW ROLES`).

### DatabaseRole Drift-Detected Fields

The DatabaseRole controller detects drift on:

- `comment`

The observed state is captured in `status.showOutput` (from `SHOW DATABASE ROLES`).

### Table Drift-Detected Fields

The Table controller detects drift on:

- `comment`, `changeTracking`, `enableSchemaEvolution`

The observed state is captured in `status.showOutput` (from `SHOW TABLES`).

### View Drift-Detected Fields

The View controller detects drift on:

- `statement`, `comment`, `secure`, `changeTracking`

The observed state is captured in `status.showOutput` (from `SHOW VIEWS`).

### Stage Drift-Detected Fields

The Stage controller detects drift on:

- `comment`, `url`, `storageIntegration`

The observed state is captured in `status.showOutput` (from `SHOW STAGES`).

### User Drift-Detected Fields

The User controller detects drift on all mutable spec fields:

- `loginName`, `displayName`, `firstName`, `lastName`, `email`
- `comment`, `disabled`, `mustChangePassword`
- `defaultRole`, `defaultSecondaryRoles`, `defaultWarehouse`, `defaultNamespace`

Password and RSA key fields are not drift-detected because Snowflake does not expose their current values. Instead, password changes are tracked via `status.lastAppliedPasswordHash` (SHA-256 hash comparison).

The observed state is captured in `status.showOutput` (from `SHOW USERS`).

## Detect-Only Policy

By default, Snowplane corrects detected drift to bring the Snowflake resource back in sync with the Kubernetes spec. If you want to **detect and report** drift without correcting it, add the `detect-only` annotation:

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: analytics
  annotations:
    snowplane.hupe1980.github.io/drift-policy: detect-only
spec:
  providerRef:
    name: default
  name: ANALYTICS
  comment: "Analytics database"
```

With `detect-only`:
- Drift is detected and reported via the `DriftDetected` condition and events
- No `ALTER` statement is issued
- The resource remains in a "Ready" state with an active `DriftDetected` condition
- Useful for auditing, compliance, or environments where manual approval is required before changes

## Conditions and Events

### DriftDetected Condition

When drift is detected, a `DriftDetected` condition is set with status `True` and a message summarizing the drifted fields:

```
Type:    DriftDetected
Status:  True
Message: COMMENT: "desired" → "drifted", DATA_RETENTION_TIME_IN_DAYS: 30 → 1
```

After successful correction (or when drift is no longer present), the condition is cleared.

### Kubernetes Events

| Event | Type | When |
|-------|------|------|
| `DriftDetected` | Warning | Drift is found during reconciliation |
| `DriftCorrected` | Normal | Drift is successfully corrected |

## Architecture

The drift detection engine (`internal/drift/`) is a generic, reusable package with a fluent builder API:

```go
result := drift.New().
    CompareString("COMMENT", spec.Comment, obs.Comment, false).
    CompareInt32("RETENTION", spec.Retention, obs.Retention, false).
    CompareBool("AUTO_RESUME", spec.AutoResume, obs.AutoResume, false).
    Result()

if result.HasDrift {
    fmt.Println("Drifted fields:", result.Summary())
    fmt.Println("Mutable diffs:", result.FieldDiffs())
}

if result.HasImmutableViolation {
    fmt.Println("Immutable violations:", result.ImmutableSummary())
    fmt.Println("Immutable diffs:", result.ImmutableDiffs())
}
```

The `immutable` flag (last parameter) on each comparison marks fields that cannot be changed after creation. If an immutable field has drifted, `result.HasImmutableViolation` is set to `true`. The `HasDrift` flag only reflects mutable field changes. Use `FieldDiffs()` for mutable-only changes and `ImmutableDiffs()` for immutable-only changes.

## Nil-Means-Unmanaged

Pointer fields (`*string`, `*int32`, `*bool`) use `nil` to mean "not managed by Snowplane." When a field is `nil`:
- It is **not included** in drift detection comparisons
- It is **not included** in `CREATE` or `ALTER` statements
- Snowflake's server-side default remains in effect

Setting a field to a non-nil value puts it under declarative management and enables drift detection for that field.
