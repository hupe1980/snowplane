// Package failovergroup implements the reconciler for FailoverGroup resources.
package failovergroup

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

const finalizerName = "snowplane.hupe1980.github.io/failovergroup"

// SnowflakeClient is an alias for the client factory's SnowflakeClient.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service abstracts the Snowflake operations for a failover group.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.FailoverGroupObservation, error)
	Create(ctx context.Context, opts snowflake.CreateFailoverGroupOptions) error
	Alter(ctx context.Context, opts snowflake.AlterFailoverGroupOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler creates a new FailoverGroup reconciler using the default service factory.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.FailoverGroup, Service, *snowflake.FailoverGroupObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewFailoverGroupClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory creates a new FailoverGroup reconciler with a custom service factory.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.FailoverGroup, Service, *snowflake.FailoverGroupObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.FailoverGroup, Service, *snowflake.FailoverGroupObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.FailoverGroup, Service, *snowflake.FailoverGroupObservation]{
		ResourceNameVal:  "failovergroup",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.FailoverGroup { return &snowplanev1alpha1.FailoverGroup{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.FailoverGroup) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.FailoverGroupObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.FailoverGroupObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.FailoverGroup, id snowflake.AccountObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterFailoverGroupOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.FailoverGroup, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.FailoverGroupObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.FailoverGroup, obs *reconciler.Observation[*snowflake.FailoverGroupObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.FailoverGroup, obs *reconciler.Observation[*snowflake.FailoverGroupObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

// validateImmutableFields checks that immutable fields have not changed.
func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.FailoverGroup) error {
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

func applyObservation(obj *snowplanev1alpha1.FailoverGroup, obs *snowflake.FailoverGroupObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = obs.ShowOutput.Name
		obj.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.FailoverGroup, id snowflake.AccountObjectIdentifier) snowflake.CreateFailoverGroupOptions {
	opts := snowflake.CreateFailoverGroupOptions{
		Name:            id,
		ObjectTypes:     obj.Spec.ObjectTypes,
		AllowedAccounts: obj.Spec.AllowedAccounts,
	}

	if len(obj.Spec.AllowedDatabases) > 0 {
		opts.AllowedDatabases = append([]string{}, obj.Spec.AllowedDatabases...)
	}

	if len(obj.Spec.AllowedShares) > 0 {
		opts.AllowedShares = append([]string{}, obj.Spec.AllowedShares...)
	}

	if len(obj.Spec.AllowedIntegrationTypes) > 0 {
		opts.AllowedIntegrationTypes = append([]string{}, obj.Spec.AllowedIntegrationTypes...)
	}

	opts.IgnoreEditionCheck = obj.Spec.IgnoreEditionCheck
	opts.ReplicationSchedule = obj.Spec.ReplicationSchedule
	opts.ErrorIntegration = obj.Spec.ErrorIntegration
	opts.Comment = obj.Spec.Comment

	return opts
}

func buildAlterOptions(obj *snowplanev1alpha1.FailoverGroup, id snowflake.AccountObjectIdentifier, obs *snowflake.FailoverGroupObservation) snowflake.AlterFailoverGroupOptions {
	opts := snowflake.AlterFailoverGroupOptions{
		Name: id,
	}

	// Compute which fields to UNSET.
	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	// Always send object types (required, no UNSET possible).
	ot := append([]string{}, obj.Spec.ObjectTypes...)
	opts.ObjectTypes = &ot

	// Always send allowed accounts (required, no UNSET possible).
	aa := append([]string{}, obj.Spec.AllowedAccounts...)
	opts.AllowedAccounts = &aa

	// List fields: always send when non-empty.
	if len(obj.Spec.AllowedDatabases) > 0 {
		dbs := append([]string{}, obj.Spec.AllowedDatabases...)
		opts.AllowedDatabases = &dbs
	}

	if len(obj.Spec.AllowedShares) > 0 {
		shares := append([]string{}, obj.Spec.AllowedShares...)
		opts.AllowedShares = &shares
	}

	if len(obj.Spec.AllowedIntegrationTypes) > 0 {
		its := append([]string{}, obj.Spec.AllowedIntegrationTypes...)
		opts.AllowedIntegrationTypes = &its
	}

	// Comment: only SET if spec has value and differs from observed.
	if obj.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	// Replication schedule.
	if obj.Spec.ReplicationSchedule != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.ReplicationSchedule != obs.ShowOutput.ReplicationSchedule {
			opts.ReplicationSchedule = obj.Spec.ReplicationSchedule
		}
	}

	// Error integration.
	if obj.Spec.ErrorIntegration != nil {
		opts.ErrorIntegration = obj.Spec.ErrorIntegration
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.FailoverGroup, obs *snowflake.FailoverGroupObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)

		// Compare object types.
		obsObjTypes := helpers.ParseCommaList(obs.ShowOutput.ObjectTypes)
		d.CompareStringSliceFold("OBJECT_TYPES", obj.Spec.ObjectTypes, obsObjTypes, false)

		// Compare allowed accounts.
		obsAccounts := helpers.ParseCommaList(obs.ShowOutput.AllowedAccounts)
		d.CompareStringSliceFold("ALLOWED_ACCOUNTS", obj.Spec.AllowedAccounts, obsAccounts, false)

		// Compare allowed integration types.
		obsIntTypes := helpers.ParseCommaList(obs.ShowOutput.AllowedIntegrationTypes)
		d.CompareStringSliceFold("ALLOWED_INTEGRATION_TYPES", obj.Spec.AllowedIntegrationTypes, obsIntTypes, false)

		// Compare replication schedule.
		d.CompareString("REPLICATION_SCHEDULE", obj.Spec.ReplicationSchedule, obs.ShowOutput.ReplicationSchedule, false)

		// Note: AllowedDatabases, AllowedShares, and ErrorIntegration are not
		// returned by SHOW FAILOVER GROUPS, so drift cannot be detected for
		// these fields. They are always sent unconditionally via ALTER.
	}

	return d.Result()
}
