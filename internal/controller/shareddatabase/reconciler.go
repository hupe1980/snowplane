// Package shareddatabase implements the reconciler for SharedDatabase resources.
package shareddatabase

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/shareddatabase"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake shared databases.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SharedDatabaseObservation, error)
	Create(ctx context.Context, opts snowflake.CreateSharedDatabaseOptions) error
	Alter(ctx context.Context, opts snowflake.AlterSharedDatabaseOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new SharedDatabase reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.SharedDatabase, Service, *snowflake.SharedDatabaseObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewSharedDatabaseClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.SharedDatabase, Service, *snowflake.SharedDatabaseObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for SharedDatabase resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.SharedDatabase, Service, *snowflake.SharedDatabaseObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.SharedDatabase, Service, *snowflake.SharedDatabaseObservation]{
		ResourceNameVal:  "shareddatabase",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.SharedDatabase { return &snowplanev1alpha1.SharedDatabase{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.SharedDatabase) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.SharedDatabaseObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.SharedDatabaseObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.SharedDatabase, id snowflake.AccountObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterSharedDatabaseOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.SharedDatabase, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.SharedDatabaseObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.SharedDatabase, obs *reconciler.Observation[*snowflake.SharedDatabaseObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.SharedDatabase, obs *reconciler.Observation[*snowflake.SharedDatabaseObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

// --------------------------------------------------------------------------
// Pure helper functions (testable without service/client)
// --------------------------------------------------------------------------

// validateImmutableFields checks that immutable fields have not changed.
func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.SharedDatabase) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ShowOutput == nil {
		return nil
	}

	if !strings.EqualFold(obj.Spec.Name, obj.Status.ShowOutput.Name) {
		return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)",
			obj.Status.ShowOutput.Name, obj.Spec.Name)
	}

	// FromShare is validated via Origin from ShowOutput.
	if obj.Status.ShowOutput.Origin != "" {
		if !strings.EqualFold(obj.Spec.FromShare, obj.Status.ShowOutput.Origin) {
			return fmt.Errorf("spec.fromShare is immutable after creation (current: %q, desired: %q)",
				obj.Status.ShowOutput.Origin, obj.Spec.FromShare)
		}
	}

	return nil
}

func applyObservation(obj *snowplanev1alpha1.SharedDatabase, obs *snowflake.SharedDatabaseObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = obs.ShowOutput.Name
	}

	obj.Status.ShowOutput = obs.ShowOutput
}

func buildCreateOptions(obj *snowplanev1alpha1.SharedDatabase, id snowflake.AccountObjectIdentifier) snowflake.CreateSharedDatabaseOptions {
	return snowflake.CreateSharedDatabaseOptions{
		Name:      id,
		FromShare: obj.Spec.FromShare,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.SharedDatabase, id snowflake.AccountObjectIdentifier, obs *snowflake.SharedDatabaseObservation) snowflake.AlterSharedDatabaseOptions {
	opts := snowflake.AlterSharedDatabaseOptions{
		Name: id,
	}
	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	// Comment - only set if changed.
	if obj.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	// ExternalVolume - compare against observed parameters.
	if obj.Spec.ExternalVolume != nil {
		if obs == nil || obs.Parameters == nil || *obj.Spec.ExternalVolume != obs.Parameters.ExternalVolume {
			opts.ExternalVolume = obj.Spec.ExternalVolume
		}
	}

	// Catalog - compare against observed parameters.
	if obj.Spec.Catalog != nil {
		if obs == nil || obs.Parameters == nil || *obj.Spec.Catalog != obs.Parameters.Catalog {
			opts.Catalog = obj.Spec.Catalog
		}
	}

	// DefaultDDLCollation - compare against observed parameters.
	if obj.Spec.DefaultDDLCollation != nil {
		if obs == nil || obs.Parameters == nil || *obj.Spec.DefaultDDLCollation != obs.Parameters.DefaultDDLCollation {
			opts.DefaultDDLCollation = obj.Spec.DefaultDDLCollation
		}
	}

	// ReplaceInvalidCharacters - compare against observed parameters.
	if obj.Spec.ReplaceInvalidCharacters != nil {
		if obs == nil || obs.Parameters == nil || obs.Parameters.ReplaceInvalidCharacters == nil ||
			*obj.Spec.ReplaceInvalidCharacters != *obs.Parameters.ReplaceInvalidCharacters {
			opts.ReplaceInvalidCharacters = obj.Spec.ReplaceInvalidCharacters
		}
	}

	// StorageSerializationPolicy - compare against observed parameters.
	if obj.Spec.StorageSerializationPolicy != nil {
		v := string(*obj.Spec.StorageSerializationPolicy)
		if obs == nil || obs.Parameters == nil || v != obs.Parameters.StorageSerializationPolicy {
			opts.StorageSerializationPolicy = &v
		}
	}

	// LogLevel - compare against observed parameters.
	if obj.Spec.LogLevel != nil {
		v := string(*obj.Spec.LogLevel)
		if obs == nil || obs.Parameters == nil || v != obs.Parameters.LogLevel {
			opts.LogLevel = &v
		}
	}

	// TraceLevel - compare against observed parameters.
	if obj.Spec.TraceLevel != nil {
		v := string(*obj.Spec.TraceLevel)
		if obs == nil || obs.Parameters == nil || v != obs.Parameters.TraceLevel {
			opts.TraceLevel = &v
		}
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.SharedDatabase, obs *snowflake.SharedDatabaseObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	if obs.Parameters != nil {
		d.CompareString("EXTERNAL_VOLUME", obj.Spec.ExternalVolume, obs.Parameters.ExternalVolume, false)
		d.CompareString("CATALOG", obj.Spec.Catalog, obs.Parameters.Catalog, false)
		d.CompareString("DEFAULT_DDL_COLLATION", obj.Spec.DefaultDDLCollation, obs.Parameters.DefaultDDLCollation, false)
		d.CompareBool("REPLACE_INVALID_CHARACTERS", obj.Spec.ReplaceInvalidCharacters, obs.Parameters.ReplaceInvalidCharacters, false)

		if obj.Spec.StorageSerializationPolicy != nil {
			v := string(*obj.Spec.StorageSerializationPolicy)
			d.CompareStringValue("STORAGE_SERIALIZATION_POLICY", v, obs.Parameters.StorageSerializationPolicy, false)
		}

		if obj.Spec.LogLevel != nil {
			v := string(*obj.Spec.LogLevel)
			d.CompareStringValue("LOG_LEVEL", v, obs.Parameters.LogLevel, false)
		}

		if obj.Spec.TraceLevel != nil {
			v := string(*obj.Spec.TraceLevel)
			d.CompareStringValue("TRACE_LEVEL", v, obs.Parameters.TraceLevel, false)
		}
	}

	return d.Result()
}
