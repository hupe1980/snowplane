// Package sequence implements the reconciler for Sequence resources.
package sequence

import (
	"context"
	"strconv"

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
	finalizerName = "snowplane.hupe1980.github.io/sequence"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake sequences.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.SequenceObservation, error)
	Create(ctx context.Context, opts snowflake.CreateSequenceOptions) error
	Alter(ctx context.Context, opts snowflake.AlterSequenceOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Sequence reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Sequence, Service, *snowflake.SequenceObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Sequence, Service, *snowflake.SequenceObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.Sequence, Service, *snowflake.SequenceObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Sequence, Service, *snowflake.SequenceObservation]{
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

	return snowflake.NewSequenceClient(sfC), cleanup, nil
}

func applyObservation(seq *snowplanev1alpha1.Sequence, obs *snowflake.SequenceObservation) {
	if obs.ShowOutput != nil {
		seq.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		seq.Status.DatabaseName = obs.ShowOutput.DatabaseName
		seq.Status.SchemaName = obs.ShowOutput.SchemaName

		seq.Status.ShowOutput = &snowplanev1alpha1.SequenceShowOutput{
			CreatedOn:    obs.ShowOutput.CreatedOn,
			Name:         obs.ShowOutput.Name,
			DatabaseName: obs.ShowOutput.DatabaseName,
			SchemaName:   obs.ShowOutput.SchemaName,
			Owner:        obs.ShowOutput.Owner,
			Comment:      obs.ShowOutput.Comment,
			NextValue:    obs.ShowOutput.NextValue,
			Interval:     obs.ShowOutput.Interval,
			Ordering:     obs.ShowOutput.Ordering,
		}
	}
}

func buildCreateOptions(seq *snowplanev1alpha1.Sequence, id snowflake.SchemaObjectIdentifier) snowflake.CreateSequenceOptions {
	return snowflake.CreateSequenceOptions{
		Name:      id,
		Start:     seq.Spec.Start,
		Increment: seq.Spec.Increment,
		Ordering:  seq.Spec.Ordering,
		Comment:   seq.Spec.Comment,
	}
}

func buildAlterOptions(seq *snowplanev1alpha1.Sequence, id snowflake.SchemaObjectIdentifier, obs *snowflake.SequenceObservation) snowflake.AlterSequenceOptions {
	opts := snowflake.AlterSequenceOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&seq.Spec, seq.Status.TrackedParameters)

	// Increment — compare against SHOW output to avoid unnecessary ALTER.
	if seq.Spec.Increment != nil {
		if obs == nil || obs.ShowOutput == nil || strconv.FormatInt(*seq.Spec.Increment, 10) != obs.ShowOutput.Interval {
			opts.Increment = seq.Spec.Increment
		}
	}

	// Ordering — compare against SHOW output.
	if seq.Spec.Ordering != nil {
		if obs == nil || obs.ShowOutput == nil || *seq.Spec.Ordering != obs.ShowOutput.Ordering {
			opts.Ordering = seq.Spec.Ordering
		}
	}

	// Comment — compare against SHOW output.
	if seq.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *seq.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = seq.Spec.Comment
		}
	}

	return opts
}

func detectDrift(seq *snowplanev1alpha1.Sequence, obs *snowflake.SequenceObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", seq.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(seq.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(seq.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		// Mutable fields.
		d.CompareString("COMMENT", seq.Spec.Comment, obs.ShowOutput.Comment, false)

		if seq.Spec.Increment != nil {
			d.CompareStringValue("INCREMENT", strconv.FormatInt(*seq.Spec.Increment, 10), obs.ShowOutput.Interval, false)
		}

		if seq.Spec.Ordering != nil {
			d.CompareStringValue("ORDERING", *seq.Spec.Ordering, obs.ShowOutput.Ordering, false)
		}
	}

	return d.Result()
}
