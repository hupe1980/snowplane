# Adding a New Managed Resource

This guide walks through every step needed to add a new Snowflake resource to Snowplane — from CRD type definitions to integration tests. Use it as a checklist; every step links to real code in the repository.

> **Reference implementation:** The Schema resource is the simplest database-level resource and serves as the canonical example throughout this guide.

---

## Table of Contents

1. [Decide the Resource Level](#1-decide-the-resource-level)
2. [Define CRD Types](#2-define-crd-types)
3. [Add Validation](#3-add-validation)
4. [Implement the ManagedResource Interface](#4-implement-the-managedresource-interface)
5. [Regenerate DeepCopy & CRD Manifests](#5-regenerate-deepcopy--crd-manifests)
6. [Create the Snowflake Client](#6-create-the-snowflake-client)
7. [Create the Reconciler Package](#7-create-the-reconciler-package)
8. [Wire into the Manager](#8-wire-into-the-manager)
9. [Add Admission Webhooks](#9-add-admission-webhooks)
10. [Add Sample Manifests](#10-add-sample-manifests)
11. [Write Tests](#11-write-tests)
12. [Final Checklist](#12-final-checklist)

---

## 1. Decide the Resource Level

Snowflake objects live at different scopes. The scope determines which parent references your resource needs:

| Level | Parent References | Examples |
|-------|-------------------|----------|
| **Account** | None | Database, Warehouse, AccountRole, User, NetworkPolicy, ResourceMonitor |
| **Database** | `databaseRef` / `databaseName` | Schema, DatabaseRole |
| **Schema** | `databaseRef` / `databaseName` + `schemaRef` / `schemaName` | Table, View, Stage, Task, Stream, Tag, MaskingPolicy, RowAccessPolicy |
| **Grant** | Role ref or inline | AccountRoleGrant, DatabaseRoleGrant, ShareGrant, GrantOwnership |

Account-level resources are the simplest — they have no parent dependencies. Database-level and schema-level resources require reference resolution (see [Step 2](#2-define-crd-types)).

---

## 2. Define CRD Types

Create `api/v1alpha1/<resource>_types.go`. Every resource follows this structure:

### Account-Level Resource (no parent refs)

```go
// ThingSpec defines the desired state of a Thing.
type ThingSpec struct {
    CommonSpec `json:",inline"`

    // Name is the Snowflake object name. Immutable after creation.
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.name is immutable after creation"
    Name string `json:"name"`

    // Comment is an optional description.
    Comment *string `json:"comment,omitempty"`
}
```

### Database-Level Resource (one parent ref)

Database-level resources reference their parent database via **either** a CR reference (`databaseRef`) or a raw Snowflake identifier (`databaseName`). This dual pattern supports mixed management — users can manage the database with Snowplane or reference a pre-existing database not managed by Snowplane.

```go
// ThingSpec defines the desired state of a Thing.
//
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
type ThingSpec struct {
    CommonSpec `json:",inline"`

    // Name is the Snowflake object name. Immutable after creation.
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.name is immutable after creation"
    Name string `json:"name"`

    // DatabaseRef references a Database CR in the same namespace.
    // Mutually exclusive with DatabaseName. Immutable after creation.
    // +optional
    // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseRef is immutable after creation"
    DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

    // DatabaseName is the raw Snowflake database identifier (e.g. "ANALYTICS").
    // Use this when the database is NOT managed by Snowplane.
    // Mutually exclusive with DatabaseRef. Immutable after creation.
    // +optional
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseName is immutable after creation"
    DatabaseName *string `json:"databaseName,omitempty"`

    // ... resource-specific fields ...
}
```

**Key points:**

- `DatabaseRef` is a **pointer** (`*LocalObjectReference`), making it optional in JSON.
- `DatabaseName` is a **pointer** (`*string`), also optional.
- The struct-level `XValidation` CEL rule enforces exactly-one-of (XOR) at the API server level.
- Both fields have field-level `XValidation` for immutability (`self == oldSelf`).

### Schema-Level Resource (two parent refs)

Schema-level resources need both database and schema references. Add two XOR rules:

```go
// ThingSpec defines the desired state of a Thing.
//
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
type ThingSpec struct {
    CommonSpec `json:",inline"`

    Name string `json:"name"`

    // DatabaseRef references a Database CR in the same namespace.
    // +optional
    // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseRef is immutable after creation"
    DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

    // DatabaseName is the raw Snowflake database identifier.
    // +optional
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseName is immutable after creation"
    DatabaseName *string `json:"databaseName,omitempty"`

    // SchemaRef references a Schema CR in the same namespace.
    // +optional
    // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.schemaRef is immutable after creation"
    SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

    // SchemaName is the raw Snowflake schema FQN (e.g. '"ANALYTICS"."PUBLIC"').
    // +optional
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.schemaName is immutable after creation"
    SchemaName *string `json:"schemaName,omitempty"`

    // ... resource-specific fields ...
}
```

### Status Type

Every status includes `DatabaseName` (and `SchemaName` for schema-level resources). These are populated during `PreReconcile` and used as cached fallbacks during cascading deletes:

```go
type ThingStatus struct {
    CommonStatus `json:",inline"`

    // DatabaseName is the resolved Snowflake database name (cached for deletion resilience).
    DatabaseName string `json:"databaseName,omitempty"`

    // SchemaName is the resolved Snowflake schema FQN (for schema-level resources).
    SchemaName string `json:"schemaName,omitempty"`

    // ShowOutput holds the latest SHOW output from Snowflake.
    ShowOutput *ThingShowOutput `json:"showOutput,omitempty"`

    // TrackedParameters lists Snowflake parameter names that have been explicitly
    // set via spec fields. Used to compute UNSET operations.
    TrackedParameters []string `json:"trackedParameters,omitempty"`
}
```

### Kubebuilder Root Markers

Add the standard kubebuilder markers on the root type:

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type="string",JSONPath=".status.showOutput.name",priority=0
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Synced",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:categories=snowplane
type Thing struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec   ThingSpec   `json:"spec,omitempty"`
    Status ThingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ThingList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []Thing `json:"items"`
}

func init() {
    SchemeBuilder.Register(&Thing{}, &ThingList{})
}
```

---

## 3. Add Validation

Add a `Validate()` method in `api/v1alpha1/validation.go`:

```go
func (s *ThingSpec) Validate() error {
    var errs []error

    if s.Name == "" {
        errs = append(errs, errors.New("spec.name is required"))
    }

    // For database-level or schema-level resources:
    if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
        errs = append(errs, err)
    }

    // For schema-level resources only:
    if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
        errs = append(errs, err)
    }

    // Resource-specific validation...
    if s.SomeField != nil && *s.SomeField < 0 {
        errs = append(errs, fmt.Errorf("spec.someField must be non-negative (got: %d)", *s.SomeField))
    }

    if err := s.CommonSpec.Validate(); err != nil {
        errs = append(errs, err)
    }

    return errors.Join(errs...)
}
```

The shared helpers enforce XOR semantics:

- `validateDatabaseSource(ref, name)` — exactly one of `databaseRef` or `databaseName` must be set
- `validateSchemaSource(ref, name)` — exactly one of `schemaRef` or `schemaName` must be set

These are defense-in-depth — they complement the CEL rules on the struct.

---

## 4. Implement the ManagedResource Interface

Every CRD type must satisfy the `reconciler.ManagedResource` constraint. Add these methods to the root type (the `Thing` struct, not the spec):

```go
func (t *Thing) GetDeletionPolicy() DeletionPolicy         { return t.Spec.DeletionPolicy }
func (t *Thing) GetSpecName() string                        { return t.Spec.Name }
func (t *Thing) GetProviderRef() ProviderReference          { return t.Spec.ProviderRef }
func (t *Thing) GetUseRole() *string                      { return t.Spec.UseRole }
func (t *Thing) GetObservedGeneration() int64               { return t.Status.ObservedGeneration }
func (t *Thing) SetObservedGeneration(gen int64)            { t.Status.ObservedGeneration = gen }
func (t *Thing) GetLastAppliedSpecHash() string             { return t.Status.LastAppliedSpecHash }
func (t *Thing) SetLastAppliedSpecHash(h string)            { t.Status.LastAppliedSpecHash = h }
func (t *Thing) GetTrackedParametersList() []string         { return t.Status.TrackedParameters }
func (t *Thing) SetTrackedParametersList(p []string)        { t.Status.TrackedParameters = p }
func (t *Thing) GetFullyQualifiedName() string              { return t.Status.FullyQualifiedName }
func (t *Thing) GetOwner() string {
    if t.Status.ShowOutput != nil { return t.Status.ShowOutput.Owner }
    return ""
}
func (t *Thing) ValidateSpec() error                        { return t.Spec.Validate() }
func (t *Thing) ComputeSpecHash() (string, error)           { return ComputeSpecHash(t.Spec) }

// ConditionedObject methods:
func (t *Thing) GetConditions() []metav1.Condition          { return t.Status.Conditions }
func (t *Thing) SetConditions(c []metav1.Condition)         { t.Status.Conditions = c }
```

---

## 5. Regenerate DeepCopy & CRD Manifests

After any type changes, regenerate:

```bash
# DeepCopy methods + CRD YAML manifests
just generate

# Sync CRDs into Helm chart
just sync-crds
```

This produces:
- `api/v1alpha1/zz_generated.deepcopy.go` — handles nil-safe copying of pointer fields
- `config/crd/bases/snowplane.hupe1980.github.io_things.yaml` — the CRD manifest with CEL rules

**Never edit these files manually.**

Add a maturity label to the generated CRD manifest:

```yaml
metadata:
  labels:
    snowplane.hupe1980.github.io/maturity: alpha
```

---

## 6. Create the Snowflake Client

Create `internal/clients/snowflake/<resource>.go` with:

1. **Identifier type** — use the existing `AccountObjectIdentifier`, `DatabaseObjectIdentifier`, or `SchemaObjectIdentifier` from `internal/clients/snowflake/identifiers.go`.

2. **Observation type** — holds the SHOW output and exists flag:
   ```go
   type ThingObservation struct {
       Exists     bool
       Name       string
       Comment    string
       // ... fields matching SHOW THINGS output
   }
   ```

3. **Options types** — `CreateThingOptions`, `AlterThingOptions` (implement `HasChanges() bool`)

4. **Client type** — wraps `SQLExecutor`:
   ```go
   type ThingClient struct {
       exec SQLExecutor
   }

   func NewThingClient(exec SQLExecutor) *ThingClient {
       return &ThingClient{exec: exec}
   }

   func (c *ThingClient) Observe(ctx context.Context, id SchemaObjectIdentifier) (*ThingObservation, error) { ... }
   func (c *ThingClient) Create(ctx context.Context, opts CreateThingOptions) error { ... }
   func (c *ThingClient) Alter(ctx context.Context, opts AlterThingOptions) error { ... }
   func (c *ThingClient) Drop(ctx context.Context, id SchemaObjectIdentifier) error { ... }
   ```

---

## 7. Create the Reconciler Package

Create `internal/controller/<resource>/` with two files:

### `reconciler.go`

Defines the `Service` interface, `NewReconciler`, and standalone helper functions:

```go
package thing

const finalizerName = "snowplane.hupe1980.github.io/thing"

type SnowflakeClient = clientfactory.SnowflakeClient

type Service interface {
    Observe(ctx context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.ThingObservation, error)
    Create(ctx context.Context, opts snowflake.CreateThingOptions) error
    Alter(ctx context.Context, opts snowflake.AlterThingOptions) error
    Drop(ctx context.Context, id snowflake.SchemaObjectIdentifier) error
}

type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Thing] {
    a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
    return &reconciler.GenericReconciler[*snowplanev1alpha1.Thing]{
        Client:      c,
        Factory:     factory,
        Recorder:    recorder,
        RateLimiter: rl,
        Adapter:     a,
    }
}

func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
    sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
    if err != nil {
        return nil, nil, err
    }
    return snowflake.NewThingClient(sfC), cleanup, nil
}

// Helper functions: applyObservation, buildCreateOptions, buildAlterOptions,
// computeTrackedParameters, computeUnsetFields, detectDrift
```

### `adapter.go`

Implements `ResourceAdapter[T]`. The structure varies by resource level:

#### Account-Level Adapter (simplest)

```go
type adapter struct {
    client     sigs.Client
    recorder   record.EventRecorder
    newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "thing" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.Thing { return &snowplanev1alpha1.Thing{} }

func (a *adapter) PreReconcile(_ context.Context, _ *snowplanev1alpha1.Thing) error { return nil }

func (a *adapter) BuildIdentifier(thing *snowplanev1alpha1.Thing) any {
    return snowflake.NewAccountObjectIdentifier(thing.Spec.Name)
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc { return nil }
```

#### Database-Level Adapter

Database-level resources must resolve `databaseRef`/`databaseName` in `PreReconcile` and set up a field indexer + database watch. Use the shared helpers from `refresolver`:

```go
func (a *adapter) PreReconcile(ctx context.Context, thing *snowplanev1alpha1.Thing) error {
    dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, thing,
        thing.Namespace, thing.Spec.DatabaseRef, thing.Spec.DatabaseName, thing.Status.DatabaseName)
    if err != nil {
        return err
    }

    thing.Status.DatabaseName = dbFQN

    refresolver.SetDatabaseResolvedCondition(thing, thing.Spec.DatabaseRef, thing.Spec.DatabaseName, dbFQN)

    return nil
}
```

For schema-level resources, add `PreReconcileSchemaRef` after the database resolution:

```go
    schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, thing,
        thing.Namespace, thing.Spec.SchemaRef, thing.Spec.SchemaName, thing.Status.SchemaName)
    if err != nil {
        return err
    }

    thing.Status.SchemaName = schemaFQN

    refresolver.SetDatabaseAndSchemaResolvedCondition(thing, thing.Spec.DatabaseRef, thing.Spec.DatabaseName,
        thing.Spec.SchemaRef, thing.Spec.SchemaName)
```

##### Reference Resolution with Dual Mode

The `PreReconcileDatabaseRef` and `PreReconcileSchemaRef` helpers handle **everything** internally:
- Resolution via `ResolveDatabaseSource` / `ResolveSchemaSource`
- Condition setting (`ReferencesNotResolved` / `NotReady` / warning event)
- Deletion-timestamp fallback to cached `status.databaseName` / `status.schemaName`
- Logging when reference is not found or not ready

You do **not** need a local `resolveDatabaseRef` helper, `SourceName()` calls, or manual condition setting — the shared helpers do it all.

See `docs/development.md > PreReconcile Reference Resolution Helpers` for full API details.

##### Nil-Safe Field Indexers

Field indexers enable the watch-based requeue (when a parent Database CR changes, all dependent resources are re-reconciled). Since `DatabaseRef` is now a pointer, the indexer must handle nil:

```go
func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
    return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
        if err := mgr.GetFieldIndexer().IndexField(
            ctx,
            &snowplanev1alpha1.Thing{},
            ".spec.databaseRef.name",
            func(o sigs.Object) []string {
                t, ok := o.(*snowplanev1alpha1.Thing)
                if !ok || t.Spec.DatabaseRef == nil {  // ← nil check required
                    return nil
                }
                return []string{t.Spec.DatabaseRef.Name}
            },
        ); err != nil {
            return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
        }

        bldr.Watches(
            &snowplanev1alpha1.Database{},
            handler.EnqueueRequestsFromMapFunc(a.mapDatabaseToThings),
        )

        return nil
    }
}
```

> **Important:** Resources using `databaseName` (raw string) won't trigger watch-based requeue since there's no corresponding CR to watch. This is by design — the raw name points to a Snowflake object outside Snowplane's management.

##### Interface Assertion

Always add a compile-time interface assertion at the bottom of the file:

```go
var _ reconciler.ResourceAdapter[*snowplanev1alpha1.Thing] = (*adapter)(nil)
```

#### Schema-Level Adapter

Schema-level resources repeat the exact same patterns for both database and schema:

- `PreReconcile`: resolve database first, then schema
- `resolveDatabaseRef` + `resolveSchemaRef` with `ResolveDatabaseSource` / `ResolveSchemaSource`
- Two field indexers (`.spec.databaseRef.name` and `.spec.schemaRef.name`), both nil-safe
- Two watch map functions (`mapDatabaseToThings`, `mapSchemaToThings`)
- Shared display-name helper `refresolver.SourceName()` for log messages (no local copy needed)

The `SetReferencesResolved` call goes in `resolveSchemaRef` (only after both refs are resolved):

```go
func (a *adapter) resolveSchemaRef(ctx context.Context, thing *snowplanev1alpha1.Thing) (string, error) {
    schemaFQN, err := refresolver.ResolveSchemaSource(ctx, a.client, thing.Namespace, thing.Spec.SchemaRef, thing.Spec.SchemaName)
    if err != nil {
        // ... set conditions + event ...
        return "", err
    }

    dbName := refresolver.SourceName(thing.Spec.DatabaseRef, thing.Spec.DatabaseName)
    schName := refresolver.SourceName(thing.Spec.SchemaRef, thing.Spec.SchemaName)
    conditions.SetReferencesResolved(thing, fmt.Sprintf("Database %s and Schema %s resolved", dbName, schName))

    return schemaFQN, nil
}
```

---

## 8. Wire into the Manager

In `cmd/manager/main.go`:

### Import

```go
thingctl "github.com/hupe1980/snowplane/internal/controller/thing"
```

### Register the Controller

```go
if err := thingctl.NewReconciler(
    mgr.GetClient(),
    factory,
    sanitize.NewSafeRecorderFromEvents(mgr.GetEventRecorder("thing-controller")),
    rl,
).WithCircuitBreaker(cb).WithRequeueInterval(requeueInterval).WithMaturity("alpha").WithAlphaEnabled(enableAlphaResources).WithDisabled(disabled["thing"]).SetupWithManager(mgr, maxConcurrentReconciles); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "Thing")
    os.Exit(1)
}
```

### Add RBAC Markers

In `cmd/manager/main.go`, add RBAC markers for the new resource:

```go
//+kubebuilder:rbac:groups=snowplane.hupe1980.github.io,resources=things,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=snowplane.hupe1980.github.io,resources=things/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=snowplane.hupe1980.github.io,resources=things/finalizers,verbs=update
```

---

## 9. Add Admission Webhooks

### Mutating Webhook

Add the resource name to the mutating webhook loop in `cmd/manager/main.go`:

```go
for _, res := range []string{"database", "schema", ..., "thing"} {
```

### Validating Webhook

1. Create `webhook.NewThingValidator(scheme)` in `internal/webhook/`.
2. Register in `cmd/manager/main.go`:
   ```go
   hookServer.Register(
       "/validate-snowplane-v1alpha1-thing",
       &ctrlwebhook.Admission{Handler: webhook.NewThingValidator(scheme)},
   )
   ```
3. Add webhook configuration to `config/webhook/validating-webhook-configuration.yaml`.

The validator must:
- Call `spec.Validate()` on CREATE
- Check immutable fields on UPDATE (unless `force-new` annotation is set)
- Always allow DELETE

---

## 10. Add Sample Manifests

Create sample CRs in `config/samples/`:

### Using CR Reference (Snowplane-managed database)

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Thing
metadata:
  name: my-thing
spec:
  name: MY_THING
  providerRef:
    name: default
  databaseRef:
    name: my-database      # references a Database CR
  schemaRef:
    name: my-schema        # references a Schema CR (schema-level only)
```

### Using Raw Name (pre-existing Snowflake database)

```yaml
apiVersion: snowplane.hupe1980.github.io/v1alpha1
kind: Thing
metadata:
  name: my-thing
spec:
  name: MY_THING
  providerRef:
    name: default
  databaseName: ANALYTICS  # raw Snowflake name — no Database CR needed
  schemaName: '"ANALYTICS"."PUBLIC"'  # raw Snowflake FQN (schema-level only)
```

---

## 11. Write Tests

### Unit Tests

| File | What to Test |
|------|-------------|
| `api/v1alpha1/validation_test.go` | Valid spec, empty name, missing databaseRef+databaseName, both set (XOR error), range errors |
| `api/v1alpha1/deepcopy_test.go` | Add mutation test for new pointer fields |
| `internal/clients/snowflake/<resource>_test.go` | SQL generation, option validation, error handling |
| `internal/controller/<resource>/reconciler_test.go` | Full reconcile loop with mock service, conditions, status, ref resolution |
| `internal/webhook/validators_test.go` | Immutable field rejection, allow on no change, CREATE passthrough |

**Important:** All `DatabaseRef` and `SchemaRef` fields use pointer syntax in tests:

```go
// ✅ Correct — pointer type
DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "db"},
SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: "sch"},

// ❌ Wrong — value type (won't compile)
DatabaseRef: snowplanev1alpha1.LocalObjectReference{Name: "db"},
```

### Integration Tests

Add tests in `test/integration/<resource>_test.go` covering:
- Create lifecycle (from CR creation to Ready=True)
- Update triggers ALTER
- Delete with orphan policy
- Drift detection
- Wait for parent reference to become Ready (database/schema-level)
- Immutable field rejection

### Run All Tests

```bash
# Unit tests
just test

# Integration tests
just test-integration

# Webhook integration tests
just test-webhook-integration
```

---

## 12. Final Checklist

Use this checklist to verify completeness:

- [ ] **Types:** `api/v1alpha1/<resource>_types.go` with spec, status, show output, kubebuilder markers
- [ ] **Dual ref/name:** `DatabaseRef *LocalObjectReference` + `DatabaseName *string` with CEL XOR rule (if database/schema-level)
- [ ] **Validation:** `Validate()` method using `validateDatabaseSource`/`validateSchemaSource` helpers
- [ ] **ManagedResource:** All interface methods implemented on the root type
- [ ] **DeepCopy:** `just generate` run (never edit `zz_generated.deepcopy.go`)
- [ ] **CRD manifest:** `just generate && just sync-crds` run, maturity label added
- [ ] **Enum markers:** `+kubebuilder:validation:Enum` on all string-typed enum fields (quote values starting with digits, e.g. `"2XLARGE"`)
- [ ] **Snowflake client:** Observe/Create/Alter/Drop with proper identifier type
- [ ] **Adapter:** Implements `ResourceAdapter[T]` with nil-safe field indexers
- [ ] **Reference resolution:** `PreReconcile` uses `refresolver.PreReconcileDatabaseRef()` / `PreReconcileSchemaRef()` (handles resolution, conditions, deletion fallback, and logging)
- [ ] **Reconciler:** `NewReconciler` + `defaultServiceFactory` + helper functions
- [ ] **Drift detection:** `detectDrift` includes all immutable fields (`name`, `useRole`, container refs, type fields) with `immutable=true`, plus all mutable spec fields
- [ ] **Manager wiring:** Import alias, controller registration with maturity/alpha/disabled flags, RBAC markers
- [ ] **Webhooks:** Mutating (defaults) + validating (immutable fields + spec validation)
- [ ] **Samples:** CR YAML with both `databaseRef` and `databaseName` variants
- [ ] **Tests:** Unit + integration + webhook integration
- [ ] **Interface assertion:** `var _ reconciler.ResourceAdapter[...] = (*adapter)(nil)`
- [ ] **Safe type assertions:** Use `reconciler.AssertIdentifier[I]` / `AssertDetail[D]` / `AssertAlterOptions[A]` in error-returning methods; comma-ok pattern in non-error-returning methods (never bare `x.(T)` assertions)
- [ ] **ProviderConfig in-use guard:** Add the new type to the in-use check in `internal/controller/providerconfig/adapter.go`
