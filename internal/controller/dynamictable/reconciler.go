// Package dynamictable implements the reconciler for DynamicTable resources.
package dynamictable

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
	finalizerName = "snowplane.hupe1980.github.io/dynamictable"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake dynamic tables.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error)
	Create(ctx context.Context, opts snowflake.CreateDynamicTableOptions) error
	Alter(ctx context.Context, opts snowflake.AlterDynamicTableOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new DynamicTable reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.DynamicTable, Service, *snowflake.DynamicTableObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.DynamicTable, Service, *snowflake.DynamicTableObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.DynamicTable, Service, *snowflake.DynamicTableObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.DynamicTable, Service, *snowflake.DynamicTableObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// defaultServiceFactory is the production ServiceFactory.
func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
	if err != nil {
		return nil, nil, err
	}

	return snowflake.NewDynamicTableClient(sfC), cleanup, nil
}

func applyObservation(dt *snowplanev1alpha1.DynamicTable, obs *snowflake.DynamicTableObservation) {
	if obs.ShowOutput != nil {
		dt.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		dt.Status.DatabaseName = obs.ShowOutput.DatabaseName
		dt.Status.SchemaName = obs.ShowOutput.SchemaName

		dt.Status.ShowOutput = &snowplanev1alpha1.DynamicTableShowOutput{
			CreatedOn:       obs.ShowOutput.CreatedOn,
			Name:            obs.ShowOutput.Name,
			DatabaseName:    obs.ShowOutput.DatabaseName,
			SchemaName:      obs.ShowOutput.SchemaName,
			Owner:           obs.ShowOutput.Owner,
			Comment:         obs.ShowOutput.Comment,
			TargetLag:       obs.ShowOutput.TargetLag,
			Warehouse:       obs.ShowOutput.Warehouse,
			RefreshMode:     obs.ShowOutput.RefreshMode,
			Text:            obs.ShowOutput.Text,
			SchedulingState: obs.ShowOutput.SchedulingState,
			ClusterBy:       obs.ShowOutput.ClusterBy,
			DataTimestamp:   obs.ShowOutput.DataTimestamp,
		}
	}
}

func buildCreateOptions(dt *snowplanev1alpha1.DynamicTable, id snowflake.SchemaObjectIdentifier) snowflake.CreateDynamicTableOptions {
	opts := snowflake.CreateDynamicTableOptions{
		Name:                       id,
		Query:                      dt.Spec.Query,
		TargetLag:                  dt.Spec.TargetLag,
		Warehouse:                  dt.Spec.Warehouse,
		Comment:                    dt.Spec.Comment,
		Transient:                  dt.Spec.Transient,
		ClusterBy:                  dt.Spec.ClusterBy,
		DataRetentionTimeInDays:    dt.Spec.DataRetentionTimeInDays,
		MaxDataExtensionTimeInDays: dt.Spec.MaxDataExtensionTimeInDays,
	}

	if dt.Spec.RefreshMode != nil {
		rm := string(*dt.Spec.RefreshMode)
		opts.RefreshMode = &rm
	}

	if dt.Spec.Initialize != nil {
		init := string(*dt.Spec.Initialize)
		opts.Initialize = &init
	}

	return opts
}

func buildAlterOptions(dt *snowplanev1alpha1.DynamicTable, id snowflake.SchemaObjectIdentifier, obs *snowflake.DynamicTableObservation) snowflake.AlterDynamicTableOptions {
	opts := snowflake.AlterDynamicTableOptions{Name: id}
	opts.UnsetFields = computeUnsetFields(dt)

	// Target lag — always send if it differs.
	if obs.ShowOutput != nil && !strings.EqualFold(dt.Spec.TargetLag, obs.ShowOutput.TargetLag) {
		tl := dt.Spec.TargetLag
		opts.TargetLag = &tl
	}

	// Warehouse — always send if it differs.
	if obs.ShowOutput != nil && !strings.EqualFold(dt.Spec.Warehouse, obs.ShowOutput.Warehouse) {
		wh := dt.Spec.Warehouse
		opts.Warehouse = &wh
	}

	// Comment — set if changed.
	if dt.Spec.Comment != nil {
		if obs.ShowOutput == nil || *dt.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = dt.Spec.Comment
		}
	}

	// ClusterBy — set if specified, or drop if previously managed but now removed.
	if len(dt.Spec.ClusterBy) > 0 {
		opts.ClusterBy = dt.Spec.ClusterBy
	} else {
		for _, p := range dt.Status.TrackedParameters {
			if p == "CLUSTER_BY" {
				opts.UnsetClusterBy = true
				break
			}
		}
	}

	// DataRetentionTimeInDays — always send if specified.
	if dt.Spec.DataRetentionTimeInDays != nil {
		opts.DataRetentionTimeInDays = dt.Spec.DataRetentionTimeInDays
	}

	// MaxDataExtensionTimeInDays — always send if specified.
	if dt.Spec.MaxDataExtensionTimeInDays != nil {
		opts.MaxDataExtensionTimeInDays = dt.Spec.MaxDataExtensionTimeInDays
	}

	return opts
}

func computeUnsetFields(dt *snowplanev1alpha1.DynamicTable) []string {
	if len(dt.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(dt.Status.TrackedParameters))
	for _, f := range dt.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if dt.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	if dt.Spec.DataRetentionTimeInDays == nil && managed["DATA_RETENTION_TIME_IN_DAYS"] {
		unset = append(unset, "DATA_RETENTION_TIME_IN_DAYS")
	}

	if dt.Spec.MaxDataExtensionTimeInDays == nil && managed["MAX_DATA_EXTENSION_TIME_IN_DAYS"] {
		unset = append(unset, "MAX_DATA_EXTENSION_TIME_IN_DAYS")
	}

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.DynamicTableSpec) []string {
	var fields []string

	// TargetLag and Warehouse are required, not optional — do not track.
	// only optional fields that can be UNSET need tracking.
	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	if len(spec.ClusterBy) > 0 {
		fields = append(fields, "CLUSTER_BY")
	}

	if spec.DataRetentionTimeInDays != nil {
		fields = append(fields, "DATA_RETENTION_TIME_IN_DAYS")
	}

	if spec.MaxDataExtensionTimeInDays != nil {
		fields = append(fields, "MAX_DATA_EXTENSION_TIME_IN_DAYS")
	}

	return fields
}

func detectDrift(dt *snowplanev1alpha1.DynamicTable, obs *snowflake.DynamicTableObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", dt.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(dt.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(dt.Status.SchemaName), obs.ShowOutput.SchemaName, true)
		d.CompareStringValue("QUERY", dt.Spec.Query, obs.ShowOutput.Text, true)

		if dt.Spec.RefreshMode != nil {
			d.CompareStringValueFold("REFRESH_MODE", string(*dt.Spec.RefreshMode), obs.ShowOutput.RefreshMode, true)
		}

		// Mutable fields.
		d.CompareStringValueFold("TARGET_LAG", dt.Spec.TargetLag, obs.ShowOutput.TargetLag, false)
		d.CompareStringValueFold("WAREHOUSE", dt.Spec.Warehouse, obs.ShowOutput.Warehouse, false)
		d.CompareString("COMMENT", dt.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareStringValue("CLUSTER_BY", strings.Join(dt.Spec.ClusterBy, ", "), obs.ShowOutput.ClusterBy, false)
	}

	return d.Result()
}
