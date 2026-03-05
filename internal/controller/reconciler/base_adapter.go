// Package reconciler provides a generic, type-safe reconciliation framework.
// This file contains the BaseAdapter, a closure-based implementation of
// ResourceAdapter that eliminates per-resource adapter struct boilerplate.
package reconciler

import (
	"context"
	"reflect"

	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/tracked"
)

// BaseAdapter is a closure-based implementation of ResourceAdapter that
// eliminates the need for per-resource adapter structs and method
// declarations. Resource-specific behavior is injected via exported
// function fields.
//
// Optional interfaces (PreReconciler, WatchConfigurer, PostCreateHook,
// PostUpdateHook, CreateOrAlterSupporter, CascadeDropSupporter,
// CascadeDropper, LateInitializer) are satisfied with nil-safe defaults —
// set the corresponding function field to opt in.
type BaseAdapter[T ManagedResource, S any, D any] struct {
	// --- Required: identity ---

	// ResourceNameVal is the controller name (e.g. "database").
	ResourceNameVal string
	// FinalizerNameVal is the finalizer string for the resource.
	FinalizerNameVal string

	// --- Required: object factory ---

	// NewObjectFn returns a zero-value T for client.Get.
	NewObjectFn func() T

	// --- Required: service factory ---

	// ServiceFactoryFn creates the CRUD service from a Snowflake client.
	ServiceFactoryFn func(ctx context.Context, sfClient clientfactory.SnowflakeClient, useRole string) (S, func(context.Context), error)

	// --- Required: CRUD operations ---

	// BuildIdentifierFn constructs the Snowflake identifier from the object.
	BuildIdentifierFn func(obj T) (Identifier, error)
	// ObserveFn queries Snowflake for the current state.
	ObserveFn func(ctx context.Context, svc S, id Identifier) (*Observation[D], error)
	// CreateFn creates the resource in Snowflake.
	CreateFn func(ctx context.Context, svc S, obj T, id Identifier) error
	// AlterFn updates the resource in Snowflake.
	AlterFn func(ctx context.Context, svc S, opts AlterOptions) error
	// DropFn drops the resource from Snowflake.
	DropFn func(ctx context.Context, svc S, id Identifier) error

	// --- Required: spec management ---

	// ValidateImmutableFn checks resource-specific immutability.
	ValidateImmutableFn func(ctx context.Context, obj T) error
	// BuildAlterOptsFn diffs spec vs observation and returns alter options.
	BuildAlterOptsFn func(ctx context.Context, obj T, id Identifier, obs *Observation[D]) (AlterOptions, error)
	// ApplyObservationFn maps the observation into the CR's status fields.
	ApplyObservationFn func(obj T, obs *Observation[D])
	// DetectDriftFn compares spec vs observation for reporting.
	DetectDriftFn func(obj T, obs *Observation[D]) *drift.Result

	// --- Optional: tracked parameters ---

	// TrackedParamsFn returns actively-managed field names.
	// Default: uses reflection on obj.Spec via tracked.ComputeTracked.
	TrackedParamsFn func(obj T) []string

	// --- Optional: cascade drop ---

	// DropCascadeFn drops the resource with CASCADE. When nil,
	// CascadeDropper is satisfied but delegates to DropFn.
	DropCascadeFn func(ctx context.Context, svc S, id Identifier) error

	// --- Optional: pre-reconcile hook ---

	// PreReconcileFn runs resource-specific setup before reconciliation.
	// When nil, pre-reconcile is a no-op.
	PreReconcileFn func(ctx context.Context, obj T) error

	// --- Optional: watch configuration ---

	// SetupWatchesFn adds resource-specific watches.
	// When nil, no additional watches are configured.
	SetupWatchesFn SetupWatchesFunc

	// --- Optional: post-create hook ---

	// PostCreateFn runs after a successful create. When nil, no-op.
	PostCreateFn func(obj T)

	// --- Optional: post-update hook ---

	// PostUpdateFn runs after a successful update. When nil, no-op.
	PostUpdateFn func(obj T, altered bool, opts AlterOptions)

	// --- Optional: CREATE OR ALTER support ---

	// SupportsCoA indicates whether the resource supports CREATE OR ALTER.
	SupportsCoA bool

	// --- Optional: late initialization ---

	// LateInitializeFn fills unset spec fields from observed state.
	// When nil, late-initialization is skipped (returns false).
	LateInitializeFn func(obj T, obs *Observation[D]) bool

	// --- Optional: pre-flight check ---

	// PreFlightCheckFn validates prerequisite resources exist in Snowflake
	// (e.g. databases/schemas for raw databaseName/schemaName).
	// When nil, pre-flight check is skipped.
	PreFlightCheckFn func(ctx context.Context, sfClient clientfactory.SnowflakeClient, obj T) error
}

// ---------------------------------------------------------------------------
// ResourceAdapter interface — core methods
// ---------------------------------------------------------------------------

// ResourceName returns the human-readable resource name for logging and metrics.
func (a *BaseAdapter[T, S, D]) ResourceName() string { return a.ResourceNameVal }

// FinalizerName returns the finalizer string for this resource type.
func (a *BaseAdapter[T, S, D]) FinalizerName() string { return a.FinalizerNameVal }

// NewObject returns a new zero-value instance of the managed resource type.
func (a *BaseAdapter[T, S, D]) NewObject() T { return a.NewObjectFn() }

// ServiceFromClient creates a resource-specific service from the Snowflake client.
func (a *BaseAdapter[T, S, D]) ServiceFromClient(ctx context.Context, sfClient clientfactory.SnowflakeClient, useRole string) (S, func(context.Context), error) {
	return a.ServiceFactoryFn(ctx, sfClient, useRole)
}

// BuildIdentifier constructs the Snowflake identifier from the managed resource.
func (a *BaseAdapter[T, S, D]) BuildIdentifier(obj T) (Identifier, error) {
	return a.BuildIdentifierFn(obj)
}

// Observe checks whether the Snowflake resource exists and returns its current state.
func (a *BaseAdapter[T, S, D]) Observe(ctx context.Context, svc S, id Identifier) (*Observation[D], error) {
	return a.ObserveFn(ctx, svc, id)
}

// Create provisions the Snowflake resource.
func (a *BaseAdapter[T, S, D]) Create(ctx context.Context, svc S, obj T, id Identifier) error {
	return a.CreateFn(ctx, svc, obj, id)
}

// Alter applies changes to an existing Snowflake resource.
func (a *BaseAdapter[T, S, D]) Alter(ctx context.Context, svc S, opts AlterOptions) error {
	return a.AlterFn(ctx, svc, opts)
}

// Drop deletes the Snowflake resource.
func (a *BaseAdapter[T, S, D]) Drop(ctx context.Context, svc S, id Identifier) error {
	return a.DropFn(ctx, svc, id)
}

// ValidateImmutableFields checks that immutable spec fields have not been changed.
func (a *BaseAdapter[T, S, D]) ValidateImmutableFields(ctx context.Context, obj T) error {
	return a.ValidateImmutableFn(ctx, obj)
}

// BuildAlterOptions computes the ALTER options by comparing the desired spec to the observed state.
func (a *BaseAdapter[T, S, D]) BuildAlterOptions(ctx context.Context, obj T, id Identifier, obs *Observation[D]) (AlterOptions, error) {
	return a.BuildAlterOptsFn(ctx, obj, id, obs)
}

// ApplyObservation copies the observed Snowflake state into the resource's status.
func (a *BaseAdapter[T, S, D]) ApplyObservation(obj T, obs *Observation[D]) {
	a.ApplyObservationFn(obj, obs)
}

// ComputeTrackedParameters returns the list of Snowflake parameters currently managed by the spec.
func (a *BaseAdapter[T, S, D]) ComputeTrackedParameters(obj T) []string {
	if a.TrackedParamsFn != nil {
		return a.TrackedParamsFn(obj)
	}

	// Default: reflection-based tracked parameter computation via the Spec field.
	v := reflect.ValueOf(obj).Elem()

	spec := v.FieldByName("Spec")
	if !spec.IsValid() {
		return nil
	}

	return tracked.ComputeTracked(spec.Addr().Interface())
}

// DetectDrift compares the desired spec to the observed state and returns drift details.
func (a *BaseAdapter[T, S, D]) DetectDrift(obj T, obs *Observation[D]) *drift.Result {
	return a.DetectDriftFn(obj, obs)
}

// ---------------------------------------------------------------------------
// Optional interfaces — nil-safe defaults
// ---------------------------------------------------------------------------

// PreReconcile implements PreReconciler[T]. No-op when PreReconcileFn is nil.
func (a *BaseAdapter[T, S, D]) PreReconcile(ctx context.Context, obj T) error {
	if a.PreReconcileFn == nil {
		return nil
	}

	return a.PreReconcileFn(ctx, obj)
}

// SetupWatches implements WatchConfigurer. Returns nil when SetupWatchesFn is nil.
func (a *BaseAdapter[T, S, D]) SetupWatches() SetupWatchesFunc {
	return a.SetupWatchesFn
}

// PostCreate implements PostCreateHook[T]. No-op when PostCreateFn is nil.
func (a *BaseAdapter[T, S, D]) PostCreate(obj T) {
	if a.PostCreateFn != nil {
		a.PostCreateFn(obj)
	}
}

// PostUpdate implements PostUpdateHook[T]. No-op when PostUpdateFn is nil.
func (a *BaseAdapter[T, S, D]) PostUpdate(obj T, altered bool, opts AlterOptions) {
	if a.PostUpdateFn != nil {
		a.PostUpdateFn(obj, altered, opts)
	}
}

// SupportsCreateOrAlter implements CreateOrAlterSupporter.
func (a *BaseAdapter[T, S, D]) SupportsCreateOrAlter() bool {
	return a.SupportsCoA
}

// SupportsCascadeDrop returns true when DropCascadeFn is set.
// The reconciler checks this to determine whether to emit the
// "force-destroy not supported" warning.
func (a *BaseAdapter[T, S, D]) SupportsCascadeDrop() bool {
	return a.DropCascadeFn != nil
}

// DropCascade implements CascadeDropper[T, S]. Falls back to DropFn
// when DropCascadeFn is nil.
func (a *BaseAdapter[T, S, D]) DropCascade(ctx context.Context, svc S, id Identifier) error {
	if a.DropCascadeFn != nil {
		return a.DropCascadeFn(ctx, svc, id)
	}

	return a.DropFn(ctx, svc, id)
}

// LateInitialize implements LateInitializer[T, D]. Returns false
// when LateInitializeFn is nil (no fields modified).
func (a *BaseAdapter[T, S, D]) LateInitialize(obj T, obs *Observation[D]) bool {
	if a.LateInitializeFn == nil {
		return false
	}

	return a.LateInitializeFn(obj, obs)
}

// PreFlightCheck implements PreFlightChecker[T]. No-op when
// PreFlightCheckFn is nil.
func (a *BaseAdapter[T, S, D]) PreFlightCheck(ctx context.Context, sfClient clientfactory.SnowflakeClient, obj T) error {
	if a.PreFlightCheckFn == nil {
		return nil
	}

	return a.PreFlightCheckFn(ctx, sfClient, obj)
}
