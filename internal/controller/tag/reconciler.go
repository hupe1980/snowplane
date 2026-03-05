// Package tag implements the reconciler for Tag resources.
package tag

import (
	"context"
	"fmt"
	"sort"
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
	finalizerName = "snowplane.hupe1980.github.io/tag"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake tags.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error)
	Create(ctx context.Context, opts snowflake.CreateTagOptions) error
	Alter(ctx context.Context, opts snowflake.AlterTagOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Tag reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Tag, Service, *snowflake.TagObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewTagClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.Tag, Service, *snowflake.TagObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for Tag resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.Tag, Service, *snowflake.TagObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.Tag, Service, *snowflake.TagObservation]{
		ResourceNameVal:  "tag",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.Tag { return &snowplanev1alpha1.Tag{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(tag *snowplanev1alpha1.Tag) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(tag.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(tag.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, tag.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.TagObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.Tag, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterTagOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.Tag, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.TagObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.Tag, obs *reconciler.Observation[*snowflake.TagObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.Tag, obs *reconciler.Observation[*snowflake.TagObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		SupportsCoA:      true,
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, tag *snowplanev1alpha1.Tag) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, tag,
				tag.Namespace, tag.Spec.DatabaseRef, tag.Spec.DatabaseName, tag.Status.DatabaseName)
			if err != nil {
				return err
			}

			tag.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, tag,
				tag.Namespace, tag.Spec.SchemaRef, tag.Spec.SchemaName, tag.Status.SchemaName)
			if err != nil {
				return err
			}

			tag.Status.SchemaName = schemaFQN

			refresolver.SetDatabaseAndSchemaResolvedCondition(tag, tag.Spec.DatabaseRef, tag.Spec.DatabaseName, tag.Spec.SchemaRef, tag.Spec.SchemaName)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.Tag{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					t, ok := o.(*snowplanev1alpha1.Tag)
					if !ok || t.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{t.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.Tag{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					t, ok := o.(*snowplanev1alpha1.Tag)
					if !ok || t.Spec.SchemaRef == nil {
						return nil
					}

					return []string{t.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.TagList{} }, ".spec.databaseRef.name", "listing tags for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.TagList{} }, ".spec.schemaRef.name", "listing tags for schema watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, tag *snowplanev1alpha1.Tag) error {
	if reconciler.ShouldSkipImmutableValidation(tag) {
		return nil
	}

	if tag.Status.ShowOutput != nil {
		if tag.Status.ShowOutput.Name != "" && !strings.EqualFold(tag.Spec.Name, tag.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", tag.Status.ShowOutput.Name, tag.Spec.Name)
		}

		if tag.Status.ShowOutput.DatabaseName != "" && tag.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(tag.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, tag.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", tag.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if tag.Status.ShowOutput.SchemaName != "" && tag.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(tag.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, tag.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", tag.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func applyObservation(tag *snowplanev1alpha1.Tag, obs *snowflake.TagObservation) {
	if obs.ShowOutput != nil {
		tag.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		tag.Status.DatabaseName = obs.ShowOutput.DatabaseName
		tag.Status.SchemaName = obs.ShowOutput.SchemaName

		tag.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(tag *snowplanev1alpha1.Tag, id snowflake.SchemaObjectIdentifier) snowflake.CreateTagOptions {
	return snowflake.CreateTagOptions{
		Name:          id,
		AllowedValues: tag.Spec.AllowedValues,
		Comment:       tag.Spec.Comment,
	}
}

func buildAlterOptions(tag *snowplanev1alpha1.Tag, id snowflake.SchemaObjectIdentifier, obs *snowflake.TagObservation) snowflake.AlterTagOptions {
	opts := snowflake.AlterTagOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&tag.Spec, tag.Status.TrackedParameters)

	// Compare allowed values.
	if len(tag.Spec.AllowedValues) > 0 {
		desiredSorted := make([]string, len(tag.Spec.AllowedValues))
		copy(desiredSorted, tag.Spec.AllowedValues)
		sort.Strings(desiredSorted)

		observedCSV := ""
		if obs.ShowOutput != nil {
			observedCSV = obs.ShowOutput.AllowedValues
		}

		if strings.Join(desiredSorted, ",") != observedCSV {
			av := make([]string, len(tag.Spec.AllowedValues))
			copy(av, tag.Spec.AllowedValues)
			opts.AllowedValues = &av
		}
	} else if obs.ShowOutput != nil && obs.ShowOutput.AllowedValues != "" {
		// Spec has no allowed values but Snowflake has them → unset.
		empty := []string{}
		opts.AllowedValues = &empty
	}

	// Compare comment.
	if tag.Spec.Comment != nil {
		if obs.ShowOutput == nil || *tag.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = tag.Spec.Comment
		}
	}

	return opts
}

func detectDrift(tag *snowplanev1alpha1.Tag, obs *snowflake.TagObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", tag.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(tag.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(tag.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		// Mutable fields.
		d.CompareString("COMMENT", tag.Spec.Comment, obs.ShowOutput.Comment, false)

		// Allowed values comparison: sort desired and compare to observed CSV.
		desiredCSV := ""
		if len(tag.Spec.AllowedValues) > 0 {
			sorted := make([]string, len(tag.Spec.AllowedValues))
			copy(sorted, tag.Spec.AllowedValues)
			sort.Strings(sorted)

			desiredCSV = strings.Join(sorted, ",")
		}

		d.CompareStringValue("ALLOWED_VALUES", desiredCSV, obs.ShowOutput.AllowedValues, false)
	}

	return d.Result()
}
