package tag

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
)

// adapter implements reconciler.ResourceAdapter for Tag.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "tag" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.Tag {
	return &snowplanev1alpha1.Tag{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, tag *snowplanev1alpha1.Tag) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, tag,
		tag.Namespace, tag.Spec.DatabaseRef, tag.Spec.DatabaseName, tag.Status.DatabaseName)
	if err != nil {
		return err
	}

	tag.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, tag,
		tag.Namespace, tag.Spec.SchemaRef, tag.Spec.SchemaName, tag.Status.SchemaName)
	if err != nil {
		return err
	}

	tag.Status.SchemaName = schemaFQN

	refresolver.SetDatabaseAndSchemaResolvedCondition(tag, tag.Spec.DatabaseRef, tag.Spec.DatabaseName, tag.Spec.SchemaRef, tag.Spec.SchemaName)

	return nil
}

func (a *adapter) BuildIdentifier(tag *snowplanev1alpha1.Tag) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(tag.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(tag.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, tag.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
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
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.TagList{} }, ".spec.databaseRef.name", "listing tags for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.TagList{} }, ".spec.schemaRef.name", "listing tags for schema watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.TagObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.TagObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.Tag, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)
	opts.UseCreateOrAlter = snowplanev1alpha1.IsCreateOrAlter(obj.GetAnnotations())

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterTagOptions](opts)
	if err != nil {
		return err
	}

	return svc.Alter(ctx, *ao)
}

func (a *adapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	return svc.Drop(ctx, sid)
}

func (a *adapter) ValidateImmutableFields(_ context.Context, tag *snowplanev1alpha1.Tag) error {
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

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.Tag, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.TagObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)
	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.Tag, obs *reconciler.Observation[*snowflake.TagObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.Tag) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.Tag, obs *reconciler.Observation[*snowflake.TagObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) SupportsCreateOrAlter() bool { return true }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.Tag, Service, *snowflake.TagObservation] = (*adapter)(nil)
