// Package streamondynamictable implements the reconciler for StreamOnDynamicTable resources.
package streamondynamictable

import (
	"context"

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
	finalizerName = "snowplane.hupe1980.github.io/streamondynamictable"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake streams.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error)
	Create(ctx context.Context, opts snowflake.CreateStreamOptions) error
	Alter(ctx context.Context, opts snowflake.AlterStreamOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new StreamOnDynamicTable reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.StreamOnDynamicTable, Service, *snowflake.StreamObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.StreamOnDynamicTable, Service, *snowflake.StreamObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.StreamOnDynamicTable, Service, *snowflake.StreamObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.StreamOnDynamicTable, Service, *snowflake.StreamObservation]{
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

	return snowflake.NewStreamClient(sfC), cleanup, nil
}

func applyObservation(obj *snowplanev1alpha1.StreamOnDynamicTable, obs *snowflake.StreamObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		obj.Status.DatabaseName = obs.ShowOutput.DatabaseName
		obj.Status.SchemaName = obs.ShowOutput.SchemaName

		obj.Status.ShowOutput = &snowplanev1alpha1.StreamShowOutput{
			CreatedOn:    obs.ShowOutput.CreatedOn,
			Name:         obs.ShowOutput.Name,
			DatabaseName: obs.ShowOutput.DatabaseName,
			SchemaName:   obs.ShowOutput.SchemaName,
			Owner:        obs.ShowOutput.Owner,
			Comment:      obs.ShowOutput.Comment,
			TableName:    obs.ShowOutput.TableName,
			SourceType:   obs.ShowOutput.SourceType,
			Mode:         obs.ShowOutput.Mode,
			Stale:        obs.ShowOutput.Stale,
			StaleAfter:   obs.ShowOutput.StaleAfter,
		}
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.StreamOnDynamicTable, id snowflake.SchemaObjectIdentifier) snowflake.CreateStreamOptions {
	return snowflake.CreateStreamOptions{
		Name:            id,
		SourceType:      snowflake.StreamSourceDynamicTable,
		SourceName:      obj.Spec.DynamicTable,
		AppendOnly:      obj.Spec.AppendOnly,
		ShowInitialRows: obj.Spec.ShowInitialRows,
		Comment:         obj.Spec.Comment,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.StreamOnDynamicTable, id snowflake.SchemaObjectIdentifier, obs *snowflake.StreamObservation) snowflake.AlterStreamOptions {
	opts := snowflake.AlterStreamOptions{Name: id}
	opts.UnsetFields = computeUnsetFields(obj)

	if obj.Spec.Comment != nil {
		if obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	return opts
}

func computeUnsetFields(obj *snowplanev1alpha1.StreamOnDynamicTable) []string {
	if len(obj.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(obj.Status.TrackedParameters))
	for _, f := range obj.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if obj.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.StreamOnDynamicTableSpec) []string {
	var fields []string

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	return fields
}

func detectDrift(obj *snowplanev1alpha1.StreamOnDynamicTable, obs *snowflake.StreamObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName), obs.ShowOutput.SchemaName, true)
		d.CompareStringValueFold("SOURCE", obj.Spec.DynamicTable, obs.ShowOutput.TableName, true)

		expectedMode := "DEFAULT"
		if obj.Spec.AppendOnly != nil && *obj.Spec.AppendOnly {
			expectedMode = "APPEND_ONLY"
		}

		d.CompareStringValueFold("MODE", expectedMode, obs.ShowOutput.Mode, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	return d.Result()
}
