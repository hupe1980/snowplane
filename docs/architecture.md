---
layout: default
title: Architecture
parent: Concepts
nav_order: 5
description: "Reconciler state machine, adapter pattern, resilience layers, and component model."
---

# Architecture
{: .fs-8 }

A deep dive into Snowplane's controller architecture — the generic reconciler, adapter pattern, resilience layers, and crash-recovery mechanisms.
{: .fs-5 .fw-300 }

---

## Component Overview

```
┌─────────────────────────────────────────────────────────────┐
│                  Controller Manager                         │
│                                                             │
│  ┌────────────────────────────────────────────────────┐     │
│  │  Validating Admission Webhook (optional)           │     │
│  │  OwnershipValidator — deny duplicate FQN mappings  │     │
│  └────────────────────────────────────────────────────┘     │
│                                                             │
│  ┌───────────┐ ┌───────────┐ ┌───────────┐                │
│  │ Database   │ │ Warehouse │ │ Schema    │  ... (65 CRDs) │
│  │ Controller │ │ Controller│ │ Controller│                │
│  └─────┬─────┘ └─────┬─────┘ └─────┬─────┘                │
│        │              │              │                      │
│  ┌─────▼──────────────▼──────────────▼─────┐               │
│  │         GenericReconciler[T, S, D]       │               │
│  │  ┌─────────────┐  ┌───────────────────┐ │               │
│  │  │ResourceAdapter│  │ drift.Detector   │ │               │
│  │  └──────┬──────┘  └───────────────────┘ │               │
│  └─────────┼───────────────────────────────┘               │
│            │                                                │
│  ┌─────────▼───────────────────────────────┐               │
│  │         Resilience Layer                 │               │
│  │  ┌────────────┐ ┌──────────┐ ┌────────┐ │               │
│  │  │RateLimiter │ │CircuitBkr│ │ Retry  │ │               │
│  │  └────────────┘ └──────────┘ └────────┘ │               │
│  └─────────┬───────────────────────────────┘               │
│            │                                                │
│  ┌─────────▼───────────────────────────────┐               │
│  │    ClientFactory (LRU + Singleflight)   │               │
│  └─────────┬───────────────────────────────┘               │
└────────────┼────────────────────────────────────────────────┘
             │
             ▼
    ┌──────────────────┐
    │   Snowflake API  │
    │  (SQL over HTTPS)│
    └──────────────────┘
```

Every CRD controller is an instance of `GenericReconciler[T, S, D]` parameterized with:

| Parameter | Purpose | Example |
|:----------|:--------|:--------|
| **T** | CRD Go type (implements `ManagedResource`) | `*v1alpha1.Database` |
| **S** | Snowflake CRUD service interface | `database.Service` |
| **D** | Observation detail type (resource-specific show output) | `database.ShowOutput` |

All Snowflake-specific logic lives in the **ResourceAdapter** — the reconciler itself is resource-agnostic.

---

## Reconciler State Machine

The reconciler follows an **Observe → Classify → Act** loop:

```
                         ┌──────────┐
                         │  Fetch   │
                         │   CR     │
                         └────┬─────┘
                              │
                    ┌─────────▼──────────┐
                    │ Paused? PreRecon?   │
                    │ Resolve Provider?   │
                    └─────────┬──────────┘
                              │
                   ┌──────────▼──────────┐
                   │ DeletionTimestamp?   │──Yes──► reconcileDelete()
                   └──────────┬──────────┘
                              │ No
                   ┌──────────▼──────────┐
                   │  Add Finalizer      │
                   │  Validate Spec      │
                   └──────────┬──────────┘
                              │
                   ┌──────────▼──────────┐
                   │  Pre-flight Checks  │
                   │  (DB/Schema exist?) │
                   └──────────┬──────────┘
                              │
                   ┌──────────▼──────────┐
                   │     OBSERVE         │
                   │  (SHOW + DESCRIBE)  │
                   └──────────┬──────────┘
                              │
              ┌───────────────┼───────────────┐
              │               │               │
       ┌──────▼──────┐┌──────▼──────┐ ┌──────▼──────────────┐
       │ Not Exists  ││  Exists +   │ │  Exists +           │
       │             ││ ObsGen == 0 │ │  ObsGen > 0         │
       └──────┬──────┘└──────┬──────┘ └──────┬──────────────┘
              │              │               │
              ▼              ▼               ▼
       reconcileCreate  Adopt/Crash?   reconcileUpdate
```

### Observe Phase

Every cycle starts with an **Observe** call — typically a Snowflake `SHOW <resource>` query. The result is wrapped in an `Observation[D]` struct:

```go
type Observation[D any] struct {
    Exists bool   // Whether the Snowflake resource exists
    Detail D      // Resource-specific show output
}
```

### Classification

After observing, the reconciler classifies the situation:

| Condition | Branch |
|:----------|:-------|
| Resource doesn't exist in Snowflake | `reconcileCreate()` |
| Exists, `ObservedGeneration == 0`, **creation-initiated annotation** present | `reconcilePostCrashCreate()` |
| Exists, `ObservedGeneration == 0`, no creation-initiated annotation | `reconcileAdoptOrReject()` |
| Exists, `ObservedGeneration > 0` | `reconcileUpdate()` |

---

## Pre-flight Checks

After provider resolution and before OBSERVE, the reconciler runs automatic pre-flight validation for all 38 database/schema-scoped resource types. This prevents opaque Snowflake SQL errors when prerequisite resources don't exist.

**Two-phase architecture:**

1. **Automatic phase** — The reconciler detects whether the resource implements `ScopedResource` (all 38 scoped types do via generated accessors). When `databaseName`/`schemaName` raw strings are used (not CR references), it issues `SHOW DATABASES LIKE`/`SHOW SCHEMAS LIKE` queries to verify existence. When `databaseRef`/`schemaRef` are used, existence was already validated during reference resolution in PreReconcile.

2. **Adapter-specific phase** — If the adapter implements `PreFlightChecker`, its custom check runs after the automatic checks (reserved for non-standard requirements).

**Error discrimination:** The pre-flight distinguishes between definitive "not found" responses (`ErrObjectNotFound` from empty `SHOW` results) and non-definitive errors (connection timeouts, auth failures). Only definitive "not found" produces a hard failure with `DependencyNotReady`. Non-definitive errors are logged and skipped — the subsequent service creation surfaces connectivity issues with proper error handling.

---

## Create Flow

1. **Set creation-initiated annotation** — crash-recovery marker written *before* the Snowflake CREATE.
2. **Stamp external-name label** — SHA-256 hash of the fully qualified name for cross-cluster conflict detection.
3. **PATCH metadata** — persist annotation and label.
4. **Execute Snowflake CREATE** — with fallback: if CREATE OR ALTER is unsupported, retry with CREATE IF NOT EXISTS.
5. **Post-create observation** — re-observe to verify; requeue in 5s if not yet observable.
6. **Apply observation** — write observed state to `status`, set `Ready=True`, `Synced=True`, compute spec hash and tracked parameters.
7. **Invoke PostCreate hook** — optional adapter hook (e.g., hash initial password for User resources).
8. **Remove creation-initiated annotation** — cleanup after status is committed.

---

## Update Flow

1. **Apply observation** from the initial Observe call.
2. **Build alter options** — adapter computes the diff between desired spec and observed state.
3. **Check for changes** — if `HasChanges()`:
   - Compute current spec hash. Compare with `lastAppliedSpecHash`.
   - **Same hash** → external **drift** (Snowflake changed outside the controller). Invoke `DetectDrift()` for field-level diffs.
   - **Different hash** → user **spec change**.
4. **Drift policy check** — if `driftPolicy: detect-only`, report drift but skip correction.
5. **Execute ALTER** — prefer CREATE OR ALTER if supported (with fallback to plain ALTER).
6. **Re-observe** — verify the change was applied.
7. **Finalize** — update spec hash, tracked parameters, generation, conditions.

See [Drift Detection]({% link drift-detection.md %}) for the full drift engine documentation.

---

## Delete Flow

```
  DeletionTimestamp set
          │
  ┌───────▼───────┐
  │ abandon-on-   │──Yes──► Remove finalizer, emit warning, done
  │ delete annot? │
  └───────┬───────┘
          │ No
  ┌───────▼───────┐
  │ Orphan policy?│──Yes──► Remove finalizer, skip DROP, done
  └───────┬───────┘
          │ No
  ┌───────▼───────┐
  │  DROP resource │
  └───────┬───────┘
          │
   ┌──────▼──────┐    ┌─────────────────────────────┐
   │  Success?   │─No─► Set DeleteBlocked condition  │
   └──────┬──────┘    │ Suggest abandon-on-delete    │
          │ Yes       └─────────────────────────────┘
   ┌──────▼──────┐
   │  Remove     │
   │  finalizer  │
   └─────────────┘
```

If a DROP fails with a terminal error (e.g., dependencies exist, permissions revoked), the controller sets a `DeleteBlocked` condition and suggests the `abandon-on-delete` annotation as an escape hatch.

---

## Adoption Flow

When a Snowflake resource **already exists** but the CRD has never been reconciled (`ObservedGeneration == 0`):

| Policy | Behavior |
|:-------|:---------|
| `fail-if-exists` (default) | Terminal error — user must delete the Snowflake resource manually |
| `adopt` | Check ownership label for conflicts, stamp label, apply observation, mark `Adopted` |

Ownership conflict detection uses the `external-name` label (hash of FQN). If another CR in the same cluster already manages the same Snowflake object, adoption fails with `ConflictDetected`.

---

## Crash Recovery

The **creation-initiated annotation** solves a specific failure mode:

1. Controller sends CREATE to Snowflake — **succeeds**.
2. Controller crashes before committing status to Kubernetes.
3. On restart, the resource exists in Snowflake but `ObservedGeneration == 0` — looks like it should be adopted.

Without the annotation, the controller would apply the adoption policy and potentially reject the resource. With the annotation present, the controller enters `reconcilePostCrashCreate()` — it applies the observation and marks the resource as recovered without requiring adoption policy configuration.

---

## Adapter Pattern

Each resource defines a `newAdapter()` function that returns a `*reconciler.BaseAdapter[T, S, D]` — a closure-based implementation of the `ResourceAdapter[T, S, D]` interface. Instead of defining a per-resource struct with 14+ methods, resource-specific behavior is injected via function fields on `BaseAdapter`:

| BaseAdapter Field | Purpose |
|:------------------|:--------|
| `ResourceNameVal` | Human-readable name for logs and events |
| `FinalizerNameVal` | Finalizer string for deletion protection |
| `NewObjectFn` | Factory for empty CRD instances |
| `ServiceFactoryFn` | Create the Snowflake CRUD service |
| `BuildIdentifierFn` | Construct the fully qualified Snowflake identifier |
| `ObserveFn` | Execute SHOW query, return observation |
| `CreateFn` | Execute CREATE statement |
| `AlterFn` | Execute ALTER statement |
| `DropFn` | Execute DROP statement |
| `ValidateImmutableFn` | Check for forbidden field changes |
| `BuildAlterOptsFn` | Compute diff between spec and observed |
| `ApplyObservationFn` | Write observed state to CRD status |
| `TrackedParamsFn` | Custom tracked params (default: reflection) |
| `DetectDriftFn` | Field-level drift comparison |

Generic helper factories (`MakeObserve`, `MakeCreate`, `MakeAlter`, `MakeDrop`, `MakeBuildAlterOpts`) handle the repetitive `AssertIdentifier`/`AssertAlterOptions` boilerplate. `MakeServiceFactory` provides the standard `WithUseRole + newClient` pattern.

### Optional Capabilities

`BaseAdapter` satisfies all optional interfaces with nil-safe defaults. Set the corresponding function field to opt in:

| Field | Interface | Purpose |
|:------|:----------|:--------|
| `PreReconcileFn` | `PreReconciler[T]` | Pre-reconcile setup (e.g., Schema resolves databaseRef) |
| `SetupWatchesFn` | `WatchConfigurer` | Extra watches (e.g., Schema watches Database) |
| `PostCreateFn` | `PostCreateHook[T]` | Post-create logic (e.g., hash initial password) |
| `PostUpdateFn` | `PostUpdateHook[T]` | Post-update logic with access to alter options |
| `SupportsCoA` | `CreateOrAlterSupporter` | Flag whether CREATE OR ALTER SQL is supported |
| `DropCascadeFn` | `CascadeDropper[T,S]` | Enable `DROP ... CASCADE` support |
| `LateInitializeFn` | `LateInitializer[T,D]` | Fill unset spec fields from observed state |

---

## Tracked Parameters

The `tracked` package uses reflection over `snowflake:"PARAM_NAME"` struct tags to generically compute which Snowflake parameters a user actively manages.

```go
// In any spec struct:
type DatabaseSpec struct {
    DataRetentionTimeInDays *int32  `json:"dataRetentionTimeInDays,omitempty" snowflake:"DATA_RETENTION_TIME_IN_DAYS"`
    MaxDataExtensionTime    *int32  `json:"maxDataExtensionTime,omitempty"    snowflake:"MAX_DATA_EXTENSION_TIME_IN_DAYS"`
    Comment                 *string `json:"comment,omitempty"                 snowflake:"COMMENT"`
}

// BaseAdapter handles ComputeTrackedParameters automatically via reflection.
// Override TrackedParamsFn only for custom behavior.
```

**Tag modifiers:**

| Modifier | Effect |
|:---------|:-------|
| `always` | Always included in tracked list regardless of nil |
| `nounset` | Excluded from `ComputeUnset` (parameter cannot be UNSET) |
| `prefix` | Map keys expanded to `PREFIX_<key>` entries |

`ComputeUnset(spec, previouslyTracked)` returns parameters that were tracked in the previous reconciliation but are now nil — enabling the controller to issue `ALTER ... UNSET` for removed fields.

See [Development Guide]({% link development.md %}) for more on the nil-means-unmanaged pattern.

---

## Resilience Layers

### Circuit Breaker

Per-provider circuit breaker with three states:

| State | Behavior |
|:------|:---------|
| **Closed** | Normal operation |
| **Open** | All calls rejected with `ErrCircuitOpen` after 5 consecutive failures |
| **HalfOpen** | Single probe call after backoff. Success → Closed, failure → Open |

Backoff starts at 60s and doubles with ±20% jitter on each failure, capped at 15 minutes. Each `ProviderConfig` gets an independent breaker.

### Rate Limiter

Hierarchical token-bucket rate limiting:

| Level | Default QPS | Default Burst | Purpose |
|:------|:------------|:--------------|:--------|
| Per-controller | 10 | 20 | Fairness between controllers sharing one account |
| Per-account | 50 | 100 | Aggregate cap across all controllers for one Snowflake account |

### Client Factory

LRU-cached client pool with singleflight deduplication:

- **Keyed** by namespace-qualified provider name (`namespace/name`) + config hash. Namespace qualification ensures multi-tenant isolation — two ProviderConfigs named `"default"` in different namespaces cannot collide.
- **Singleflight** — only one connection attempt per provider during thundering-herd scenarios.
- **Idle TTL** — optional time-based eviction for unused clients.
- **LRU eviction** — when `MaxSize` is reached, least-recently-used client is closed.

### Retry

Configurable retry with exponential backoff for transient Snowflake errors. Terminal errors (`snowflake.IsTerminalError`) exit the retry loop immediately.

---

## Error Classification

| Category | Requeue | Examples |
|:---------|:--------|:--------|
| **Terminal** | No — reconciliation stops | Invalid SQL, validation failure, immutable field changes |
| **Recoverable** | Yes — exponential backoff | Network errors, credential issues, rate limits |
| **Transient** | Yes — fast retry within operation | Connection reset, temporary Snowflake unavailability |

Terminal errors set `Ready=False` with `ReasonTerminalError` and emit a Kubernetes Warning event. The resource remains in this state until the user fixes the spec.

---

## Status Updates

All status patches use **Server-Side Apply (SSA)** with field owner `snowplane-controller`. This avoids conflicts with other writers and enables clean field ownership tracking. Key status fields:

| Field | Purpose |
|:------|:--------|
| `conditions` | Standard Kubernetes conditions (Ready, Synced, DriftDetected, ReferencesResolved) |
| `observedGeneration` | Last successfully reconciled `metadata.generation` |
| `lastAppliedSpecHash` | SHA-256 of the spec at last successful reconciliation |
| `trackedParameters` | List of Snowflake parameters actively managed by the spec |
| `showOutput` | Last observed Snowflake state (resource-specific) |

---

## Admission Webhook

Snowplane includes an optional **validating admission webhook** that intercepts `CREATE` and `UPDATE` requests for all snowplane CRDs at admission time, preventing ownership conflicts before they reach the reconciler.

### Why a Webhook?

Without the webhook, ownership conflicts are only detected **after** the reconciler runs — the duplicate CR is created in the cluster and a `ConflictDetected` condition is set asynchronously. The webhook shifts detection **left**, rejecting the request synchronously at the API server level.

### How It Works

```
  kubectl apply ─────► API Server ─────► Webhook
                                          │
                                    ┌─────▼──────┐
                                    │ Compute FQN │
                                    │ from spec   │
                                    └─────┬──────┘
                                          │
                                    ┌─────▼──────────────┐
                                    │ Hash FQN           │
                                    │ (same algorithm as │
                                    │  reconciler)       │
                                    └─────┬──────────────┘
                                          │
                                    ┌─────▼──────────────┐
                                    │ List CRs with      │
                                    │ matching hash label │
                                    └─────┬──────────────┘
                                          │
                               ┌──────────┼──────────┐
                               │                     │
                          No conflict           Conflict!
                               │                     │
                          ┌────▼────┐           ┌────▼────┐
                          │ ALLOW   │           │  DENY   │
                          └─────────┘           └─────────┘
```

**FQN computation from spec fields:**

| Resource Level | Spec Fields | FQN Format |
|:---------------|:------------|:-----------|
| Account (Database, Warehouse, etc.) | `spec.name` | `"NAME"` |
| Database (Schema, DatabaseRole) | `spec.databaseName` or `spec.databaseRef` + `spec.name` | `"DB"."NAME"` |
| Schema (Table, View, etc.) | `spec.databaseName`/`databaseRef` + `spec.schemaName`/`schemaRef` + `spec.name` | `"DB"."SCHEMA"."NAME"` |

When CR references (`databaseRef`, `schemaRef`) are used instead of inline names, the webhook resolves them by reading the referenced CR's `spec.name` from the API server.

### Graceful Degradation

The webhook is designed to **never block valid operations**:

| Condition | Behavior |
|:----------|:---------|
| Referenced CR doesn't exist yet | Allow — reconciler validates later |
| Label list API call fails | Allow — reconciler catches conflict |
| FQN cannot be computed | Allow — insufficient data for a verdict |
| Webhook pod is down | Allow — `failurePolicy: Ignore` (default) |

This makes the webhook a **best-effort early check** — the reconciler remains the authoritative conflict detector.

### Enabling

The webhook is opt-in and requires [cert-manager](https://cert-manager.io/) for TLS certificate management:

```yaml
webhook:
  enabled: true
  failurePolicy: Ignore  # or Fail for strict enforcement
  certManager:
    enabled: true
```

See [Helm Chart]({% link helm-chart.md %}) for the full set of webhook configuration options.

---

## Further Reading

- [Resource Dependencies & Ordering](/snowplane/resource-dependencies/) — Built-in readiness gating, FieldExport, kro, and GitOps ordering
- [Drift Detection](/snowplane/drift-detection/) — Field-level drift detection and correction policies
- [CRD Lifecycle](/snowplane/crd-lifecycle/) — Create, adopt, update, and delete lifecycle details
- [Observability](/snowplane/observability/) — Metrics, events, and Grafana dashboard
