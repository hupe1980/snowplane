// Package databaserole implements the reconciler for DatabaseRole resources.
package databaserole

import (
	"context"

	"k8s.io/client-go/tools/record"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/databaserole"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake database roles.
type Service interface {
	Observe(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error)
	Create(ctx context.Context, opts snowflake.CreateDatabaseRoleOptions) error
	Alter(ctx context.Context, opts snowflake.AlterDatabaseRoleOptions) error
	Drop(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
// When useRole is non-empty the factory pins a connection, switches to that
// role, and returns a cleanup function that restores the original role.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new DatabaseRole reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRole, Service, *snowflake.DatabaseRoleObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRole, Service, *snowflake.DatabaseRoleObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory. This is intended for integration tests that
// inject mock Snowflake services while still going through SetupWithManager.
func NewReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRole, Service, *snowflake.DatabaseRoleObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRole, Service, *snowflake.DatabaseRoleObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// defaultServiceFactory is the production ServiceFactory used by NewReconciler.
func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
	if err != nil {
		return nil, nil, err
	}

	return snowflake.NewDatabaseRoleClient(sfC), cleanup, nil
}

func applyObservation(role *snowplanev1alpha1.DatabaseRole, obs *snowflake.DatabaseRoleObservation) {
	if obs.ShowOutput != nil {
		role.Status.FullyQualifiedName = snowflake.NewDatabaseObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		role.Status.DatabaseName = obs.ShowOutput.DatabaseName

		role.Status.ShowOutput = &snowplanev1alpha1.DatabaseRoleShowOutput{
			CreatedOn:      obs.ShowOutput.CreatedOn,
			Name:           obs.ShowOutput.Name,
			DatabaseName:   obs.ShowOutput.DatabaseName,
			Comment:        obs.ShowOutput.Comment,
			Owner:          obs.ShowOutput.Owner,
			GrantedToRoles: obs.ShowOutput.GrantedToRoles,
			GrantedRoles:   obs.ShowOutput.GrantedRoles,
		}
	}
}

func buildCreateOptions(role *snowplanev1alpha1.DatabaseRole, id snowflake.DatabaseObjectIdentifier) snowflake.CreateDatabaseRoleOptions {
	return snowflake.CreateDatabaseRoleOptions{
		Name:    id,
		Comment: role.Spec.Comment,
	}
}

func buildAlterOptions(role *snowplanev1alpha1.DatabaseRole, id snowflake.DatabaseObjectIdentifier, obs *snowflake.DatabaseRoleObservation) snowflake.AlterDatabaseRoleOptions {
	opts := snowflake.AlterDatabaseRoleOptions{Name: id}

	// Detect fields that were previously managed but are now nil -> UNSET.
	opts.UnsetFields = computeUnsetFields(role)

	if role.Spec.Comment != nil {
		if obs.ShowOutput == nil || *role.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = role.Spec.Comment
		}
	}

	return opts
}

// computeUnsetFields returns the Snowflake parameter names that were previously
// SET (tracked in status.TrackedParameters) but are now nil in the spec.
func computeUnsetFields(role *snowplanev1alpha1.DatabaseRole) []string {
	if len(role.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(role.Status.TrackedParameters))
	for _, f := range role.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if role.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	return unset
}

// computeTrackedParameters returns the Snowflake parameter names that are
// actively managed (non-nil) in the database role spec.
func computeTrackedParameters(spec *snowplanev1alpha1.DatabaseRoleSpec) []string {
	var fields []string

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	return fields
}

// detectDrift compares desired spec against the observed state and
// returns a structured drift result.
func detectDrift(role *snowplanev1alpha1.DatabaseRole, obs *snowflake.DatabaseRoleObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", role.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(role.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)

		// Mutable fields.
		d.CompareString("COMMENT", role.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	return d.Result()
}
