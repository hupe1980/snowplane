// Package reconciler provides a generic, type-safe reconciliation framework
// for Snowflake-managed Kubernetes resources. It encodes the shared
// Observe-Diff-Apply state machine once, delegating resource-specific
// behaviour to a ResourceAdapter implementation.
package reconciler

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/utils/conditions"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

// AlterOptions represents Snowflake ALTER options that can report
// whether any changes exist.
type AlterOptions interface {
	HasChanges() bool
}

// ManagedResource is the constraint satisfied by every Snowplane CRD type.
// It combines client.Object, conditions support, and the accessors the
// generic reconciler needs for the shared state machine.
type ManagedResource interface {
	client.Object
	conditions.ConditionedObject

	// GetDeletionPolicy returns the CR's deletion policy.
	GetDeletionPolicy() snowplanev1alpha1.DeletionPolicy

	// Spec accessors used by the generic state machine.
	GetSpecName() string
	GetProviderRef() snowplanev1alpha1.ProviderReference
	GetUseRole() *string
	GetPaused() bool
	GetManagementPolicies() snowplanev1alpha1.ManagementPolicies
	SetCreateOrAlter(v *bool)

	// Status accessors.
	GetObservedGeneration() int64
	SetObservedGeneration(int64)
	GetLastAppliedSpecHash() string
	SetLastAppliedSpecHash(string)
	GetLastReconcileTime() *metav1.Time
	SetLastReconcileTime(*metav1.Time)
	GetTrackedParametersList() []string
	SetTrackedParametersList([]string)
	GetOwner() string

	// ValidateSpec validates the spec and returns an error if invalid.
	ValidateSpec() error

	// ComputeSpecHash returns a hash of the spec for drift detection.
	ComputeSpecHash() (string, error)
}

// Identifier is the minimal interface satisfied by all Snowflake resource
// identifiers (AccountObjectIdentifier, DatabaseObjectIdentifier,
// SchemaObjectIdentifier, GrantIdentifier). It replaces the previous `any`
// type, constraining identifier parameters at compile time (B-4).
type Identifier interface {
	FullyQualifiedName() string
	String() string
}

// Observation is the generic observation from Snowflake.
// The type parameter D holds the resource-specific observation data,
// providing compile-time safety instead of runtime type assertions.
type Observation[D any] struct {
	// Exists indicates whether the resource exists in Snowflake.
	Exists bool
	// Detail holds the resource-specific observation data.
	Detail D
}

// ResourceAdapter encapsulates all resource-specific behaviour for the generic
// reconciler. Each managed resource type (Database, Schema, Warehouse, User,
// AccountRole) implements this interface.
//
// The interface follows the Interface Segregation Principle: only the 14 core
// methods that every adapter must implement are required. Optional behaviours
// (PreReconcile, SetupWatches, PostCreate, PostUpdate, SupportsCreateOrAlter)
// are expressed as separate interfaces that adapters opt into. The reconciler
// detects these via type assertions, applying sensible defaults when absent.
// This eliminates the trivial no-op implementations that most adapters
// previously required.
//
// Type parameters:
//   - T: the CRD type (e.g. *snowplanev1alpha1.Database)
//   - S: the Snowflake CRUD service interface (e.g. database.Service)
//   - D: the resource-specific observation detail type (e.g. *snowflake.DatabaseObservation)
//
// Using type parameters for S and D eliminates runtime type assertions in
// every adapter method. A wrong service or observation type is now a
// compile-time error instead of a runtime panic.
type ResourceAdapter[T ManagedResource, S any, D any] interface {
	// ResourceName returns the controller name (e.g. "database").
	ResourceName() string

	// FinalizerName returns the finalizer string for the resource.
	FinalizerName() string

	// NewObject returns a zero-value T for client.Get.
	NewObject() T

	// ServiceFromClient creates the CRUD service from a Snowflake client.
	// When useRole is non-empty the factory pins a connection, switches to
	// that role, and returns a cleanup function.
	ServiceFromClient(ctx context.Context, sfClient clientfactory.SnowflakeClient, useRole string) (S, func(context.Context), error)

	// BuildIdentifier constructs the Snowflake identifier from the object.
	BuildIdentifier(obj T) (Identifier, error)

	// Observe queries Snowflake for the current state.
	Observe(ctx context.Context, svc S, id Identifier) (*Observation[D], error)

	// Create creates the resource in Snowflake.
	Create(ctx context.Context, svc S, obj T, id Identifier) error

	// Alter updates the resource in Snowflake.
	Alter(ctx context.Context, svc S, opts AlterOptions) error

	// Drop drops the resource from Snowflake.
	Drop(ctx context.Context, svc S, id Identifier) error

	// ValidateImmutableFields checks resource-specific immutability.
	ValidateImmutableFields(ctx context.Context, obj T) error

	// BuildAlterOptions diffs spec vs observation and returns alter options.
	BuildAlterOptions(ctx context.Context, obj T, id Identifier, obs *Observation[D]) (AlterOptions, error)

	// ApplyObservation maps the observation into the CR's status fields.
	ApplyObservation(obj T, obs *Observation[D])

	// ComputeTrackedParameters returns actively-managed field names.
	ComputeTrackedParameters(obj T) []string

	// DetectDrift compares spec vs observation for reporting.
	DetectDrift(obj T, obs *Observation[D]) *drift.Result
}

// ---------------------------------------------------------------------------
// Optional adapter interfaces — checked via type assertions in the reconciler.
// Adapters that do not implement these get sensible defaults (no-op / false).
// ---------------------------------------------------------------------------

// PreReconciler is an optional interface for adapters that need to run
// resource-specific setup before the main reconciliation state machine.
// Schema uses this to resolve databaseRef; Grant and RoleAssignment adapters
// use it to resolve role and target references.
// Default when absent: skip (no pre-reconcile step).
type PreReconciler[T ManagedResource] interface {
	PreReconcile(ctx context.Context, obj T) error
}

// WatchConfigurer is an optional interface for adapters that add
// resource-specific watches beyond the primary For() watch. Schema-scoped
// resources watch their parent Database; grant adapters watch referenced roles.
// Default when absent: no additional watches.
type WatchConfigurer interface {
	SetupWatches() SetupWatchesFunc
}

// PostCreateHook is an optional interface for adapters that need to run
// resource-specific logic after a successful create, before the status patch.
// Default when absent: no-op.
type PostCreateHook[T ManagedResource] interface {
	PostCreate(obj T)
}

// PostUpdateHook is an optional interface for adapters that need to run
// resource-specific logic after a successful update, before the final status
// patch. "altered" indicates whether an ALTER was issued. alterOpts are the
// options returned by BuildAlterOptions; adapters read per-reconciliation
// values from them (e.g. password hash, resource constraint) instead of
// storing mutable state on the adapter struct.
// Default when absent: no-op.
type PostUpdateHook[T ManagedResource] interface {
	PostUpdate(obj T, altered bool, alterOpts AlterOptions)
}

// CreateOrAlterSupporter is an optional interface for adapters whose resource
// type supports the CREATE OR ALTER SQL syntax. When implemented (returning
// true) and spec.managementPolicies.createOrAlter is true (the default), the
// reconciler uses a single CREATE OR ALTER statement for both create and update
// paths. Default when absent: CREATE OR ALTER is not supported.
type CreateOrAlterSupporter interface {
	SupportsCreateOrAlter() bool
}

// CascadeDropper is an optional interface for adapters whose Snowflake
// resource supports DROP … CASCADE. When implemented and the force-destroy
// annotation is set on the CR, the reconciler calls DropCascade instead of
// Drop. Only resources where Snowflake DDL supports CASCADE (Database,
// Schema) should implement this.
// Default when absent: force-destroy annotation is ignored and standard Drop is used.
type CascadeDropper[T ManagedResource, S any] interface {
	DropCascade(ctx context.Context, svc S, id Identifier) error
}

// LateInitializer is an optional interface for adapters that fill unset
// (nil) spec fields from observed Snowflake state during adoption.
// This follows the Crossplane late-initialization pattern: when a CR is
// adopted with adoptionPolicy=adopt, spec fields left nil by the user are
// populated from the live Snowflake resource, making the spec a complete
// representation of the managed state.
// Returns true if any spec field was modified (triggers spec persist).
// Default when absent: no late-initialization.
type LateInitializer[T ManagedResource, D any] interface {
	LateInitialize(obj T, obs *Observation[D]) bool
}

// SetupWatchesFunc is a callback used during SetupWithManager to add
// resource-specific watches (e.g., Schema watching Database changes).
type SetupWatchesFunc func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error

// ShouldSkipImmutableValidation returns true when immutable field checks
// should be bypassed — either because the resource has never been observed
// (ObservedGeneration == 0, meaning the first reconcile) or because the
// force-new annotation is active.
func ShouldSkipImmutableValidation[T ManagedResource](obj T) bool {
	return obj.GetObservedGeneration() == 0 || snowplanev1alpha1.IsForceNew(obj.GetAnnotations())
}

// WithUseRole optionally switches to the given useRole and returns a
// SQLExecutor suitable for constructing resource clients, plus a cleanup
// function. When useRole is empty the original client is returned as-is.
//
// Role switch failures (ErrRoleSwitchFailed) are wrapped as terminal errors
// because they require human intervention in Snowflake (GRANT ROLE ... TO USER ...).
// Connection errors are returned as-is so the controller retries with backoff.
func WithUseRole(ctx context.Context, sfClient clientfactory.SnowflakeClient, useRole string) (snowflake.SQLExecutor, func(context.Context), error) {
	if useRole == "" {
		// No role switch needed — the client itself satisfies SQLExecutor.
		return sfClient, nil, nil
	}

	scoped, cleanup, err := sfClient.WithRole(ctx, useRole)
	if err != nil {
		if snowflake.IsRoleSwitchFailed(err) {
			return nil, nil, snowflake.NewTerminalError(fmt.Errorf(
				"USE ROLE %q failed: %w — ensure the role is granted to the service user with: GRANT ROLE %s TO USER <service_user>",
				useRole, err, useRole,
			))
		}

		return nil, nil, fmt.Errorf("switching to use role %q: %w", useRole, err)
	}

	return scoped, cleanup, nil
}
