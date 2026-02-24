// Package stream implements the reconciler for Stream resources.
package stream

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
	finalizerName = "snowplane.hupe1980.github.io/stream"
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

// NewReconciler returns a new Stream reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Stream, Service, *snowflake.StreamObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Stream, Service, *snowflake.StreamObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.Stream, Service, *snowflake.StreamObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Stream, Service, *snowflake.StreamObservation]{
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

func applyObservation(stream *snowplanev1alpha1.Stream, obs *snowflake.StreamObservation) {
	if obs.ShowOutput != nil {
		stream.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		stream.Status.DatabaseName = obs.ShowOutput.DatabaseName
		stream.Status.SchemaName = obs.ShowOutput.SchemaName

		stream.Status.ShowOutput = &snowplanev1alpha1.StreamShowOutput{
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

func buildCreateOptions(stream *snowplanev1alpha1.Stream, id snowflake.SchemaObjectIdentifier) snowflake.CreateStreamOptions {
	return snowflake.CreateStreamOptions{
		Name:            id,
		SourceType:      snowflake.StreamSourceType(stream.Spec.SourceType),
		SourceName:      stream.Spec.SourceName,
		AppendOnly:      stream.Spec.AppendOnly,
		InsertOnly:      stream.Spec.InsertOnly,
		ShowInitialRows: stream.Spec.ShowInitialRows,
		Comment:         stream.Spec.Comment,
	}
}

func buildAlterOptions(stream *snowplanev1alpha1.Stream, id snowflake.SchemaObjectIdentifier, obs *snowflake.StreamObservation) snowflake.AlterStreamOptions {
	opts := snowflake.AlterStreamOptions{Name: id}
	opts.UnsetFields = computeUnsetFields(stream)

	// Only comment is mutable for streams.
	if stream.Spec.Comment != nil {
		if obs.ShowOutput == nil || *stream.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = stream.Spec.Comment
		}
	}

	return opts
}

func computeUnsetFields(stream *snowplanev1alpha1.Stream) []string {
	if len(stream.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(stream.Status.TrackedParameters))
	for _, f := range stream.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if stream.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.StreamSpec) []string {
	var fields []string

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	return fields
}

func detectDrift(stream *snowplanev1alpha1.Stream, obs *snowflake.StreamObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", stream.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(stream.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(stream.Status.SchemaName), obs.ShowOutput.SchemaName, true)
		d.CompareStringValueFold("SOURCE", stream.Spec.SourceName, obs.ShowOutput.TableName, true)

		// Source type: map spec enum to Snowflake mode.
		expectedMode := specSourceTypeToMode(stream.Spec.SourceType, stream.Spec.AppendOnly, stream.Spec.InsertOnly)
		d.CompareStringValueFold("MODE", expectedMode, obs.ShowOutput.Mode, true)

		// Mutable fields.
		d.CompareString("COMMENT", stream.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	return d.Result()
}

// specSourceTypeToMode maps the spec's source type + flags to the expected SHOW STREAMS "mode" value.
func specSourceTypeToMode(_ snowplanev1alpha1.StreamSourceType, appendOnly, insertOnly *bool) string {
	switch {
	case appendOnly != nil && *appendOnly:
		return "APPEND_ONLY"
	case insertOnly != nil && *insertOnly:
		return "INSERT_ONLY"
	default:
		return "DEFAULT"
	}
}
