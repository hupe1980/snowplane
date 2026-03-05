// Package sequence implements the reconciler for Sequence resources.
package sequence

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
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
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Sequence, Service, *snowflake.SequenceObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewSequenceClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.Sequence, Service, *snowflake.SequenceObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for Sequence resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.Sequence, Service, *snowflake.SequenceObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.Sequence, Service, *snowflake.SequenceObservation]{
		ResourceNameVal:  "sequence",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.Sequence { return &snowplanev1alpha1.Sequence{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(seq *snowplanev1alpha1.Sequence) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(seq.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(seq.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, seq.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.SequenceObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.SequenceObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.Sequence, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterSequenceOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.Sequence, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.SequenceObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.Sequence, obs *reconciler.Observation[*snowflake.SequenceObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.Sequence, obs *reconciler.Observation[*snowflake.SequenceObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		SupportsCoA:      true,
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, seq *snowplanev1alpha1.Sequence) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, seq,
				seq.Namespace, seq.Spec.DatabaseRef, seq.Spec.DatabaseName, seq.Status.DatabaseName)
			if err != nil {
				return err
			}

			seq.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, seq,
				seq.Namespace, seq.Spec.SchemaRef, seq.Spec.SchemaName, seq.Status.SchemaName)
			if err != nil {
				return err
			}

			seq.Status.SchemaName = schemaFQN

			refresolver.SetDatabaseAndSchemaResolvedCondition(seq, seq.Spec.DatabaseRef, seq.Spec.DatabaseName, seq.Spec.SchemaRef, seq.Spec.SchemaName)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.Sequence{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.Sequence)
					if !ok || s.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{s.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.Sequence{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.Sequence)
					if !ok || s.Spec.SchemaRef == nil {
						return nil
					}

					return []string{s.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.SequenceList{} }, ".spec.databaseRef.name", "listing sequences for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.SequenceList{} }, ".spec.schemaRef.name", "listing sequences for schema watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, seq *snowplanev1alpha1.Sequence) error {
	if reconciler.ShouldSkipImmutableValidation(seq) {
		return nil
	}

	if seq.Status.ShowOutput != nil {
		if seq.Status.ShowOutput.Name != "" && !strings.EqualFold(seq.Spec.Name, seq.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", seq.Status.ShowOutput.Name, seq.Spec.Name)
		}

		if seq.Status.ShowOutput.DatabaseName != "" && seq.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(seq.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, seq.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", seq.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if seq.Status.ShowOutput.SchemaName != "" && seq.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(seq.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, seq.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", seq.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
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

		seq.Status.ShowOutput = obs.ShowOutput
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
