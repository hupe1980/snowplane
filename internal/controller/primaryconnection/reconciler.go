// Package primaryconnection implements the reconciler for PrimaryConnection resources.
package primaryconnection

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/helpers"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/primaryconnection"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.PrimaryConnectionObservation, error)
	Create(ctx context.Context, opts snowflake.CreatePrimaryConnectionOptions) error
	Alter(ctx context.Context, opts snowflake.AlterPrimaryConnectionOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new PrimaryConnection reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.PrimaryConnection, Service, *snowflake.PrimaryConnectionObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewPrimaryConnectionClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.PrimaryConnection, Service, *snowflake.PrimaryConnectionObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.PrimaryConnection, Service, *snowflake.PrimaryConnectionObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.PrimaryConnection, Service, *snowflake.PrimaryConnectionObservation]{
		ResourceNameVal:  "primaryconnection",
		FinalizerNameVal: finalizerName,
		NewObjectFn: func() *snowplanev1alpha1.PrimaryConnection {
			return &snowplanev1alpha1.PrimaryConnection{}
		},
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.PrimaryConnection) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.PrimaryConnectionObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.PrimaryConnectionObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.PrimaryConnection, id snowflake.AccountObjectIdentifier) error {
			return svc.Create(ctx, buildCreateOptions(obj, id))
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterPrimaryConnectionOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.PrimaryConnection, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.PrimaryConnectionObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.PrimaryConnection, obs *reconciler.Observation[*snowflake.PrimaryConnectionObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.PrimaryConnection, obs *reconciler.Observation[*snowflake.PrimaryConnectionObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.PrimaryConnection) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ShowOutput != nil {
		if obj.Status.ShowOutput.Name != "" && !strings.EqualFold(obj.Spec.Name, obj.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", obj.Status.ShowOutput.Name, obj.Spec.Name)
		}
	}

	return nil
}

func applyObservation(obj *snowplanev1alpha1.PrimaryConnection, obs *snowflake.PrimaryConnectionObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = obs.ShowOutput.Name
		obj.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.PrimaryConnection, id snowflake.AccountObjectIdentifier) snowflake.CreatePrimaryConnectionOptions {
	return snowflake.CreatePrimaryConnectionOptions{
		Name:    id,
		Comment: obj.Spec.Comment,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.PrimaryConnection, id snowflake.AccountObjectIdentifier, obs *snowflake.PrimaryConnectionObservation) snowflake.AlterPrimaryConnectionOptions {
	opts := snowflake.AlterPrimaryConnectionOptions{
		Name: id,
	}

	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	if obj.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	// Parse current failover accounts from ShowOutput for diff-checking.
	var currentAccounts []string
	if obs != nil && obs.ShowOutput != nil && obs.ShowOutput.FailoverAllowedTo != "" {
		currentAccounts = helpers.ParseCommaList(obs.ShowOutput.FailoverAllowedTo)
	}

	if len(obj.Spec.EnableFailoverToAccounts) > 0 {
		if !helpers.StringSlicesEqualFold(obj.Spec.EnableFailoverToAccounts, currentAccounts) {
			list := make([]string, len(obj.Spec.EnableFailoverToAccounts))
			copy(list, obj.Spec.EnableFailoverToAccounts)
			opts.EnableFailoverToAccounts = &list
		}
	} else if len(currentAccounts) > 0 {
		// Desired is empty but current has accounts — need to clear.
		empty := []string{}
		opts.EnableFailoverToAccounts = &empty
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.PrimaryConnection, obs *snowflake.PrimaryConnectionObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)

		// Parse SHOW CONNECTIONS failoverAllowedTo into a slice for comparison.
		var actualAccounts []string
		if obs.ShowOutput.FailoverAllowedTo != "" {
			actualAccounts = helpers.ParseCommaList(obs.ShowOutput.FailoverAllowedTo)
		}

		d.CompareStringSliceFold("ENABLE_FAILOVER_TO_ACCOUNTS", obj.Spec.EnableFailoverToAccounts, actualAccounts, false)
	}

	return d.Result()
}
