// Package secondarydatabase implements the reconciler for SecondaryDatabase resources.
package secondarydatabase

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
	finalizerName = "snowplane.hupe1980.github.io/secondarydatabase"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake secondary databases.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecondaryDatabaseObservation, error)
	Create(ctx context.Context, opts snowflake.CreateSecondaryDatabaseOptions) error
	Alter(ctx context.Context, opts snowflake.AlterSecondaryDatabaseOptions) error
	Refresh(ctx context.Context, name snowflake.AccountObjectIdentifier) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// --------------------------------------------------------------------------
// secondaryDatabaseAlterOpts wraps the real alter options and always reports
// HasChanges() == true so that the generic reconciler always calls AlterFn.
// This is necessary because secondary databases must be refreshed on every
// reconcile, regardless of whether any ALTER properties changed.
// --------------------------------------------------------------------------

type secondaryDatabaseAlterOpts struct {
	inner snowflake.AlterSecondaryDatabaseOptions
}

func (o *secondaryDatabaseAlterOpts) HasChanges() bool { return true }

// NewReconciler returns a new SecondaryDatabase reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.SecondaryDatabase, Service, *snowflake.SecondaryDatabaseObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewSecondaryDatabaseClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.SecondaryDatabase, Service, *snowflake.SecondaryDatabaseObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for SecondaryDatabase resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.SecondaryDatabase, Service, *snowflake.SecondaryDatabaseObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.SecondaryDatabase, Service, *snowflake.SecondaryDatabaseObservation]{
		ResourceNameVal:  "secondarydatabase",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.SecondaryDatabase { return &snowplanev1alpha1.SecondaryDatabase{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.SecondaryDatabase) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.SecondaryDatabaseObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.SecondaryDatabaseObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.SecondaryDatabase, id snowflake.AccountObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			if err := svc.Create(ctx, opts); err != nil {
				return err
			}

			// Refresh immediately after creation to sync with the primary.
			return svc.Refresh(ctx, id)
		}),
		// AlterFn: we write this directly (not MakeAlter) because we need to
		// handle both ALTER and REFRESH in the same call. The wrapper type
		// secondaryDatabaseAlterOpts ensures the reconciler always calls us.
		AlterFn: func(ctx context.Context, svc Service, rawOpts reconciler.AlterOptions) error {
			wrapper, ok := rawOpts.(*secondaryDatabaseAlterOpts)
			if !ok {
				return fmt.Errorf("unexpected alter options type: %T", rawOpts)
			}

			// Only issue ALTER if there are real property changes.
			if wrapper.inner.HasChanges() {
				if err := svc.Alter(ctx, wrapper.inner); err != nil {
					return err
				}
			}

			// Always refresh to keep the replica in sync with the primary.
			return svc.Refresh(ctx, wrapper.inner.Name)
		},
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.SecondaryDatabase, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.SecondaryDatabaseObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &secondaryDatabaseAlterOpts{inner: opts}, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.SecondaryDatabase, obs *reconciler.Observation[*snowflake.SecondaryDatabaseObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.SecondaryDatabase, obs *reconciler.Observation[*snowflake.SecondaryDatabaseObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

// --------------------------------------------------------------------------
// Pure helper functions (testable without service/client)
// --------------------------------------------------------------------------

// validateImmutableFields checks that immutable fields have not changed.
func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.SecondaryDatabase) error {
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

	// AsReplicaOf is validated via Origin from ShowOutput.
	if obj.Status.ShowOutput.Origin != "" {
		if !strings.EqualFold(obj.Spec.AsReplicaOf, obj.Status.ShowOutput.Origin) {
			return fmt.Errorf("spec.asReplicaOf is immutable after creation (current: %q, desired: %q)",
				obj.Status.ShowOutput.Origin, obj.Spec.AsReplicaOf)
		}
	}

	return nil
}

func applyObservation(obj *snowplanev1alpha1.SecondaryDatabase, obs *snowflake.SecondaryDatabaseObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = obs.ShowOutput.Name
	}

	obj.Status.ShowOutput = obs.ShowOutput
}

func buildCreateOptions(obj *snowplanev1alpha1.SecondaryDatabase, id snowflake.AccountObjectIdentifier) snowflake.CreateSecondaryDatabaseOptions {
	return snowflake.CreateSecondaryDatabaseOptions{
		Name:                    id,
		AsReplicaOf:             obj.Spec.AsReplicaOf,
		DataRetentionTimeInDays: obj.Spec.DataRetentionTimeInDays,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.SecondaryDatabase, id snowflake.AccountObjectIdentifier, obs *snowflake.SecondaryDatabaseObservation) snowflake.AlterSecondaryDatabaseOptions {
	opts := snowflake.AlterSecondaryDatabaseOptions{
		Name: id,
	}
	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	// Comment - only set if changed.
	if obj.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	// DataRetentionTimeInDays - compare against observed parameters.
	if obj.Spec.DataRetentionTimeInDays != nil {
		if obs == nil || obs.Parameters == nil || obs.Parameters.DataRetentionTimeInDays == nil ||
			*obj.Spec.DataRetentionTimeInDays != *obs.Parameters.DataRetentionTimeInDays {
			opts.DataRetentionTimeInDays = obj.Spec.DataRetentionTimeInDays
		}
	}

	// MaxDataExtensionTimeInDays - compare against observed parameters.
	if obj.Spec.MaxDataExtensionTimeInDays != nil {
		if obs == nil || obs.Parameters == nil || obs.Parameters.MaxDataExtensionTimeInDays == nil ||
			*obj.Spec.MaxDataExtensionTimeInDays != *obs.Parameters.MaxDataExtensionTimeInDays {
			opts.MaxDataExtensionTimeInDays = obj.Spec.MaxDataExtensionTimeInDays
		}
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.SecondaryDatabase, obs *snowflake.SecondaryDatabaseObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	if obs.Parameters != nil {
		d.CompareInt32("DATA_RETENTION_TIME_IN_DAYS", obj.Spec.DataRetentionTimeInDays, obs.Parameters.DataRetentionTimeInDays, false)
		d.CompareInt32("MAX_DATA_EXTENSION_TIME_IN_DAYS", obj.Spec.MaxDataExtensionTimeInDays, obs.Parameters.MaxDataExtensionTimeInDays, false)
	}

	return d.Result()
}
