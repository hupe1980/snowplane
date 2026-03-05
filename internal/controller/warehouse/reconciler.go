// Package warehouse implements the reconciler for Warehouse resources.
package warehouse

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
	finalizerName = "snowplane.hupe1980.github.io/warehouse"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake warehouses.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error)
	Create(ctx context.Context, opts snowflake.CreateWarehouseOptions) error
	Alter(ctx context.Context, opts snowflake.AlterWarehouseOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
// When useRole is non-empty the factory pins a connection, switches to that
// role, and returns a cleanup function that restores the original role.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Warehouse reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Warehouse, Service, *snowflake.WarehouseObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewWarehouseClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory. This is intended for integration tests that
// inject mock Snowflake services while still going through SetupWithManager.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.Warehouse, Service, *snowflake.WarehouseObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for Warehouse resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.Warehouse, Service, *snowflake.WarehouseObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.Warehouse, Service, *snowflake.WarehouseObservation]{
		ResourceNameVal:  "warehouse",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.Warehouse { return &snowplanev1alpha1.Warehouse{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.Warehouse) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.WarehouseObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.Warehouse, id snowflake.AccountObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()

			if err := svc.Create(ctx, opts); err != nil {
				return err
			}

			// Track resource constraint and generation for change detection on future reconciles.
			if obj.Spec.ResourceConstraint != nil {
				obj.Status.LastAppliedResourceConstraint = string(*obj.Spec.ResourceConstraint)
			}

			if obj.Spec.Generation != nil {
				obj.Status.LastAppliedGeneration = *obj.Spec.Generation
			}

			return nil
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterWarehouseOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.Warehouse, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.WarehouseObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.Warehouse, obs *reconciler.Observation[*snowflake.WarehouseObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.Warehouse, obs *reconciler.Observation[*snowflake.WarehouseObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		SupportsCoA:      true,
		LateInitializeFn: lateInitialize,
		PostUpdateFn: func(wh *snowplanev1alpha1.Warehouse, altered bool, alterOpts reconciler.AlterOptions) {
			// Commit resource constraint and generation only after a successful ALTER,
			// reading from the per-reconciliation AlterOptions (no shared mutable state).
			if altered {
				if opts, ok := alterOpts.(*snowflake.AlterWarehouseOptions); ok {
					if opts.ResourceConstraint != nil {
						wh.Status.LastAppliedResourceConstraint = *opts.ResourceConstraint
					}

					if opts.Generation != nil {
						wh.Status.LastAppliedGeneration = *opts.Generation
					}
				}
			}

			// Clear tracked values when fields are removed from spec.
			if wh.Spec.ResourceConstraint == nil {
				wh.Status.LastAppliedResourceConstraint = ""
			}

			if wh.Spec.Generation == nil {
				wh.Status.LastAppliedGeneration = ""
			}
		},
	}
}

func validateImmutableFields(_ context.Context, wh *snowplanev1alpha1.Warehouse) error {
	if reconciler.ShouldSkipImmutableValidation(wh) {
		return nil
	}

	if wh.Status.ShowOutput != nil {
		if wh.Status.ShowOutput.Name != "" && !strings.EqualFold(wh.Spec.Name, wh.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", wh.Status.ShowOutput.Name, wh.Spec.Name)
		}

	}

	return nil
}

func applyObservation(wh *snowplanev1alpha1.Warehouse, obs *snowflake.WarehouseObservation) {
	if obs.ShowOutput != nil {
		wh.Status.FullyQualifiedName = snowflake.NewAccountObjectIdentifier(obs.ShowOutput.Name).FullyQualifiedName()
		wh.Status.State = obs.ShowOutput.State

		wh.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(wh *snowplanev1alpha1.Warehouse, id snowflake.AccountObjectIdentifier) snowflake.CreateWarehouseOptions {
	opts := snowflake.CreateWarehouseOptions{
		Name:                            id,
		InitiallySuspended:              wh.Spec.InitiallySuspended,
		Comment:                         wh.Spec.Comment,
		AutoResume:                      wh.Spec.AutoResume,
		AutoSuspend:                     wh.Spec.AutoSuspend,
		MinClusterCount:                 wh.Spec.MinClusterCount,
		MaxClusterCount:                 wh.Spec.MaxClusterCount,
		ResourceMonitor:                 wh.Spec.ResourceMonitor,
		EnableQueryAcceleration:         wh.Spec.EnableQueryAcceleration,
		QueryAccelerationMaxScaleFactor: wh.Spec.QueryAccelerationMaxScaleFactor,
		MaxConcurrencyLevel:             wh.Spec.MaxConcurrencyLevel,
		StatementQueuedTimeoutInSeconds: wh.Spec.StatementQueuedTimeoutInSeconds,
		StatementTimeoutInSeconds:       wh.Spec.StatementTimeoutInSeconds,
	}

	if wh.Spec.WarehouseType != nil {
		s := string(*wh.Spec.WarehouseType)
		opts.WarehouseType = &s
	}

	if wh.Spec.WarehouseSize != nil {
		s := string(*wh.Spec.WarehouseSize)
		opts.WarehouseSize = &s
	}

	if wh.Spec.ScalingPolicy != nil {
		s := string(*wh.Spec.ScalingPolicy)
		opts.ScalingPolicy = &s
	}

	if wh.Spec.ResourceConstraint != nil {
		s := string(*wh.Spec.ResourceConstraint)
		opts.ResourceConstraint = &s
	}

	if wh.Spec.Generation != nil {
		opts.Generation = wh.Spec.Generation
	}

	return opts
}

func buildAlterOptions(wh *snowplanev1alpha1.Warehouse, id snowflake.AccountObjectIdentifier, obs *snowflake.WarehouseObservation) snowflake.AlterWarehouseOptions {
	opts := snowflake.AlterWarehouseOptions{Name: id}

	// Detect fields that were previously managed but are now nil -> UNSET.
	opts.UnsetFields = tracked.ComputeUnset(&wh.Spec, wh.Status.TrackedParameters)

	if wh.Spec.WarehouseType != nil {
		s := string(*wh.Spec.WarehouseType)
		if obs.ShowOutput == nil || !strings.EqualFold(s, obs.ShowOutput.Type) {
			opts.WarehouseType = &s
		}
	}

	if wh.Spec.Comment != nil {
		if obs.ShowOutput == nil || *wh.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = wh.Spec.Comment
		}
	}

	if wh.Spec.WarehouseSize != nil {
		s := string(*wh.Spec.WarehouseSize)
		if obs.ShowOutput == nil || !strings.EqualFold(s, string(normalizeWarehouseSize(obs.ShowOutput.Size))) {
			opts.WarehouseSize = &s
		}
	}

	if wh.Spec.AutoSuspend != nil {
		if obs.ShowOutput == nil || *wh.Spec.AutoSuspend != obs.ShowOutput.AutoSuspend {
			opts.AutoSuspend = wh.Spec.AutoSuspend
		}
	}

	if wh.Spec.AutoResume != nil {
		if obs.ShowOutput == nil || *wh.Spec.AutoResume != obs.ShowOutput.AutoResume {
			opts.AutoResume = wh.Spec.AutoResume
		}
	}

	if wh.Spec.MinClusterCount != nil {
		if obs.ShowOutput == nil || *wh.Spec.MinClusterCount != obs.ShowOutput.MinClusterCount {
			opts.MinClusterCount = wh.Spec.MinClusterCount
		}
	}

	if wh.Spec.MaxClusterCount != nil {
		if obs.ShowOutput == nil || *wh.Spec.MaxClusterCount != obs.ShowOutput.MaxClusterCount {
			opts.MaxClusterCount = wh.Spec.MaxClusterCount
		}
	}

	if wh.Spec.ScalingPolicy != nil {
		s := string(*wh.Spec.ScalingPolicy)
		obsScalingPolicy := ""
		if obs.ShowOutput != nil {
			obsScalingPolicy = obs.ShowOutput.ScalingPolicy
		}

		if !strings.EqualFold(s, obsScalingPolicy) {
			opts.ScalingPolicy = &s
		}
	}

	if wh.Spec.ResourceMonitor != nil {
		obsResourceMonitor := ""
		if obs.ShowOutput != nil {
			obsResourceMonitor = obs.ShowOutput.ResourceMonitor
		}

		if !strings.EqualFold(*wh.Spec.ResourceMonitor, obsResourceMonitor) {
			opts.ResourceMonitor = wh.Spec.ResourceMonitor
		}
	}

	// ResourceConstraint is not surfaced in SHOW WAREHOUSES; compare against
	// the last value we applied (stored in status) to avoid unnecessary ALTERs.
	if wh.Spec.ResourceConstraint != nil {
		s := string(*wh.Spec.ResourceConstraint)
		if s != wh.Status.LastAppliedResourceConstraint {
			opts.ResourceConstraint = &s
		}
	}

	// Generation is not surfaced in SHOW WAREHOUSES; compare against
	// the last value we applied (stored in status) to avoid unnecessary ALTERs.
	if wh.Spec.Generation != nil {
		if *wh.Spec.Generation != wh.Status.LastAppliedGeneration {
			opts.Generation = wh.Spec.Generation
		}
	}

	if obs.Parameters == nil {
		return opts
	}

	p := obs.Parameters

	if wh.Spec.EnableQueryAcceleration != nil {
		if p.EnableQueryAcceleration == nil || *wh.Spec.EnableQueryAcceleration != *p.EnableQueryAcceleration {
			opts.EnableQueryAcceleration = wh.Spec.EnableQueryAcceleration
		}
	}

	if wh.Spec.QueryAccelerationMaxScaleFactor != nil {
		if p.QueryAccelerationMaxScaleFactor == nil || *wh.Spec.QueryAccelerationMaxScaleFactor != *p.QueryAccelerationMaxScaleFactor {
			opts.QueryAccelerationMaxScaleFactor = wh.Spec.QueryAccelerationMaxScaleFactor
		}
	}

	if wh.Spec.MaxConcurrencyLevel != nil {
		if p.MaxConcurrencyLevel == nil || *wh.Spec.MaxConcurrencyLevel != *p.MaxConcurrencyLevel {
			opts.MaxConcurrencyLevel = wh.Spec.MaxConcurrencyLevel
		}
	}

	if wh.Spec.StatementQueuedTimeoutInSeconds != nil {
		if p.StatementQueuedTimeoutInSeconds == nil || *wh.Spec.StatementQueuedTimeoutInSeconds != *p.StatementQueuedTimeoutInSeconds {
			opts.StatementQueuedTimeoutInSeconds = wh.Spec.StatementQueuedTimeoutInSeconds
		}
	}

	if wh.Spec.StatementTimeoutInSeconds != nil {
		if p.StatementTimeoutInSeconds == nil || *wh.Spec.StatementTimeoutInSeconds != *p.StatementTimeoutInSeconds {
			opts.StatementTimeoutInSeconds = wh.Spec.StatementTimeoutInSeconds
		}
	}

	return opts
}

// detectDrift uses the drift.Detector to compute field-level differences between
// the desired spec and observed state. This is used for condition/event reporting.
func detectDrift(wh *snowplanev1alpha1.Warehouse, obs *snowflake.WarehouseObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", wh.Spec.Name, obs.ShowOutput.Name, true)

		// Mutable fields.
		if wh.Spec.WarehouseType != nil {
			s := string(*wh.Spec.WarehouseType)
			d.CompareStringValueFold("WAREHOUSE_TYPE", s, obs.ShowOutput.Type, false)
		}

		d.CompareString("COMMENT", wh.Spec.Comment, obs.ShowOutput.Comment, false)

		if wh.Spec.WarehouseSize != nil {
			s := string(*wh.Spec.WarehouseSize)
			d.CompareStringValueFold("WAREHOUSE_SIZE", s, string(normalizeWarehouseSize(obs.ShowOutput.Size)), false)
		}

		if wh.Spec.AutoSuspend != nil {
			obsVal := obs.ShowOutput.AutoSuspend
			d.CompareInt32("AUTO_SUSPEND", wh.Spec.AutoSuspend, &obsVal, false)
		}

		if wh.Spec.AutoResume != nil {
			obsVal := obs.ShowOutput.AutoResume
			d.CompareBool("AUTO_RESUME", wh.Spec.AutoResume, &obsVal, false)
		}

		if wh.Spec.MinClusterCount != nil {
			obsVal := obs.ShowOutput.MinClusterCount
			d.CompareInt32("MIN_CLUSTER_COUNT", wh.Spec.MinClusterCount, &obsVal, false)
		}

		if wh.Spec.MaxClusterCount != nil {
			obsVal := obs.ShowOutput.MaxClusterCount
			d.CompareInt32("MAX_CLUSTER_COUNT", wh.Spec.MaxClusterCount, &obsVal, false)
		}

		if wh.Spec.ScalingPolicy != nil {
			s := string(*wh.Spec.ScalingPolicy)
			d.CompareStringValueFold("SCALING_POLICY", s, obs.ShowOutput.ScalingPolicy, false)
		}

		if wh.Spec.ResourceMonitor != nil {
			d.CompareStringValueFold("RESOURCE_MONITOR", *wh.Spec.ResourceMonitor, obs.ShowOutput.ResourceMonitor, false)
		}
	}

	if obs.Parameters != nil {
		p := obs.Parameters
		d.CompareBool("ENABLE_QUERY_ACCELERATION", wh.Spec.EnableQueryAcceleration, p.EnableQueryAcceleration, false)
		d.CompareInt32("QUERY_ACCELERATION_MAX_SCALE_FACTOR", wh.Spec.QueryAccelerationMaxScaleFactor, p.QueryAccelerationMaxScaleFactor, false)
		d.CompareInt32("MAX_CONCURRENCY_LEVEL", wh.Spec.MaxConcurrencyLevel, p.MaxConcurrencyLevel, false)
		d.CompareInt32("STATEMENT_QUEUED_TIMEOUT_IN_SECONDS", wh.Spec.StatementQueuedTimeoutInSeconds, p.StatementQueuedTimeoutInSeconds, false)
		d.CompareInt32("STATEMENT_TIMEOUT_IN_SECONDS", wh.Spec.StatementTimeoutInSeconds, p.StatementTimeoutInSeconds, false)
	}

	return d.Result()
}
