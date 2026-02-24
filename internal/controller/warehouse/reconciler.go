// Package warehouse implements the reconciler for Warehouse resources.
package warehouse

import (
	"context"
	"strings"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
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
	a := &adapter{newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Warehouse, Service, *snowflake.WarehouseObservation]{
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
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.Warehouse, Service, *snowflake.WarehouseObservation] {
	a := &adapter{newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Warehouse, Service, *snowflake.WarehouseObservation]{
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

	return snowflake.NewWarehouseClient(sfC), cleanup, nil
}

func applyObservation(wh *snowplanev1alpha1.Warehouse, obs *snowflake.WarehouseObservation) {
	if obs.ShowOutput != nil {
		wh.Status.FullyQualifiedName = snowflake.NewAccountObjectIdentifier(obs.ShowOutput.Name).FullyQualifiedName()
		wh.Status.State = obs.ShowOutput.State

		wh.Status.ShowOutput = &snowplanev1alpha1.WarehouseShowOutput{
			CreatedOn:       obs.ShowOutput.CreatedOn,
			Name:            obs.ShowOutput.Name,
			State:           obs.ShowOutput.State,
			Type:            obs.ShowOutput.Type,
			Size:            obs.ShowOutput.Size,
			Comment:         obs.ShowOutput.Comment,
			Owner:           obs.ShowOutput.Owner,
			AutoSuspend:     obs.ShowOutput.AutoSuspend,
			AutoResume:      obs.ShowOutput.AutoResume,
			MinClusterCount: obs.ShowOutput.MinClusterCount,
			MaxClusterCount: obs.ShowOutput.MaxClusterCount,
			ScalingPolicy:   obs.ShowOutput.ScalingPolicy,
			ResourceMonitor: obs.ShowOutput.ResourceMonitor,
		}
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
	opts.UnsetFields = computeUnsetFields(wh)

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
		if obs.ShowOutput == nil || !strings.EqualFold(s, obs.ShowOutput.Size) {
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
			d.CompareStringValueFold("WAREHOUSE_SIZE", s, obs.ShowOutput.Size, false)
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

// computeUnsetFields returns the Snowflake parameter names that were previously
// SET (tracked in status.TrackedParameters) but are now nil in the spec.
func computeUnsetFields(wh *snowplanev1alpha1.Warehouse) []string {
	if len(wh.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(wh.Status.TrackedParameters))
	for _, f := range wh.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if wh.Spec.WarehouseType == nil && managed["WAREHOUSE_TYPE"] {
		unset = append(unset, "WAREHOUSE_TYPE")
	}

	if wh.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	if wh.Spec.WarehouseSize == nil && managed["WAREHOUSE_SIZE"] {
		unset = append(unset, "WAREHOUSE_SIZE")
	}

	if wh.Spec.MinClusterCount == nil && managed["MIN_CLUSTER_COUNT"] {
		unset = append(unset, "MIN_CLUSTER_COUNT")
	}

	if wh.Spec.MaxClusterCount == nil && managed["MAX_CLUSTER_COUNT"] {
		unset = append(unset, "MAX_CLUSTER_COUNT")
	}

	if wh.Spec.ScalingPolicy == nil && managed["SCALING_POLICY"] {
		unset = append(unset, "SCALING_POLICY")
	}

	if wh.Spec.AutoSuspend == nil && managed["AUTO_SUSPEND"] {
		unset = append(unset, "AUTO_SUSPEND")
	}

	if wh.Spec.AutoResume == nil && managed["AUTO_RESUME"] {
		unset = append(unset, "AUTO_RESUME")
	}

	if wh.Spec.ResourceMonitor == nil && managed["RESOURCE_MONITOR"] {
		unset = append(unset, "RESOURCE_MONITOR")
	}

	if wh.Spec.EnableQueryAcceleration == nil && managed["ENABLE_QUERY_ACCELERATION"] {
		unset = append(unset, "ENABLE_QUERY_ACCELERATION")
	}

	if wh.Spec.QueryAccelerationMaxScaleFactor == nil && managed["QUERY_ACCELERATION_MAX_SCALE_FACTOR"] {
		unset = append(unset, "QUERY_ACCELERATION_MAX_SCALE_FACTOR")
	}

	if wh.Spec.MaxConcurrencyLevel == nil && managed["MAX_CONCURRENCY_LEVEL"] {
		unset = append(unset, "MAX_CONCURRENCY_LEVEL")
	}

	if wh.Spec.StatementQueuedTimeoutInSeconds == nil && managed["STATEMENT_QUEUED_TIMEOUT_IN_SECONDS"] {
		unset = append(unset, "STATEMENT_QUEUED_TIMEOUT_IN_SECONDS")
	}

	if wh.Spec.StatementTimeoutInSeconds == nil && managed["STATEMENT_TIMEOUT_IN_SECONDS"] {
		unset = append(unset, "STATEMENT_TIMEOUT_IN_SECONDS")
	}

	if wh.Spec.ResourceConstraint == nil && managed["RESOURCE_CONSTRAINT"] {
		unset = append(unset, "RESOURCE_CONSTRAINT")
	}

	if wh.Spec.Generation == nil && managed["GENERATION"] {
		unset = append(unset, "GENERATION")
	}

	return unset
}

// computeTrackedParameters returns the Snowflake parameter names that are
// actively managed (non-nil) in the warehouse spec.
func computeTrackedParameters(spec *snowplanev1alpha1.WarehouseSpec) []string {
	var fields []string

	if spec.WarehouseType != nil {
		fields = append(fields, "WAREHOUSE_TYPE")
	}

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	if spec.WarehouseSize != nil {
		fields = append(fields, "WAREHOUSE_SIZE")
	}

	if spec.MinClusterCount != nil {
		fields = append(fields, "MIN_CLUSTER_COUNT")
	}

	if spec.MaxClusterCount != nil {
		fields = append(fields, "MAX_CLUSTER_COUNT")
	}

	if spec.ScalingPolicy != nil {
		fields = append(fields, "SCALING_POLICY")
	}

	if spec.AutoSuspend != nil {
		fields = append(fields, "AUTO_SUSPEND")
	}

	if spec.AutoResume != nil {
		fields = append(fields, "AUTO_RESUME")
	}

	if spec.ResourceMonitor != nil {
		fields = append(fields, "RESOURCE_MONITOR")
	}

	if spec.EnableQueryAcceleration != nil {
		fields = append(fields, "ENABLE_QUERY_ACCELERATION")
	}

	if spec.QueryAccelerationMaxScaleFactor != nil {
		fields = append(fields, "QUERY_ACCELERATION_MAX_SCALE_FACTOR")
	}

	if spec.MaxConcurrencyLevel != nil {
		fields = append(fields, "MAX_CONCURRENCY_LEVEL")
	}

	if spec.StatementQueuedTimeoutInSeconds != nil {
		fields = append(fields, "STATEMENT_QUEUED_TIMEOUT_IN_SECONDS")
	}

	if spec.StatementTimeoutInSeconds != nil {
		fields = append(fields, "STATEMENT_TIMEOUT_IN_SECONDS")
	}

	if spec.ResourceConstraint != nil {
		fields = append(fields, "RESOURCE_CONSTRAINT")
	}

	if spec.Generation != nil {
		fields = append(fields, "GENERATION")
	}

	return fields
}
