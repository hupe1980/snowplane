---
layout: default
title: Drift Detection
parent: Concepts
nav_order: 1
description: "Field-level drift detection engine with automatic correction or detect-only monitoring."
---

# Drift Detection
{: .fs-8 }

Snowplane includes a field-level drift detection engine that automatically detects and optionally corrects out-of-band changes to Snowflake resources.
{: .fs-5 .fw-300 }

---

## How It Works

Every reconciliation cycle (default: every 5 minutes), each controller:

1. **Observes** the current state via `SHOW` and `SHOW PARAMETERS` commands
2. **Compares** each managed field against the observed state using the `drift.Detector`
3. **Reports** differences as structured `FieldChange` entries
4. **Sets** the `DriftDetected` condition with a summary of all drifted fields
5. **Emits** a `DriftDetected` Kubernetes event
6. **Corrects** the drift via `ALTER` with the correct values
7. **Clears** the condition and emits a `DriftCorrected` event

---

## Supported Resources

All resource types support drift detection:

| Resource | Immutable Fields |
|:---------|:----------------|
| Database | `name`, `transient` |
| Schema | `name`, `database`, `transient` |
| Warehouse | `name` |
| User | `name`, `type` |
| AccountRole | `name` |
| DatabaseRole | `name`, `database` |
| AccountRoleAssignment | `grantedTo`, `granteeName` (all fields immutable) |
| DatabaseRoleAssignment | `grantedTo`, `granteeName` (all fields immutable) |
| Table | `name`, `database`, `schema`, `transient` |
| View | `name`, `database`, `schema` |
| MaterializedView | `name`, `database`, `schema`, `statement` |
| Stage | `name`, `database`, `schema`, `stageType` |
| Task | `name`, `database`, `schema` |
| Alert | `name`, `database`, `schema` |
| StreamOnTable | `name`, `database`, `schema`, `table` |
| StreamOnView | `name`, `database`, `schema`, `view` |
| StreamOnExternalTable | `name`, `database`, `schema`, `externalTable` |
| StreamOnDirectoryTable | `name`, `database`, `schema`, `stage` |
| StreamOnDynamicTable | `name`, `database`, `schema`, `dynamicTable` |
| Tag | `name`, `database`, `schema` |
| NetworkPolicy | `name` |
| ResourceMonitor | `name` |
| MaskingPolicy | `name`, `database`, `schema`, `signature` |
| RowAccessPolicy | `name`, `database`, `schema`, `signature` |
| StorageIntegration | `name` |
| SecurityIntegration | `name`, `type` |
| NotificationIntegration | `name`, `type` |
| PasswordPolicy | `name`, `database`, `schema` |
| NetworkRule | `name`, `database`, `schema`, `type`, `mode` |
| FileFormat | `name`, `database`, `schema`, `type` |
| Pipe | `name`, `database`, `schema`, `definition`, `integration` |
| DynamicTable | `name`, `database`, `schema`, `query`, `refreshMode` |

---

## Immutable Field Violations

When an immutable field is changed externally in Snowflake:

1. **Detects** the violation and emits an `ImmutableField` warning event
2. **Reports** via the `DriftDetected` condition with details
3. **Skips ALTER** when only immutable fields drifted
4. **Corrects mutable fields** when both immutable and mutable drift exist ("mixed drift")

Immutable violations require manual intervention — delete/recreate the CR.

---

## Drift-Detected Fields by Resource

### Database
{: .text-delta }

`comment`, `dataRetentionTimeInDays`, `maxDataExtensionTimeInDays`, `defaultDDLCollation`, `replaceInvalidCharacters`, `catalog`, `externalVolume`, `storageSerializationPolicy`, `logLevel`, `metricLevel`, `traceLevel`

### Schema
{: .text-delta }

`comment`, `managedAccess`, `dataRetentionTimeInDays`, `maxDataExtensionTimeInDays`, `defaultDDLCollation`, `replaceInvalidCharacters`, `storageSerializationPolicy`, `logLevel`, `metricLevel`, `traceLevel`

### Warehouse
{: .text-delta }

`comment`, `warehouseSize`, `autoSuspend`, `autoResume`, `minClusterCount`, `maxClusterCount`, `scalingPolicy`, `resourceMonitor`, `enableQueryAcceleration`, `queryAccelerationMaxScaleFactor`, `maxConcurrencyLevel`, `statementQueuedTimeoutInSeconds`, `statementTimeoutInSeconds`, `resourceConstraint`

### Table
{: .text-delta }

`comment`, `changeTracking`, `enableSchemaEvolution`, plus column-level drift (missing columns, extra columns, type changes, nullable changes, comment changes)

### View
{: .text-delta }

`statement`, `comment`, `secure`, `changeTracking`

### User
{: .text-delta }

`loginName`, `displayName`, `firstName`, `lastName`, `middleName`, `email`, `comment`, `disabled`, `mustChangePassword`, `defaultRole`, `defaultSecondaryRoles`, `defaultWarehouse`, `defaultNamespace`, `daysToExpiry`, `minsToUnlock`, `minsToBypassMFA`, `disableMFA`, `networkPolicy`

{: .note }
> Password and RSA key fields are not drift-detected because Snowflake does not expose their current values. Password changes are tracked via `status.lastAppliedPasswordHash`.

### SecurityIntegration
{: .text-delta }

`comment`, `enabled` (from SHOW), plus sub-type fields from DESCRIBE: ExternalOAuth (`jwsKeysURL`, `anyRoleMode`, `audienceList`, `allowedRoles`, `blockedRoles`, `networkPolicy`), SAML2 (`x509Cert`, `allowedEmailPatterns`, `allowedUserDomains`), SCIM (`networkPolicy`, `syncPassword`)

### PasswordPolicy
{: .text-delta }

`comment` (from SHOW), plus all numeric parameters from DESCRIBE: `passwordMinLength`, `passwordMaxLength`, `passwordMinUpperCaseChars`, `passwordMinLowerCaseChars`, `passwordMinNumericChars`, `passwordMinSpecialChars`, `passwordMinAgeDays`, `passwordMaxAgeDays`, `passwordMaxRetries`, `passwordLockoutTimeMins`, `passwordHistory`

### NetworkRule
{: .text-delta }

`comment` (from SHOW), `valueList` (from DESCRIBE)

### Task
{: .text-delta }

`comment`, `schedule`, `warehouse`, `sqlStatement`, `when`, `errorIntegration`, `allowOverlappingExecution`, `config` (from SHOW), plus `userTaskTimeoutMs`, `suspendTaskAfterNumFailures`, `taskAutoRetryAttempts`, `logLevel`, `userTaskMinimumTriggerIntervalInSeconds` (from SHOW PARAMETERS)

### Alert
{: .text-delta }

`comment`, `schedule`, `warehouse`, `condition`, `action`

### NotificationIntegration
{: .text-delta }

`comment`, `enabled` (from SHOW), plus sub-type fields from DESCRIBE: EMAIL (`allowedRecipients`, `defaultRecipients`, `defaultSubject`), QUEUE (`notificationProvider`, cloud-specific ARNs/endpoints), WEBHOOK (`webhookURL`, `webhookSecret`, `webhookBodyTemplate`, `webhookHeaders`)

---

## Detect-Only Policy

To **detect and report** drift without correcting it, set the `driftPolicy` in `spec.managementPolicies`:

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: analytics
spec:
  managementPolicies:
    driftPolicy: detect-only
  providerRef:
    name: default
  name: ANALYTICS
  comment: "Analytics database"
```

The `driftPolicy` field is enum-validated (`correct` / `detect-only`) and visible via `kubectl explain`.

With `detect-only`:
- Drift is detected and reported via conditions and events
- No `ALTER` statement is issued
- The resource remains `Ready` with an active `DriftDetected` condition
- Useful for auditing, compliance, or environments requiring manual approval

---

## Conditions and Events

### DriftDetected Condition

```
Type:    DriftDetected
Status:  True
Message: drifted fields: COMMENT, DATA_RETENTION_TIME_IN_DAYS
```

{: .note }
> For security, drift condition messages show **field names only** — actual values are never included in status conditions or events. Full before/after values are available in debug-level structured logs only.

After successful correction, the condition is cleared.

### Events

| Event | Type | When |
|:------|:-----|:-----|
| `DriftDetected` | Warning | Drift found during reconciliation |
| `DriftCorrected` | Normal | Drift successfully corrected |

---

## Architecture

The drift detection engine (`internal/drift/`) uses a fluent builder API:

```go
result := drift.New().
    CompareString("COMMENT", spec.Comment, obs.Comment, false).
    CompareInt32("RETENTION", spec.Retention, obs.Retention, false).
    CompareBool("AUTO_RESUME", spec.AutoResume, obs.AutoResume, false).
    Result()

if result.HasDrift {
    fmt.Println("Drifted fields:", result.Summary())
}
```

### Nil-Means-Unmanaged

Pointer fields use `nil` to mean "not managed." When a field is `nil`:
- Not included in drift detection comparisons
- Not included in `CREATE` or `ALTER` statements
- Snowflake's server-side default remains in effect
