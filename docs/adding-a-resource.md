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
9. [Add CEL Validation Markers](#9-add-cel-validation-markers)
10. [Add Sample Manifests](#10-add-sample-manifests)
11. [Write Tests](#11-write-tests)
12. [Final Checklist](#12-final-checklist)

---

## 1. Decide the Resource Level

| Level | Parent References | Examples |
|:------|:------------------|:---------|
| **Account** | None | Database, Warehouse, AccountRole, User |
| **Database** | `databaseRef` / `databaseName` | Schema, DatabaseRole |
| **Schema** | `databaseRef` + `schemaRef` | Table, View, Stage, Task, Stream |
| **Grant** | Role ref or inline | AccountRoleGrant, DatabaseRoleGrant |

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

The 16 accessor methods are **code-generated**. Register your type in `hack/gen-accessors/main.go`:

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

Generated methods: `GetConditions`, `SetConditions`, `GetDeletionPolicy`, `GetSpecName`, `GetProviderRef`, `GetUseRole`, `Get/SetObservedGeneration`, `Get/SetLastAppliedSpecHash`, `Get/SetTrackedParametersList`, `GetOwner`, `ValidateSpec`, `ComputeSpecHash`.

---

## 5. Regenerate DeepCopy & CRD Manifests

```bash
just generate
just sync-crds
```

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
type Service interface {
    Observe(ctx context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.ThingObservation, error)
    Create(ctx context.Context, opts snowflake.CreateThingOptions) error
    Alter(ctx context.Context, opts snowflake.AlterThingOptions) error
    Drop(ctx context.Context, id snowflake.SchemaObjectIdentifier) error
}

func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[...] { ... }
```

### `adapter.go`

Implements `ResourceAdapter[T, S, D]` with:
- `PreReconcile` — Reference resolution (database/schema-level)
- `BuildIdentifier` — Snowflake identifier construction
- `DetectDrift` — Field-level comparisons
- `SetupWatches` — Field indexers and parent watches

For database-level resources, use shared helpers:

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
var _ reconciler.ResourceAdapter[*v1alpha1.Thing] = (*adapter)(nil)
```

---

## 8. Wire into the Manager

In `cmd/manager/main.go`:

```go
//+kubebuilder:rbac:groups=snowplane.hupe1980.github.io,resources=things,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=snowplane.hupe1980.github.io,resources=things/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=snowplane.hupe1980.github.io,resources=things/finalizers,verbs=update
```

```go
if err := thingctl.NewReconciler(
    mgr.GetClient(), factory,
    sanitize.NewSafeRecorderFromEvents(mgr.GetEventRecorder("thing-controller")),
    rl,
).WithCircuitBreaker(cb).WithRequeueInterval(requeueInterval).WithMaturity("alpha").WithAlphaEnabled(enableAlphaResources).WithDisabled(disabled["thing"]).SetupWithManager(mgr, maxConcurrentReconciles); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "Thing")
    os.Exit(1)
}
```

---

## 9. Add CEL Validation Markers

```go
// Immutable required field:
//+kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable"

// Immutable optional pointer field:
//+kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable"
```

```bash
just generate && just sync-crds
```

---

## 10. Add Sample Manifests

Create examples in `config/samples/` with both `databaseRef` and `databaseName` variants.

---

## 11. Write Tests

| File | What to Test |
|:-----|:-------------|
| `validation_test.go` | Valid spec, empty name, XOR errors |
| `deepcopy_test.go` | Mutation test for new pointer fields |
| `snowflake/<resource>_test.go` | SQL generation, option validation |
| `controller/<resource>/reconciler_test.go` | Full loop with mock service |
| `test/integration/<resource>_test.go` | Full CRD → reconciler pipeline |

---

## 12. Final Checklist

- [ ] Types with spec, status, show output, kubebuilder markers
- [ ] Dual ref/name with CEL XOR rule (database/schema-level)
- [ ] `Validate()` method using shared helpers
- [ ] Type registered in `hack/gen-accessors/main.go`, `just generate` run
- [ ] CRD manifest with maturity label (`just sync-crds`)
- [ ] Snowflake client with Observe/Create/Alter/Drop
- [ ] `ResourceAdapter` with nil-safe field indexers
- [ ] Reference resolution using `refresolver.PreReconcileDatabaseRef()` / `PreReconcileSchemaRef()`
- [ ] Drift detection with all immutable + mutable fields
- [ ] Manager wiring with RBAC markers, maturity, alpha, disabled flags
- [ ] CEL validation markers (`just generate && just sync-crds`)
- [ ] Sample CRs in `config/samples/`
- [ ] Unit + integration tests
- [ ] Interface assertion: `var _ reconciler.ResourceAdapter[...] = (*adapter)(nil)`
- [ ] Safe type assertions via `reconciler.AssertIdentifier[I]` / `AssertAlterOptions[A]`
- [ ] ProviderConfig in-use guard updated
