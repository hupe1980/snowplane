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
| **Grant** | Role ref or inline | GrantPrivilegesToAccountRole, GrantPrivilegesToDatabaseRole |
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
    DatabaseRef *ObjectReference `json:"databaseRef,omitempty"`

    // +optional
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseName is immutable"
    DatabaseName *string `json:"databaseName,omitempty"`
}
```

### Optional Cross-Resource Ref/Name (e.g. Warehouse, Integration)

For **optional** references (e.g. warehouse, notification integration), use an at-most-one-of CEL rule.
Both fields may be nil (omitted), but setting both is rejected at admission:

```go
// +kubebuilder:validation:XValidation:rule="!(has(self.warehouseRef) && has(self.warehouseName))",message="at most one of spec.warehouseRef or spec.warehouseName may be set"
// +kubebuilder:validation:XValidation:rule="!has(self.warehouseName) || !self.warehouseName.contains('.')",message="spec.warehouseName must not contain dots"
type ThingSpec struct {
    // ...

    // +optional
    WarehouseRef *ObjectReference `json:"warehouseRef,omitempty"`

    // +optional
    // +kubebuilder:validation:MinLength=1
    WarehouseName *string `json:"warehouseName,omitempty"`
}
```

Resolve in the adapter's `PreReconcile` with `PreReconcileSourceRef`:

```go
if thing.Spec.WarehouseRef != nil || thing.Spec.WarehouseName != nil {
    name, err := refresolver.PreReconcileSourceRef[*v1alpha1.Warehouse](ctx, a.client, a.recorder, thing,
        thing.Namespace, thing.Spec.WarehouseRef, thing.Spec.WarehouseName, thing.Status.WarehouseName,
        "Warehouse", func() *v1alpha1.Warehouse { return &v1alpha1.Warehouse{} },
        v1alpha1.GroupVersion.WithKind("Warehouse"),
        func(w *v1alpha1.Warehouse) string { return w.Spec.Name })
    if err != nil {
        return err
    }
    thing.Status.WarehouseName = name
}
```

Validate in `Validate()` with the optional helper:

```go
if err := validateOptionalSourceRef("warehouse", s.WarehouseRef, s.WarehouseName); err != nil {
    errs = append(errs, err)
}
```

### List-Based Predecessor Refs (e.g. Task DAG)

For **list** fields where each entry can be a ref or a name, use a struct with per-entry XOR:

```go
type TaskPredecessor struct {
    // +optional
    Ref *ObjectReference `json:"ref,omitempty"`
    // +optional
    // +kubebuilder:validation:MinLength=1
    Name *string `json:"name,omitempty"`
}

// CEL per-entry XOR:
// +kubebuilder:validation:XValidation:rule="self.after.all(p, (has(p.ref) && !has(p.name)) || (!has(p.ref) && has(p.name)))",message="each after entry must set exactly one of ref or name"
```

Resolve all entries in a loop during `PreReconcile`, accumulating resolved names into a status slice.

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

1. **Observation type** — holds SHOW output. The `ShowOutput` field reuses the API type directly (`*v1alpha1.ThingShowOutput`) — do **not** define a separate internal ShowOutput struct.
2. **Options types** — `Create` and `Alter` options (implement `HasChanges() bool`)
3. **Client type** — `Observe`, `Create`, `Alter`, `Drop` methods

Use the SQL builder at `internal/clients/snowflake/sqlbuilder/` for safe, parameterised SQL generation instead of raw string concatenation.

> ⚠️ **Always check `b.Err()`:** All `buildCreateXxxSQL` functions must return `(string, error)` and check `b.Err()` before returning the SQL string. The builder accumulates validation errors (e.g., from `SetKeyword`/`SetQuotedKeyword`). Callers must wrap the error with `NewTerminalError`:

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

// NewReconciler returns a new Thing reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*v1alpha1.Thing, Service, *snowflake.ThingObservation] {
    return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
        reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
            return snowflake.NewThingClient(exec)
        }),
    )
}

// NewReconcilerWithServiceFactory lets callers inject a custom ServiceFactory
// for integration tests with mock Snowflake services.
func NewReconcilerWithServiceFactory(
    c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder,
    rl *ratelimit.Limiter, sf ServiceFactory,
) *reconciler.GenericReconciler[*v1alpha1.Thing, Service, *snowflake.ThingObservation] {
    return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates a BaseAdapter with resource-specific closures.
func newAdapter(c client.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*v1alpha1.Thing, Service, *snowflake.ThingObservation] {
    return &reconciler.BaseAdapter[*v1alpha1.Thing, Service, *snowflake.ThingObservation]{
        ResourceNameVal:  "thing",
        FinalizerNameVal: finalizerName,
        NewObjectFn:      func() *v1alpha1.Thing { return &v1alpha1.Thing{} },
        ServiceFactoryFn: sf,
        BuildIdentifierFn: func(obj *v1alpha1.Thing) (reconciler.Identifier, error) {
            return snowflake.NewSchemaObjectIdentifier(obj.Status.DatabaseName, obj.Status.SchemaName, obj.Spec.Name), nil
        },
        ObserveFn: reconciler.MakeObserve(
            func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.ThingObservation, error) {
                return svc.Observe(ctx, id)
            },
            func(obs *snowflake.ThingObservation) bool { return obs.Exists },
        ),
        CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *v1alpha1.Thing, id snowflake.SchemaObjectIdentifier) error {
            opts := buildCreateOptions(obj, id)
            opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()
            return svc.Create(ctx, opts)
        }),
        AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterThingOptions) error {
            return svc.Alter(ctx, *opts)
        }),
        DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
            return svc.Drop(ctx, id)
        }),
        ValidateImmutableFn: validateImmutableFields,
        BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *v1alpha1.Thing, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.ThingObservation]) (reconciler.AlterOptions, error) {
            opts := buildAlterOptions(obj, id, obs.Detail)
            return &opts, nil
        }),
        ApplyObservationFn: func(obj *v1alpha1.Thing, obs *reconciler.Observation[*snowflake.ThingObservation]) {
            applyObservation(obj, obs.Detail)
        },
        DetectDriftFn: func(obj *v1alpha1.Thing, obs *reconciler.Observation[*snowflake.ThingObservation]) *drift.Result {
            return detectDrift(obj, obs.Detail)
        },
        SupportsCoA: true,
        // Optional: PreReconcile for reference resolution
        PreReconcileFn: func(ctx context.Context, thing *v1alpha1.Thing) error {
            dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, thing, ...)
            // ...
        },
    }
}
```

### BaseAdapter Configuration

The `newAdapter` function configures a `BaseAdapter[T, S, D]` with resource-specific closures. The `BaseAdapter` implements the **required** `ResourceAdapter[T, S, D]` interface:

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
| `ApplyObservation` | Maps observation into the CR status (ShowOutput is assigned directly since both layers share the same type) |
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

Then `BaseAdapter` handles this automatically via reflection — no need to set `TrackedParamsFn` unless you need custom behavior.

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

In addition, `BaseAdapter` supports **optional capabilities** via function fields. Set the corresponding field to opt in (nil = disabled):

| BaseAdapter Field | Optional Interface | When Needed |
|:------------------|:-------------------|:------------|
| `PreReconcileFn` | `PreReconciler[T]` | Reference resolution (database/schema-level resources) |
| `SetupWatchesFn` | `WatchConfigurer` | Add watches for parent resources (e.g. Schema watches Database) |
| `PostCreateFn` | `PostCreateHook[T]` | Logic after successful create, before status patch |
| `PostUpdateFn` | `PostUpdateHook[T]` | Logic after successful update (e.g. hash password) |
| `SupportsCoA` | `CreateOrAlterSupporter` | Enable `CREATE OR ALTER` SQL syntax |
| `DropCascadeFn` | `CascadeDropper[T,S]` + `CascadeDropSupporter` | Enable `DROP ... CASCADE` support |
| `LateInitializeFn` | `LateInitializer[T,D]` | Fill unset spec fields from observed state |

For database-level resources, capture `c` and `recorder` in the `PreReconcileFn` closure:

```go
PreReconcileFn: func(ctx context.Context, thing *v1alpha1.Thing) error {
    dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, thing,
        thing.Namespace, thing.Spec.DatabaseRef, thing.Spec.DatabaseName, thing.Status.DatabaseName)
    if err != nil {
        return err
    }
    thing.Status.DatabaseName = dbFQN
    return nil
},
```

Interface assertion is provided by BaseAdapter itself — no per-resource assertion needed.

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

Controller names for `--disable-controllers` validation are **auto-derived** from the registration table — no manual map update is needed. Simply add your entry to the registration table (see below) and validation is handled automatically.

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
> The controller name is auto-derived from the registration table entry. The `--disable-controllers` flag validates against the same table, so no separate map update is needed.

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
- [ ] Dual ref/name with CEL XOR rule (database/schema-level, plus any optional refs like warehouse/integration)
- [ ] Optional refs: at-most-one-of CEL, `validateOptionalSourceRef`, `PreReconcileSourceRef` with guard
- [ ] List refs (if any): per-entry XOR CEL, loop-based resolution in PreReconcile
- [ ] `Validate()` method using shared helpers
- [ ] Type registered in `hack/gen-accessors/main.go`, `just generate` run
- [ ] CRD manifest with maturity label (`just sync-crds` — copies to Helm chart)
- [ ] Snowflake client with Observe/Create/Alter/Drop (use `sqlbuilder/`)
- [ ] `newAdapter()` returning `*reconciler.BaseAdapter[...]` with all closures
- [ ] Optional capabilities via function fields (`PreReconcileFn`, `SetupWatchesFn`, `SupportsCoA`, etc.)
- [ ] `NewReconciler` / `NewReconcilerWithServiceFactory` using `reconciler.NewGenericReconciler`
- [ ] CRUD helpers: `MakeObserve`, `MakeCreate`, `MakeAlter`, `MakeDrop`, `MakeBuildAlterOpts`
- [ ] Manager wiring: RBAC markers, registration table entry
- [ ] Kustomize RBAC: all three resource blocks in `config/rbac/role.yaml`
- [ ] FieldExport registration: `ValidFieldExportSourceKinds`, CEL whitelist, `sourceResourceTypes()`
- [ ] ProviderConfig in-use guard: `managedResourceTypes()`
- [ ] CEL validation markers (`just generate && just sync-crds`)
- [ ] Sample CRs in `config/samples/`
- [ ] Tests: unit, integration, CEL validation
- [ ] Docs: api-reference, crd-lifecycle, drift-detection
