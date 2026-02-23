# Development Guide

This guide explains the project layout, conventions, and workflow for contributing to Snowplane.

## Prerequisites

| Tool | Min Version |
|------|-------------|
| Go | 1.25+ |
| kubectl | 1.27+ |
| [just](https://github.com/casey/just) | 1.0+ |
| golangci-lint | 2.0+ |

## Project Layout

```
.
├── api/v1alpha1/              # CRD type definitions
│   ├── common_types.go        # Shared types (CommonSpec, CommonStatus, DeletionPolicy, ProviderReference)
│   ├── conditions.go          # Condition type & reason constants (incl. ReferencesResolved, DriftDetected, LateInitialized)
│   ├── annotations.go         # Annotation & label constants (AnnotationForceNew, AnnotationAdoptionPolicy, AnnotationDriftPolicy, LabelMaturity)
│   ├── validation.go          # Validate() methods on all spec types (errors.Join aggregation)
│   ├── database_types.go      # Database spec, status, show output
│   ├── schema_types.go        # Schema spec, status, show output (databaseRef)
│   ├── warehouse_types.go     # Warehouse spec, status, show output
│   ├── accountrole_types.go   # AccountRole spec, status, show output
│   ├── databaserole_types.go  # DatabaseRole spec, status, show output (databaseRef)
│   ├── user_types.go          # User spec, status, show output (secret-referenced credentials)
│   ├── grant_types.go         # Grant spec, status, show output (privilege grants)
│   ├── table_types.go         # Table spec, status, show output (columns, clustering)
│   ├── view_types.go          # View spec, status, show output (AS query, secure)
│   ├── stage_types.go         # Stage spec, status, show output (internal/external stages)
│   ├── providerconfig_types.go # ProviderConfig spec & status (incl. WorkloadIdentity, WIF providers)
│   ├── groupversion_info.go   # API group registration + kubebuilder markers
│   ├── deepcopy_test.go       # Mutation-based DeepCopy correctness tests
│   └── zz_generated.deepcopy.go # Auto-generated DeepCopy (controller-gen)
├── cmd/manager/               # Controller manager entrypoint
├── config/
│   ├── crd/bases/             # CRD YAML manifests (Database, Schema, Warehouse, AccountRole, DatabaseRole, User, Grant, Table, View, Stage, ProviderConfig)
│   ├── manager/               # Controller Deployment, PDB, NetworkPolicy
│   ├── rbac/                  # RBAC roles and bindings
│   ├── samples/               # Example CR YAML files (incl. WIF, OAuth, detect-only examples)
│   └── webhook/               # Validating & Mutating WebhookConfigurations, Service
├── hack/                      # Development & code-generation scripts
├── internal/
│   ├── clients/
│   │   ├── clientfactory/     # Snowflake client cache with SHA-256 hash-based rotation
│   │   └── snowflake/         # Snowflake SDK wrapper (client, database/schema/warehouse/accountrole/databaserole/user/grant/table/view/stage ops, identifiers, errors)
│   ├── controller/
│   │   ├── reconciler/        # Generic reconciler framework (GenericReconciler[T], ResourceAdapter[T])
│   │   ├── accountrole/       # AccountRole reconciler (adapter + helpers)
│   │   ├── database/          # Database reconciler (adapter + helpers)
│   │   ├── databaserole/      # DatabaseRole reconciler with databaseRef resolution (adapter + helpers)
│   │   ├── grant/             # Grant reconciler — immutable grant/revoke lifecycle (adapter + helpers)
│   │   ├── providerconfig/    # ProviderConfig reconciler
│   │   ├── refresolver/       # Cross-resource reference resolver
│   │   ├── schema/            # Schema reconciler with dependency resolution (adapter + helpers)
│   │   ├── stage/             # Stage reconciler with schema/database ref resolution (adapter + helpers)
│   │   ├── table/             # Table reconciler with column management (adapter + helpers)
│   │   ├── user/              # User reconciler with secret-referenced credentials (adapter + helpers)
│   │   ├── view/              # View reconciler with AS query management (adapter + helpers)
│   │   └── warehouse/         # Warehouse reconciler with drift detection (adapter + helpers)
│   ├── drift/                 # Generic field-level drift detection engine
│   ├── metrics/               # Custom Prometheus metrics (counters, histograms, gauges)
│   ├── provider/              # Shared config builder & hash (BuildSnowflakeConfig, ComputeHash)
│   ├── ratelimit/             # Per-provider token-bucket rate limiter
│   ├── circuitbreaker/        # Per-provider circuit breaker for failure isolation
│   ├── sfretry/               # Retry wrapper for transient Snowflake errors (IsRetryable, Do)
│   ├── testutil/              # Shared test helpers (PtrString, TestScheme, NewTestPC, etc.)
│   ├── utils/
│   │   ├── conditions/        # Kubernetes condition helpers
│   │   ├── finalizers/        # Finalizer management helpers
│   │   └── sanitize/          # SafeRecorder — strips SQL/DSN from Kubernetes event messages
│   └── webhook/               # Validating & mutating admission webhook handlers (all 11 resource kinds)
├── test/
│   ├── e2e/                   # End-to-end tests (k3s testcontainer + real Snowflake, build tag: e2e)
│   └── integration/           # envtest integration tests (build tag: integration)
│       └── snowflake/         # Real Snowflake client integration tests
└── docs/                      # Documentation
```

## Day-to-day Workflow

```bash
# Regenerate DeepCopy methods + CRD manifests
just generate

# Run tests (with race detection)
just test

# Run integration tests (requires setup-envtest)
just test-integration

# Run webhook integration tests (requires setup-envtest)
just test-webhook-integration

# Run linter
just lint

# Run go vet
just vet

# Build binary
just build

# Full CI pipeline (lint + vet + test + build)
just ci

# Run fuzz tests (default: 10s)
just fuzz
just fuzz 30s
```

### Code Generation

DeepCopy methods are auto-generated by [`controller-gen`](https://book.kubebuilder.io/reference/controller-gen) from kubebuilder markers. After modifying any type in `api/v1alpha1/`, run:

```bash
just generate     # Regenerate DeepCopy + CRD YAMLs from kubebuilder markers
just sync-crds    # Copy CRDs into the Helm chart directory
```

To verify CRDs and Helm chart are consistent:

```bash
just verify-crds  # Fails if Helm CRDs are out-of-sync with config/crd/bases/
just verify-helm  # Runs helm lint + helm template
```

This regenerates `api/v1alpha1/zz_generated.deepcopy.go`. **Never edit this file manually** — it is overwritten on every generation run. The mutation-based test suite in `deepcopy_test.go` validates that the generated code correctly isolates all pointer fields.

## Architecture Principles

### Observe-Diff-Apply

Every reconciler follows the same lifecycle:

1. **Observe** — Query Snowflake for the current state of the resource
2. **Diff** — Compare the observed state with the desired spec
3. **Apply** — Execute only the SQL statements needed to reach desired state

This minimises Snowflake API calls and avoids unnecessary mutations.

### Generic Reconciler Framework

All ten Snowflake resource reconcilers (AccountRole, Database, DatabaseRole, Grant, Schema, Stage, Table, User, View, Warehouse) share the same Observe-Diff-Apply state machine. To eliminate ~80% code duplication, the shared logic lives in `internal/controller/reconciler/`:

- **`GenericReconciler[T ManagedResource]`** — A type-parameterised reconciler that implements the full lifecycle: finalizer management, ProviderConfig resolution, client caching, service creation, status patching (with conflict retry via `retry.RetryOnConflict`), condition management, rate limiting, retry, metrics, and drift detection.
- **`ResourceAdapter[T ManagedResource]`** — An interface that each resource package implements to provide resource-specific behaviour: `Observe`, `Create`, `Alter`, `Drop`, `BuildAlterOptions`, `ValidateImmutableFields`, `ApplyObservation`, `DetectDrift`, `ComputeTrackedParameters`, `PreReconcile`, `PostCreate`, `PostUpdate`, `SetupWatches`, `ServiceFromClient`, `BuildIdentifier`, `NewObject`, `ResourceName`, `FinalizerName`.
- **`ManagedResource`** — A constraint interface that all CRD types satisfy, providing access to spec fields (`GetSpecName`, `GetDeletionPolicy`, `GetUseRole`, `GetProviderRef`), status fields (`GetConditions`, `SetConditions`, `GetObservedGeneration`, `SetObservedGeneration`, `GetLastAppliedSpecHash`, `SetLastAppliedSpecHash`), and Kubernetes object methods.
- **`WithUseRole`** — A shared helper for `ServiceFactory` boilerplate that handles SnowflakeClient type assertion and optional role switching.

Each resource package provides:
1. **`adapter.go`** — Implements `ResourceAdapter[T]` with resource-specific logic (e.g., Schema's `PreReconcile` resolves `databaseRef`, User tracks password hashes).
2. **`reconciler.go`** — A slim file (~100-400 lines) that defines the `Service` interface, `NewReconciler` constructor, and standalone helper functions (`applyObservation`, `buildCreateOptions`, `buildAlterOptions`, `computeUnsetFields`, `computeTrackedParameters`, `detectDrift`).

This architecture reduces each reconciler from ~800 lines to ~200-400 lines of resource-specific code, while the ~420-line generic framework handles all shared concerns.

#### Safe Type Assertions

Type assertions in adapter methods (e.g. `id.(snowflake.AccountObjectIdentifier)`) must never panic. Use the generic assertion helpers from `reconciler/assert.go`:

```go
// In error-returning methods (Observe, Create, Alter, Drop, BuildAlterOptions):
aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
detail, err := reconciler.AssertDetail[ShowOutput](obs)
ao, err := reconciler.AssertAlterOptions[*AlterOptions](opts)

// In non-error-returning methods (ApplyObservation, DetectDrift):
detail, ok := obs.Detail.(ShowOutput)
if !ok { return }
```

#### Reconciler Helpers

The generic reconciler provides shared methods that eliminate repetitive patterns:

- **`finalizeSpec(ctx, obj)`** — Computes and stores the spec hash, updates the tracked-parameters list, and handles errors with terminal conditions + best-effort status patch. Called after every successful create/update/adopt.
- **`executeSnowflakeOp(ctx, opCtx, obj, opName, opVerb, opFn)`** — Runs a Snowflake operation (CREATE, ALTER, CoA) with metrics, retries, and standard terminal/non-terminal error classification.

#### PreReconcile Reference Resolution Helpers

For resources that reference a parent database or schema, use the helpers from `refresolver/helpers.go`:

```go
// Database-only resources (Schema, DatabaseRole):
dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, obj,
    obj.Namespace, obj.Spec.DatabaseRef, obj.Spec.DatabaseName, obj.Status.DatabaseName)

// Database+Schema resources (Table, View, Stage):
schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, obj,
    obj.Namespace, obj.Spec.SchemaRef, obj.Spec.SchemaName, obj.Status.SchemaName)
```

These encapsulate the full resolution → deletion-timestamp fallback → reference-not-found logging pattern.

### Drift Correction

The reconciler computes diffs on every loop — not just when `metadata.generation` changes. This means out-of-band changes made directly in Snowflake (e.g., via the Snowflake UI or SQL) are automatically corrected.

Drift detection is powered by the generic **Drift Detector** engine in `internal/drift/`:

- `drift.New()` returns a fluent builder that accumulates field-level comparisons via `CompareString()`, `CompareInt32()`, `CompareBool()`, etc.
- Each comparison accepts an `immutable` flag — immutable violations are surfaced separately.
- Nil-means-unmanaged: if the desired value is `nil`, no drift is reported (the user hasn't declared intent for that field).
- The final `.Result()` returns `HasDrift`, `HasImmutableViolation`, `Changes []FieldChange`, and `Summary()` / `FieldDiffs()` / `ImmutableDiffs()` / `ImmutableSummary()` helpers.

When drift is detected, reconcilers set the `DriftDetected` condition and emit Kubernetes events. A **detect-only** policy is supported via the `snowplane.hupe1980.github.io/drift-policy: detect-only` annotation — drift is reported but not corrected. See `docs/drift-detection.md` for full details.

When immutable fields drift externally, the reconciler emits a distinct `ImmutableField` warning event and skips ALTER when only immutable fields have drifted (ALTER can't fix them). When both mutable and immutable drift exist, mutable fields are corrected via ALTER while the immutable violation is reported.

### Immutable Field Enforcement

Fields like `spec.transient` and `spec.name` are immutable after creation. The reconciler validates these against `status.showOutput` and sets a `Terminal` condition if a change is detected. Immutable field violations are **terminal** — the reconciler returns `ctrl.Result{}` (no requeue, no error) to avoid infinite exponential backoff retries for an unfixable condition. The user must revert the field or use the `force-new` annotation.

The `force-new` annotation (`snowplane.hupe1980.github.io/force-new: "true"`) bypasses immutability checks at both the webhook *and* reconciler level. When set, the reconciler's `validateImmutableFields()` returns early, allowing the reconciler to proceed with delete+recreate. This ensures ForceNew works even when webhooks are disabled.

### Cross-Resource Reference Resolution

Schema-level resources reference their parent Database via `databaseRef`. The `ReferenceResolver` (in `internal/controller/refresolver/`) provides a centralised lookup mechanism:

1. **Resolve** — Looks up the referenced CR, checks `Ready=True`, returns `fullyQualifiedName`
2. **Wait** — If the dependency is not Ready, sets `ReferencesResolved=False` with `DependencyNotReady` reason and returns the error to controller-runtime for exponential backoff
3. **Watch** — The Schema reconciler watches Database CRs via `EnqueueRequestsFromMapFunc`, so when a Database transitions to Ready, all dependent Schemas are requeued automatically

The `ReferenceResolver` is a shared component — all schema-level resources (Schema, Table, View, Stage, DatabaseRole) use it for parent reference resolution.

### Interface-Driven Testing

All Snowflake operations are behind interfaces (`Service`, `SnowflakeClient`). Unit tests inject mocks via `ServiceFactory`, so no real Snowflake connections are needed.

### Shared Provider Package

`internal/provider/` contains `BuildSnowflakeConfig()` and `ComputeHash()` — shared between all reconcilers that need to resolve a ProviderConfig into a Snowflake connection. This prevents code duplication while keeping each reconciler self-contained.

### Condition Constants

All condition types (`Ready`, `Synced`, `Terminal`, `Recoverable`, `ReferencesResolved`, `CredentialsInvalid`, `DriftDetected`) and reasons (`Available`, `ReconcileSuccess`, `DependencyNotReady`, `TransientError`, `DriftDetected`, `DriftCorrected`, `RateLimited`, `ValidationFailed`, `ImmutableField`, etc.) are defined as constants in `api/v1alpha1/conditions.go` and reused through the `conditions` utility package.

### UNSET Support & TrackedParameters Tracking

Pointer fields use nil-means-unmanaged semantics. When a user sets a field (e.g., `dataRetentionTimeInDays: 14`) and later removes it from the spec, the reconciler must issue `ALTER ... UNSET` to revert to Snowflake defaults.

This is powered by **TrackedParameters** tracking in resource status:

1. On every successful Create or Update, the reconciler computes which spec fields are non-nil and saves their Snowflake parameter names in `status.trackedParameters`.
2. On subsequent reconciliations, `computeUnsetFields()` compares current spec (nil fields) against `status.trackedParameters` — any field present in `trackedParameters` but nil in spec needs an UNSET.
3. The SQL builders (`buildAlterStatements()`, `buildAlterSchemaStatements()`) generate separate `ALTER ... SET` and `ALTER ... UNSET` statements.

When adding a new managed resource with pointer fields, implement the corresponding `computeTrackedParameters()` and `computeUnsetFields()` functions in the reconciler.

### Recoverable Condition

The `Recoverable` condition distinguishes transient errors (timeouts, rate limits) from terminal errors (invalid config, immutable field violation):

- **Set** `Recoverable=True` with reason `TransientError` when a Snowflake operation fails with a retryable error.
- **Clear** `Recoverable` on every successful reconciliation.
- **Terminal** errors use the existing `Terminal` condition and do not set `Recoverable`.

This enables monitoring systems and alert rules to distinguish "will fix itself" from "needs user intervention."

### Deletion Resilience (Cascading Delete)

When a parent resource (Database CR or ProviderConfig) is deleted before its dependents, the dependent reconcilers handle it gracefully:

- **Schema → Database deleted:** The schema reconciler falls back to `status.databaseName` (cached from the last successful observe) instead of requiring `databaseRef` resolution. This allows the Snowflake DROP to proceed.
- **Any resource → ProviderConfig deleted:** If `resolveClient` fails during deletion, the reconciler removes the finalizer and lets Kubernetes garbage-collect the CR, since the Snowflake resource is already inaccessible.
- **ProviderConfig in-use guard:** ProviderConfig uses a `providerconfig.snowplane.hupe1980.github.io/in-use` finalizer. On deletion, the reconciler checks all 10 resource types for references. If any managed resource still references the ProviderConfig, deletion is blocked with an `InUse` warning event and the ProviderConfig is requeued after 30s. Once all references are removed, the finalizer is cleared and the ProviderConfig is deleted.

This prevents finalizer deadlocks in cascading delete scenarios.

### Defense-in-Depth Validation

Every reconciler calls `spec.Validate()` as the first step of reconciliation, *before* resolving the Snowflake client or making any API calls. This is a defense-in-depth layer that catches invalid specs even when webhooks are disabled (e.g., during development, in CI, or when bypass paths are used).

On validation failure, the reconciler:
1. Sets `Terminal=True` with reason `ValidationFailed` and the aggregated error message.
2. Sets `Synced=False` with the same reason.
3. Emits a `ValidationFailed` warning event.
4. Returns `ctrl.Result{}` (no requeue) — the user must fix the spec.

This complements the webhook-level validation: webhooks provide fast API-level rejection, while reconciler-level validation acts as a safety net.

### Input Validation

Snowflake parameter ranges are validated early in `Validate()` methods:

- `MaxDataExtensionTimeInDays` must be 0–90 per Snowflake documentation.
- `UserSpec.Email` is validated via `net/mail.ParseAddress()` when non-nil and non-empty.
- `escapeLikePattern()` escapes `\`, `%`, and `_` for safe LIKE queries.
- Identifiers reject empty/whitespace names via `ValidObjectIdentifier()`.

Validation errors are returned before any SQL is generated, giving clear error messages.

### Custom Prometheus Metrics

`internal/metrics/` defines eleven custom Prometheus metrics that provide operational visibility:

- **Reconciliation metrics** (`snowplane_reconcile_total`, `snowplane_reconcile_duration_seconds`) — Track reconciliation throughput and latency per resource type.
- **Snowflake API metrics** (`snowplane_snowflake_operation_total`, `snowplane_snowflake_operation_duration_seconds`) — Track Snowflake operations by type (create, alter, drop, observe, ping).
- **Resource gauges** (`snowplane_managed_resources`) — Track managed resource counts by type and status (ready, not_ready, terminal).
- **Client pool gauge** (`snowplane_client_pool_size`) — Track the number of cached Snowflake client connections (capped by configurable `maxSize` with LRU eviction).
- **Rate limiter** (`snowplane_rate_limit_waits_total`) — Count rate limiter wait events per controller.
- **Adoption** (`snowplane_adoption_total`) — Count resource adoption outcomes (adopted / rejected).
- **Drift** (`snowplane_drift_detected_total`) — Count drift detection events per controller.
- **Circuit breaker** (`snowplane_circuit_breaker_trips_total`, `snowplane_circuit_breaker_state`) — Track circuit breaker trips and current state per provider.

Metrics are registered via `init()` and exposed on the standard `/metrics` endpoint. The `ObserveSnowflakeOp()`, `RecordReconcile()`, `RecordDriftDetected()`, `RecordCircuitBreakerTrip()` and `SetCircuitBreakerState()` helpers are called from reconcilers and the circuit breaker to instrument operations.

### Rate Limiting

`internal/ratelimit/` provides per-provider token-bucket rate limiting for Snowflake API calls:

- Each ProviderConfig gets its own `rate.Limiter` (from `golang.org/x/time/rate`).
- The `Wait(ctx, providerName)` method blocks until a token is available or the context expires.
- Limiters are created lazily and cached in a `sync.Map` for lock-free concurrent access.
- QPS and burst size are configurable via `--rate-limit-qps` and `--rate-limit-burst` flags.
- When rate-limited, reconcilers set `Recoverable=True` with reason `RateLimited`.

### Retry for Transient Snowflake Errors

`internal/sfretry/` provides a thin retry wrapper for transient Snowflake errors:

- `IsRetryable(err)` classifies errors — connection failures, timeouts, and context errors are retryable; permission denied, already-exists, role-switch failures, SQL compilation errors (code 1003), and quota limits are terminal (non-retryable).
- `Do(ctx, opts, fn)` runs `fn` up to `opts.MaxAttempts` times with a fixed back-off (`opts.Backoff`) between retries. It short-circuits on non-retryable errors or context cancellation.
- `DefaultOptions()` returns `MaxAttempts=3, Backoff=2s`.
- Each retry is logged at debug level (`logger.V(1)`) with attempt count, max attempts, back-off, and error message.
- All mutating Snowflake operations (CREATE, ALTER, DROP) in every resource reconciler are wrapped in `sfretry.Do` inside the existing `metrics.ObserveSnowflakeOp` call.

**Idempotency Safety:** All retried operations are inherently idempotent:
- **CREATE** — If the object already exists, `ErrObjectAlreadyExists` is classified as non-retryable and the loop stops immediately.
- **ALTER** — Applying the same desired state twice is a no-op.
- **DROP** — "Object not found" after a partial drop is treated as success, not an error.
- **GRANT/REVOKE** — Single-target grant/revoke statements are idempotent in Snowflake (granting an already-held privilege is a no-op).

### Configurable Requeue Interval

By default every reconciler re-observes Snowflake state every **5 minutes** (`DefaultRequeueInterval` in the `reconciler` package). This interval can be overridden at startup with the `--requeue-interval` flag:

```
--requeue-interval 2m   # re-observe every 2 minutes
```

Internally `GenericReconciler` has a `requeueOverride` field set via the `WithRequeueInterval(d)` builder. The private `getRequeueInterval()` helper returns the override when non-zero, falling back to the compile-time default.

### Circuit Breaker (Failure Isolation)

`internal/circuitbreaker/` provides a per-provider circuit breaker that prevents one failing ProviderConfig from impacting resources managed by healthy ProviderConfigs:

- **Three states:** Closed (normal), Open (rejecting calls), HalfOpen (probing after cooldown).
- **Threshold:** Opens after N consecutive failures (default: 5). Configurable via `Options.FailureThreshold`.
- **Reset timeout:** Stays open for a configurable duration (default: 60s) before transitioning to half-open. In half-open state, a single probe call is allowed — success resets to closed, failure reopens.
- **Per-provider isolation:** Each ProviderConfig gets an independent breaker. Failures on `provider-a` don't affect `provider-b`.
- **Integration:** The `GenericReconciler` calls `RecordSuccess` / `RecordFailure` in its defer block after every reconciliation. The `ResolveClient` function calls `cb.Allow(providerName)` to check the breaker before creating a Snowflake client — if the breaker is open, the resource gets a `DependencyNotReady` condition and is requeued.
- **Metrics:** `snowplane_circuit_breaker_trips_total` (counter per provider) and `snowplane_circuit_breaker_state` (gauge: 0=closed, 1=open, 2=half-open).
- **Thread-safe:** All operations are guarded by `sync.RWMutex`. The clock is injectable for deterministic testing.

### Validating & Mutating Admission Webhooks

`internal/webhook/` implements `admission.Handler` for all eleven resource kinds (Database, Schema, Warehouse, AccountRole, DatabaseRole, User, Grant, Table, View, Stage, ProviderConfig):

**Mutating webhook** (`DefaultsMutator`):
- Injects `deletionPolicy: Delete` and `providerRef.name: default` when unset.
- Defaults `User.type` to `PERSON`.
- Registered at `/mutate-snowplane-v1alpha1-<resource>` paths.

**Validating webhooks** (per-resource validators):
- Each validator calls `spec.Validate()` for field-level validation (required fields, enum values, range bounds).
- On UPDATE, checks immutable fields unless the `force-new` annotation is set.
- Uses `errors.Join` to aggregate all violations into a single denial response.
- **Database:** `name`, `transient`, `useRole` are immutable. **Schema:** `name`, `databaseRef`, `transient`, `useRole` are immutable. **Warehouse:** `name`, `useRole` are immutable. **AccountRole:** `name`, `useRole` are immutable. **DatabaseRole:** `name`, `databaseRef`, `useRole` are immutable. **User:** `name`, `type`, `useRole` are immutable. **Grant:** all spec fields are immutable (grants are immutable — revoke and re-grant). **Table:** `name`, `databaseRef`, `schemaRef`, `useRole` are immutable. **View:** `name`, `databaseRef`, `schemaRef`, `useRole` are immutable. **Stage:** `name`, `databaseRef`, `schemaRef`, `stageType`, `useRole` are immutable. **ProviderConfig:** `account`, `user` are immutable (guarded by `ObservedGeneration > 0`, consistent with all other validators).
- ForceNew annotation (`snowplane.hupe1980.github.io/force-new: "true"`) bypasses immutability checks, allowing the reconciler to handle delete+recreate.
- Webhooks are optional — enabled via `--enable-webhooks` flag in the controller.
- Webhook configuration YAML is in `config/webhook/`.

### Workload Identity Federation (WIF)

Snowplane supports native Workload Identity Federation (WIF) via `gosnowflake.AuthTypeWorkloadIdentityFederation`:

- `authenticationType: WorkloadIdentity` uses the gosnowflake driver's native WIF support.
- The driver handles OIDC attestation exchange, token reading, and automatic refresh — the operator just provides `TokenFilePath` and `WorkloadIdentityProvider`.
- **Multi-cloud providers**: `OIDC` (any K8s cluster with projected SA tokens), `AWS` (IRSA/Pod Identity), `GCP` (GKE metadata), `Azure` (AKS IMDS). Defaults to `OIDC` if not specified.
- No Secret is needed — the token file is mounted via a projected service account token volume.
- `ComputeHash()` includes `TokenFilePath` and `WorkloadIdentityProvider` (stable config), not the volatile token value.

## Adding a New Managed Resource

See **[docs/adding-a-resource.md](adding-a-resource.md)** for the comprehensive step-by-step guide with code examples covering CRD types, dual ref/name patterns, validation, adapter wiring, webhooks, and tests.

Quick checklist (details in the linked guide):

1. Define CRD types in `api/v1alpha1/<resource>_types.go` — implement `ManagedResource`
2. Add `Validate()` in `validation.go` using `validateDatabaseSource`/`validateSchemaSource` helpers
3. Create Snowflake client in `internal/clients/snowflake/<resource>.go`
4. Create adapter (`ResourceAdapter[T]`) + reconciler in `internal/controller/<resource>/`
5. Wire into `cmd/manager/main.go` (controller + RBAC markers)
6. Add webhooks (mutating defaults + validating immutability)
7. Add CRD manifest (`just generate && just sync-crds`), sample CRs, and tests
8. Add the new type to the ProviderConfig in-use guard

## Testing Strategy

| Layer | What to test | Tool |
|-------|-------------|------|
| Snowflake client | SQL generation, option validation, error handling | `testing` + `testify` |
| Reconciler | Full loop with mock service, conditions, status, ref resolution | `testing` + fake `client.Client` |
| Reference resolver | Cross-resource lookup, Ready/NotReady, missing refs | `testing` + fake `client.Client` |
| Shared provider | Config building, hash determinism, WIF token file handling | `testing` + `testify` |
| Metrics | Counter/histogram/gauge registration and recording | `testing` + `prometheus/testutil` |
| Rate limiter | Token bucket behaviour, per-provider isolation, concurrency | `testing` + `testify` |
| Drift detector | Field-level comparisons, immutable violations, nil-means-unmanaged | `testing` + `testify` |
| Webhooks | Immutable field rejection, allow on no change, CREATE passthrough | `testing` + `admission` |
| Retry wrapper | Retryable classification, attempt exhaustion, context cancellation | `testing` + `testify` |
| Utilities | Conditions, finalizers | `testing` + `testify` |
| Test helpers | Shared setup code (scheme, PC, secret, request builders) | `internal/testutil` |
| Integration | Full CRD → reconciler pipeline against real kube-apiserver | envtest |

### Integration Tests (envtest)

The `test/integration/` package contains integration tests that run against a real Kubernetes API server using [envtest](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest). These tests validate the full reconciliation loop — from CR creation through status patching, drift detection, and deletion — with mock Snowflake services.

**Prerequisites:**

```bash
# Install setup-envtest (one-time)
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# Download API server binaries (one-time)
setup-envtest use
```

**Running integration tests:**

```bash
# Via just
just test-integration

# Via go test directly
KUBEBUILDER_ASSETS="$(setup-envtest use -p path)" go test -tags integration -v -timeout 180s -count=1 ./test/integration/
```

Integration tests use the `//go:build integration` build tag and are excluded from `just test`.

**Test coverage (55 tests across 10 resources):**

| Resource | Tests | What they validate |
|----------|-------|--------------------|
| **Database** (12) | `CreateLifecycle`, `UpdateTriggersAlter`, `DeleteWithOrphanPolicy`, `FinalizerAddedOnCreate`, `DriftDetection`, `ImmutableFieldRejection`, `ObservedGenerationUpdated`, `TrackedParametersTracking`, `StatusSubresourcePatch`, `Adoption_FailIfExists`, `Adoption_AdoptSuccess`, `Adoption_ExplicitFailIfExists` | Full lifecycle, CEL immutability, spec hash, tracked parameters tracking, status subresource, adoption flow |
| **Schema** (5) | `CreateWithDatabaseRef`, `WaitForDatabaseReady`, `UpdateTriggersAlter`, `DeleteWithDatabaseGone`, `ImmutableDatabaseRef` | Parent ref resolution, dependency waiting, cascading delete, CEL immutability |
| **Warehouse** (6) | `CreateLifecycle`, `UpdateTriggersAlter`, `DeleteWithOrphanPolicy`, `DriftDetection`, `Adoption_FailIfExists`, `Adoption_AdoptSuccess` | Account-level lifecycle, drift correction, orphan policy, adoption flow |
| **User** (3) | `CreateLifecycle`, `UpdateTriggersAlter`, `DeleteWithOrphanPolicy` | Account-level lifecycle, secret-referenced credentials |
| **AccountRole** (4) | `CreateLifecycle`, `UpdateTriggersAlter`, `DeleteWithOrphanPolicy`, `DriftDetection` | Simplest resource lifecycle, comment-only drift |
| **DatabaseRole** (5) | `CreateLifecycle`, `WaitForDatabaseReady`, `UpdateTriggersAlter`, `DeleteWithOrphanPolicy`, `DriftDetection` | Two-part identifier, parent Database dependency, drift correction |
| **Grant** (3) | `CreateLifecycle`, `WithGrantOption`, `DeleteWithOrphanPolicy` | Grant/Revoke lifecycle (no ALTER), grant option flag, immutable spec |
| **Table** (5) | `CreateLifecycle`, `UpdateTriggersAlter`, `DeleteWithOrphanPolicy`, `DriftDetection`, `WaitForSchemaReady` | Column management, schema-level ref resolution, drift correction |
| **View** (5) | `CreateLifecycle`, `UpdateTriggersAlter`, `DeleteWithOrphanPolicy`, `DriftDetection`, `WaitForSchemaReady` | AS query management (CREATE OR REPLACE on statement change), secure views, schema-level ref resolution |
| **Stage** (6) | `CreateLifecycle`, `UpdateTriggersAlter`, `DeleteWithOrphanPolicy`, `DriftDetection`, `WaitForSchemaReady`, `ImmutableStageType` | Internal/external stages, stage type immutability, file format options |

### Webhook Integration Tests (envtest + WebhookInstallOptions)

The `internal/webhook/integration_test.go` file contains integration tests that validate admission webhooks against a real kube-apiserver with TLS. These tests exercise:

- **Validating webhooks:** CREATE rejection for invalid specs, UPDATE immutability enforcement, force-new bypass
- **Mutating webhooks:** Default injection (e.g., `deletionPolicy: Delete`)
- **DELETE passthrough:** Deletion always allowed regardless of validation rules
- **All 11 resource types** covered (Database, Schema, Warehouse, AccountRole, DatabaseRole, User, Grant, Table, View, Stage, ProviderConfig)

```bash
# Run webhook integration tests
KUBEBUILDER_ASSETS="$(setup-envtest use -p path)" go test -tags integration -v -timeout 180s -count=1 ./internal/webhook/
```

## Code Style

- `golangci-lint` enforces style (see `.golangci.yml`)
- All exported functions have doc comments
- All tests are parallelised (`t.Parallel()`)
- No magic strings — use constants from `api/v1alpha1/conditions.go`

## Resource Adoption

Snowplane supports adopting pre-existing Snowflake resources into Kubernetes management. This is useful when migrating from manual management or Terraform.

### How It Works

When a CRD is created and the corresponding Snowflake resource already exists, the reconciler checks the `adoption-policy` annotation:

| Annotation Value | Behaviour |
|------------------|-----------|
| *(absent)* | Terminal error with `ReasonResourceExists` — prevents accidental takeover |
| `fail-if-exists` | Same as absent — explicit opt-out |
| `adopt` | Reconciler adopts the resource: populates status from current state, sets `LateInitialized` condition |
| *(invalid value)* | Logged as warning, defaults to `fail-if-exists` behaviour |

### Detection Mechanism

Adoption is detected when:
1. `Observe()` returns `Exists: true`
2. `ObservedGeneration == 0` (first reconciliation)

This combination means the Snowflake resource exists but the CRD has never been successfully reconciled.

### Example

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Database
metadata:
  name: adopted-database
  annotations:
    snowplane.hupe1980.github.io/adoption-policy: adopt
spec:
  name: EXISTING_DATABASE
  providerRef:
    name: default
```

After the first reconciliation, the resource will have:
- `Ready: True`
- `Synced: True`
- `LateInitialized: True` — indicates status was populated from existing state
- `status.showOutput` populated with the current Snowflake configuration

### Code References

- Annotation constants: `api/v1alpha1/annotations.go` (`AnnotationAdoptionPolicy`, `AdoptionPolicyAdopt`, `AdoptionPolicyFailIfExists`)
- Condition type: `api/v1alpha1/conditions.go` (`TypeLateInitialized`, `ReasonAdopted`, `ReasonResourceExists`)
- Reconciler logic: `internal/controller/reconciler/reconciler.go` (`reconcileAdoptOrReject`, `getAdoptionPolicy`)
- Metrics: `internal/metrics/metrics.go` (`AdoptionTotal`, `RecordAdoption`, `RecordAdoptionRejected`)
- Unit tests: `internal/controller/reconciler/reconciler_test.go` (5 adoption tests)
- Integration tests: `test/integration/database_test.go` (3 tests), `test/integration/warehouse_test.go` (2 tests)

## Maturity Classification

All CRDs carry a maturity label (`snowplane.hupe1980.github.io/maturity`) indicating their stability level:

| Level | Guarantees | Flag Control |
|-------|-----------|-------------|
| `alpha` | No stability guarantees; API may change | Gated by `--enable-alpha-resources` (default: `true`) |
| `beta` | Backwards-compatible changes expected | Always enabled |
| `stable` | Full backwards-compatibility | Always enabled |

### Controller Gating

The `GenericReconciler` supports maturity-based and per-controller feature gating:

```go
database.NewReconciler(...)
    .WithRequeueInterval(requeueInterval)
    .WithMaturity("alpha")
    .WithAlphaEnabled(enableAlphaResources)
    .WithDisabled(disabled["database"])
    .SetupWithManager(mgr, maxConcurrentReconciles)
```

When `--enable-alpha-resources=false`, alpha controllers are silently skipped during manager startup — they log a message and return without registering the controller.

For fine-grained per-controller control, the `--disable-controllers` flag accepts a comma-separated list of controller names to skip regardless of maturity:

```bash
# Disable specific controllers
--disable-controllers=grant,stage,view

# Valid controller names: database, schema, warehouse, accountrole, databaserole, grant, user, table, view, stage
```

The `WithDisabled(true)` setting takes precedence over maturity — a disabled stable controller is still skipped.

### Current Classification

All resource CRDs are currently classified as `alpha` (v1alpha1 API version). The `ProviderConfig` CRD is `stable` since it is always required for operator functionality.

### CRD Labels

Maturity labels are also applied to CRD YAML manifests in `config/crd/bases/`:

```yaml
metadata:
  labels:
    snowplane.hupe1980.github.io/maturity: alpha
```

## Terraform Migration Tool (`tfimport`)

The `cmd/tfimport` CLI tool automates migrating Snowflake resources from Terraform (or OpenTofu) to Snowplane CRDs. It reads a Terraform state file and generates Kubernetes manifests with the `adopt` annotation, so the operator adopts existing Snowflake objects instead of recreating them.

### Quick Start

```bash
# Build
go build -o tfimport ./cmd/tfimport

# Generate manifests
./tfimport -state terraform.tfstate -namespace snowflake -provider prod > manifests.yaml

# Apply to cluster
kubectl apply -f manifests.yaml
```

### State-Removal Script

After migrating, remove the resources from Terraform state so they are no longer dual-managed:

```bash
# Generate manifests + removal script
./tfimport -state terraform.tfstate -remove-script remove.sh > manifests.yaml

# Dry run
bash remove.sh --dry-run

# Execute (for OpenTofu: TF_CMD=tofu bash remove.sh)
bash remove.sh
```

### Supported Resources

12 Terraform resource types are supported — see [`cmd/tfimport/README.md`](../cmd/tfimport/README.md) for the full list and detailed usage.
