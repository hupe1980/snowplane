---
layout: default
title: Adding a Resource
parent: Development
nav_order: 2
description: "Step-by-step guide to add a new Snowflake resource to Snowplane."
---

# Adding a New Managed Resource
{: .fs-8 }

Every step needed to add a new Snowflake resource to Snowplane — from CRD types to integration tests.
{: .fs-5 .fw-300 }

{: .note }
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
9. [Register as FieldExport Source](#9-register-as-fieldexport-source)
10. [Register in ProviderConfig In-Use Guard](#10-register-in-providerconfig-in-use-guard)
11. [Add CEL Validation Markers](#11-add-cel-validation-markers)
12. [Add Sample Manifests](#12-add-sample-manifests)
13. [Write Tests](#13-write-tests)
14. [Update Documentation](#14-update-documentation)
15. [Final Checklist](#15-final-checklist)

---

## 1. Decide the Resource Level

| Level | Parent References | Examples |
|:------|:------------------|:---------|
| **Account** | None | Database, Warehouse, AccountRole, User |
| **Database** | `databaseRef` / `databaseName` | Schema, DatabaseRole |
| **Schema** | `databaseRef` + `schemaRef` | Table, View, Stage, Task, Stream |
| **Grant** | Role ref or inline | AccountRoleGrant, DatabaseRoleGrant |
| **Assignment** | Role ref, target ref | AccountRoleAssignment, DatabaseRoleAssignment |

---

## 2. Define CRD Types

Create `api/v1alpha1/<resource>_types.go`.

### Account-Level Resource

```go
type ThingSpec struct {
    CommonSpec `json:",inline"`

    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.name is immutable"
    Name string `json:"name"`

    Comment *string `json:"comment,omitempty"`
}
```

### Database-Level Resource

Add dual `databaseRef`/`databaseName` with CEL XOR rule:

```go
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
type ThingSpec struct {
    CommonSpec `json:",inline"`

    Name string `json:"name"`

    // +optional
    // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseRef is immutable"
    DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

    // +optional
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseName is immutable"
    DatabaseName *string `json:"databaseName,omitempty"`
}
```

### Status Type

```go
type ThingStatus struct {
    CommonStatus `json:",inline"`
    DatabaseName      string            `json:"databaseName,omitempty"`
    ShowOutput        *ThingShowOutput   `json:"showOutput,omitempty"`
    TrackedParameters []string           `json:"trackedParameters,omitempty"`
}
```

### Kubebuilder Root Markers

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type="string",JSONPath=".status.showOutput.name"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Synced",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:categories=snowplane
type Thing struct { ... }
```

### Scheme Registration

Every types file must register with the scheme builder. Add at the bottom:

```go
func init() {
    SchemeBuilder.Register(&Thing{}, &ThingList{})
}
```

Without this, the controller-runtime client cannot work with your type.

---

## 3. Add Validation

In `api/v1alpha1/validation.go`:

```go
func (s *ThingSpec) Validate() error {
    var errs []error
    if s.Name == "" {
        errs = append(errs, errors.New("spec.name is required"))
    }
    if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
        errs = append(errs, err)
    }
    if err := s.CommonSpec.Validate(); err != nil {
        errs = append(errs, err)
    }
    return errors.Join(errs...)
}
```

---

## 4. Implement the ManagedResource Interface

The accessor methods are **code-generated**. Register your type in `hack/gen-accessors/main.go`:

```go
// Standard resource with ShowOutput.Owner:
{TypeName: "Thing", Receiver: "t", Owner: OwnerFromShowOutputOwner, TrackedParams: TrackedParamsFromStatus},

// Resource without owner column:
{TypeName: "Thing", Receiver: "t", Owner: OwnerEmpty, TrackedParams: TrackedParamsFromStatus},

// Grant resource (custom GetSpecName):
{TypeName: "Thing", Receiver: "t", SkipGetSpecName: true, Owner: OwnerFromShowOutputGrantedBy, TrackedParams: TrackedParamsNil},
```

Then regenerate:

```bash
just generate
```

Generated methods include: `GetConditions`, `SetConditions`, `GetDeletionPolicy`, `GetSpecName`, `GetFullyQualifiedName`, `GetProviderRef`, `GetUseRole`, `Get/SetObservedGeneration`, `Get/SetLastAppliedSpecHash`, `Get/SetTrackedParametersList`, `GetOwner`, `ValidateSpec`, `ComputeSpecHash`.

---

## 5. Regenerate DeepCopy & CRD Manifests

```bash
just generate
just sync-crds
```

`just sync-crds` copies the generated CRD YAMLs from `config/crd/bases/` into `charts/snowplane/crds/` for Helm-based deployments. Both locations must stay in sync.

Add a maturity label to the generated CRD manifest:

```yaml
metadata:
  labels:
    snowplane.hupe1980.github.io/maturity: alpha
```

---

## 6. Create the Snowflake Client

Create `internal/clients/snowflake/<resource>.go` with:

1. **Observation type** — holds SHOW output
2. **Options types** — `Create` and `Alter` options (implement `HasChanges() bool`)
3. **Client type** — `Observe`, `Create`, `Alter`, `Drop` methods

Use the SQL builder at `internal/clients/snowflake/sqlbuilder/` for safe, parameterised SQL generation instead of raw string concatenation:

```go
type ThingClient struct {
    exec SQLExecutor
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

```go
// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake.
type Service interface {
    Observe(ctx context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.ThingObservation, error)
    Create(ctx context.Context, opts snowflake.CreateThingOptions) error
    Alter(ctx context.Context, opts snowflake.AlterThingOptions) error
    Drop(ctx context.Context, id snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// defaultServiceFactory is the production ServiceFactory used by NewReconciler.
func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
    sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
    if err != nil {
        return nil, nil, err
    }
    return snowflake.NewThingClient(sfC), cleanup, nil
}

// NewReconciler returns a new Thing reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*v1alpha1.Thing, Service, *snowflake.ThingObservation] {
    a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
    return &reconciler.GenericReconciler[*v1alpha1.Thing, Service, *snowflake.ThingObservation]{
        Client:      c,
        Factory:     factory,
        Recorder:    recorder,
        RateLimiter: rl,
        Adapter:     a,
    }
}

// NewReconcilerWithServiceFactory lets callers inject a custom ServiceFactory
// for integration tests with mock Snowflake services.
func NewReconcilerWithServiceFactory(
    c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder,
    rl *ratelimit.Limiter, sf ServiceFactory,
) *reconciler.GenericReconciler[*v1alpha1.Thing, Service, *snowflake.ThingObservation] {
    a := &adapter{client: c, recorder: recorder, newService: sf}
    return &reconciler.GenericReconciler[*v1alpha1.Thing, Service, *snowflake.ThingObservation]{
        Client:      c,
        Factory:     factory,
        Recorder:    recorder,
        RateLimiter: rl,
        Adapter:     a,
    }
}
```

### `adapter.go`

The adapter implements the **required** `ResourceAdapter[T, S, D]` interface:

| Method | Purpose |
|:-------|:--------|
| `ResourceName` | Controller name (e.g. `"thing"`) |
| `FinalizerName` | Finalizer string for the resource |
| `NewObject` | Returns zero-value `T` for `client.Get` |
| `ServiceFromClient` | Creates the CRUD service from a Snowflake client |
| `BuildIdentifier` | Constructs the Snowflake identifier from the object |
| `Observe` | Queries Snowflake for current state |
| `Create` | Creates the resource in Snowflake |
| `Alter` | Updates the resource in Snowflake |
| `Drop` | Drops the resource from Snowflake |
| `ValidateImmutableFields` | Checks resource-specific immutability |
| `BuildAlterOptions` | Diffs spec vs observation → alter options |
| `ApplyObservation` | Maps observation into the CR status |
| `ComputeTrackedParameters` | Returns actively-managed field names (via `tracked.ComputeTracked`) |
| `DetectDrift` | Compares spec vs observation for reporting |

#### Tracked Parameter Struct Tags

Instead of hand-writing `ComputeTrackedParameters` and `computeUnsetFields` per resource, annotate spec fields with the `snowflake` struct tag and use the generic `internal/tracked` package:

```go
type ThingSpec struct {
    // ... required/immutable fields (no snowflake tag) ...
    Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
    Size    *string `json:"size,omitempty"    snowflake:"SIZE"`
}
```

Then in your adapter:

```go
func (a *adapter) ComputeTrackedParameters(obj *v1alpha1.Thing) []string {
    return tracked.ComputeTracked(&obj.Spec)
}
```

And in your reconciler's `buildAlterOptions`:

```go
opts.UnsetFields = tracked.ComputeUnset(&thing.Spec, thing.Status.TrackedParameters)
```

**Tag options:**

| Syntax | Meaning |
|:-------|:--------|
| `snowflake:"PARAM_NAME"` | Tracked when pointer is non-nil or slice is non-empty |
| `snowflake:"PARAM_NAME,always"` | Always included in the tracked list |
| `snowflake:"PARAM_NAME,nounset"` | Tracked but excluded from UNSET computation |
| `snowflake:"PREFIX_,prefix"` | Map keys: each key becomes `PREFIX_<key>` |
| `snowflake:"-"` | Explicitly skipped |

Nested struct-pointer fields (union types like `spec.Email *EmailConfig`) are recursed into automatically when non-nil.

In addition, there are **optional interfaces** the reconciler detects via type assertion. Adapters that don't implement them get sensible defaults (no-op):

| Optional Interface | Method | When Needed |
|:-------------------|:-------|:------------|
| `PreReconciler[T]` | `PreReconcile(ctx, obj)` | Reference resolution (database/schema-level resources) |
| `WatchConfigurer` | `SetupWatches()` | Add watches for parent resources (e.g. Schema watches Database) |
| `PostCreateHook[T]` | `PostCreate(obj)` | Logic after successful create, before status patch |
| `PostUpdateHook[T]` | `PostUpdate(obj, altered, alterOpts)` | Logic after successful update (e.g. hash password) |
| `CreateOrAlterSupporter` | `SupportsCreateOrAlter()` | Enable `CREATE OR ALTER` SQL syntax |

For database-level resources, use shared reference resolution helpers:

```go
func (a *adapter) PreReconcile(ctx context.Context, thing *v1alpha1.Thing) error {
    dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, thing,
        thing.Namespace, thing.Spec.DatabaseRef, thing.Spec.DatabaseName, thing.Status.DatabaseName)
    if err != nil {
        return err
    }
    thing.Status.DatabaseName = dbFQN
    return nil
}
```

Always add the interface assertion:

```go
var _ reconciler.ResourceAdapter[*v1alpha1.Thing, Service, *snowflake.ThingObservation] = (*adapter)(nil)
```

---

## 8. Wire into the Manager

### RBAC Markers

Add kubebuilder RBAC markers in `cmd/manager/main.go`:

```go
//+kubebuilder:rbac:groups=snowplane.hupe1980.github.io,resources=things,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=snowplane.hupe1980.github.io,resources=things/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=snowplane.hupe1980.github.io,resources=things/finalizers,verbs=update
```

### Kustomize RBAC

Add the new resource to all three resource blocks in `config/rbac/role.yaml`:

1. Base resources list (verbs: `get`, `list`, `watch`, `create`, `update`, `patch`, `delete`)
2. `/status` sub-resources list (verbs: `get`, `update`, `patch`)
3. `/finalizers` sub-resources list (verbs: `update`)

{: .note }
> The Helm chart includes an automated `rbac_coverage_test.go` that will fail if any registered CRD type is missing from the RBAC definitions.

### Controller Name Map

Add the controller name to the `validControllerNames` map in `cmd/manager/main.go`:

```go
var validControllerNames = map[string]bool{
    // ... existing entries ...
    "thing": true,
}
```

This map governs `--disable-controllers` validation. Without updating it, the new controller cannot be selectively disabled.

### Registration Table

Add an entry to the declarative controller registration table in `cmd/manager/main.go`:

```go
controllers := []struct {
    name string
    ctrl reconciler.Registerable
}{
    // ... existing entries ...
    {"thing", thingctl.NewReconciler(kc, factory, controllerRec("thing"), rl)},
}
```

The shared `SetupConfig` (circuit breaker, requeue interval, snowflake op timeout, maturity, alpha enabled, max concurrent reconciles) is applied automatically via the `Setup()` method — no per-controller `With*()` chains needed.

{: .note }
> There is an existing sync test (`TestValidControllerNames_MatchesRegistrationTable`) that verifies the controller name map and the registration table stay in sync.

---

## 9. Register as FieldExport Source

If your resource is a standard managed resource (not a grant or assignment), register it as a valid FieldExport source in **three** places:

1. **Go validation map** — `ValidFieldExportSourceKinds` in `api/v1alpha1/validation.go`
2. **CEL kind whitelist** — the `XValidation` marker on `FieldExportResourceRef.Kind` in `api/v1alpha1/fieldexport_types.go`
3. **Reactive watch list** — `sourceResourceTypes()` in `internal/controller/fieldexport/reconciler.go`

{: .warning }
> Existing sync tests (`TestCELWhitelist_MatchesValidFieldExportSourceKinds` and `TestSourceResourceTypes_MatchesValidKinds`) will **fail** if these three lists are out of sync. Run tests after updating.

---

## 10. Register in ProviderConfig In-Use Guard

Add the new type to `managedResourceTypes()` in `internal/controller/providerconfig/reconciler.go`:

```go
{proto: &snowplanev1alpha1.Thing{}, newList: func() client.ObjectList { return &snowplanev1alpha1.ThingList{} }},
```

This function registers field indexers and powers the `isInUse()` check that prevents ProviderConfig deletion while resources still reference it.

---

## 11. Add CEL Validation Markers

```go
// Immutable required field:
//+kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable"

// Immutable optional pointer field:
//+kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable"

// Dot-validation for schema-scoped resources (prevent accidental FQN injection):
//+kubebuilder:validation:XValidation:rule="!self.databaseName.contains('.')",message="spec.databaseName must not contain dots — use separate databaseName and schemaName fields"
//+kubebuilder:validation:XValidation:rule="!self.schemaName.contains('.')",message="spec.schemaName must not contain dots — use separate databaseName and schemaName fields"
```

```bash
just generate && just sync-crds
```

---

## 12. Add Sample Manifests

Create examples in `config/samples/` with both `databaseRef` and `databaseName` variants.

---

## 13. Write Tests

| File | What to Test |
|:-----|:-------------|
| `api/v1alpha1/validation_test.go` | Valid spec, empty name, XOR errors |
| `api/v1alpha1/deepcopy_test.go` | Mutation test for new pointer fields |
| `internal/clients/snowflake/<resource>_test.go` | SQL generation, option validation |
| `internal/controller/<resource>/reconciler_test.go` | Full loop with mock service |
| `test/integration/<resource>_test.go` | Full CRD → reconciler pipeline with envtest |
| `test/integration/cel_validation_test.go` | CEL immutability and cross-validation rules |

### Standard Reconciler Test Suite

Every resource controller must include the standard behavioral test suite. Add
this to `reconciler_test.go`:

```go
func TestReconcile_StandardSuite(t *testing.T) {
    t.Parallel()

    testutil.ReconcileSuiteConfig{
        NewReconciler: func(objs ...runtime.Object) testutil.ReconcilerSetup {
            r := newTestReconciler(&mockService{}, objs...)
            return testutil.ReconcilerSetup{Reconciler: r, Client: r.Client}
        },
        NewFixture: func(name, ns string) client.Object {
            return newTestMyResource(name, ns)
        },
        NewBlankObject: func() client.Object {
            return &snowplanev1alpha1.MyResource{}
        },
        FinalizerName: finalizerName,
    }.Run(t)
}
```

For resources with cross-resource refs (e.g. DatabaseRef, SchemaRef), add
`PrereqObjects` to seed the fake client with ready prerequisite resources:

```go
PrereqObjects: func() []runtime.Object {
    db := newTestDB("my-db", "default")
    return []runtime.Object{db}
},
```

The suite automatically runs four sub-tests: **CRNotFound**,
**ProviderConfigNotFound**, **ProviderConfigNotReady**, and **AddsFinalizer**.

{: .note }
> Several sync tests run automatically and will catch missing registrations:
> - `TestCELWhitelist_MatchesValidFieldExportSourceKinds` — FieldExport CEL vs Go map sync
> - `TestSourceResourceTypes_MatchesValidKinds` — FieldExport reactive watch coverage
> - `TestValidControllerNames_MatchesRegistrationTable` — controller name map vs registration table
> - `charts/snowplane/rbac_coverage_test.go` — Helm RBAC covers all CRD types

---

## 14. Update Documentation

Update these docs to include the new resource:

| File | What to Update |
|:-----|:---------------|
| `docs/api-reference.md` | Add field-level documentation table |
| `docs/crd-lifecycle.md` | Add to maturity classification table |
| `docs/drift-detection.md` | Add to supported resources and drift fields |

---

## 15. Final Checklist

- [ ] Types with spec, status, show output, kubebuilder markers
- [ ] `SchemeBuilder.Register(&Thing{}, &ThingList{})` in `init()`
- [ ] Dual ref/name with CEL XOR rule (database/schema-level)
- [ ] `Validate()` method using shared helpers
- [ ] Type registered in `hack/gen-accessors/main.go`, `just generate` run
- [ ] CRD manifest with maturity label (`just sync-crds` — copies to Helm chart)
- [ ] Snowflake client with Observe/Create/Alter/Drop (use `sqlbuilder/`)
- [ ] `ResourceAdapter` with all required methods
- [ ] Optional adapter interfaces as needed (`PreReconciler`, `WatchConfigurer`, `CreateOrAlterSupporter`, etc.)
- [ ] `defaultServiceFactory` + `NewReconcilerWithServiceFactory` for test injection
- [ ] Interface assertion: `var _ reconciler.ResourceAdapter[...] = (*adapter)(nil)`
- [ ] Safe type assertions via `reconciler.AssertIdentifier[I]` / `AssertAlterOptions[A]`
- [ ] Manager wiring: RBAC markers, `validControllerNames`, registration table entry
- [ ] Kustomize RBAC: all three resource blocks in `config/rbac/role.yaml`
- [ ] FieldExport registration: `ValidFieldExportSourceKinds`, CEL whitelist, `sourceResourceTypes()`
- [ ] ProviderConfig in-use guard: `managedResourceTypes()`
- [ ] CEL validation markers (`just generate && just sync-crds`)
- [ ] Sample CRs in `config/samples/`
- [ ] Tests: unit, integration, CEL validation
- [ ] Docs: api-reference, crd-lifecycle, drift-detection
