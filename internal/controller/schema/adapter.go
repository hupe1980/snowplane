package schema

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

// adapter implements reconciler.ResourceAdapter for Schema.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "schema" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.Schema {
	return &snowplanev1alpha1.Schema{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, schema *snowplanev1alpha1.Schema) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, schema,
		schema.Namespace, schema.Spec.DatabaseRef, schema.Spec.DatabaseName, schema.Status.DatabaseName)
	if err != nil {
		return err
	}

	schema.Status.DatabaseName = dbFQN

	refresolver.SetDatabaseResolvedCondition(schema, schema.Spec.DatabaseRef, schema.Spec.DatabaseName, dbFQN)

	return nil
}

func (a *adapter) BuildIdentifier(schema *snowplanev1alpha1.Schema) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(schema.Status.DatabaseName)
	return snowflake.NewDatabaseObjectIdentifier(dbName, schema.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Schema{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				sch, ok := o.(*snowplanev1alpha1.Schema)
				if !ok || sch.Spec.DatabaseRef == nil {
					return nil
				}

				return []string{sch.Spec.DatabaseRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.SchemaList{} }, ".spec.databaseRef.name", "listing schemas for database watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.SchemaObservation], error) {
	did, err := reconciler.AssertIdentifier[snowflake.DatabaseObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, did)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.SchemaObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.Schema, id reconciler.Identifier) error {
	did, err := reconciler.AssertIdentifier[snowflake.DatabaseObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, did)
	opts.UseCreateOrAlter = snowplanev1alpha1.IsCreateOrAlter(obj.GetAnnotations())

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterSchemaOptions](opts)
	if err != nil {
		return err
	}

	return svc.Alter(ctx, *ao)
}

func (a *adapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	did, err := reconciler.AssertIdentifier[snowflake.DatabaseObjectIdentifier](id)
	if err != nil {
		return err
	}

	return svc.Drop(ctx, did)
}

func (a *adapter) ValidateImmutableFields(_ context.Context, schema *snowplanev1alpha1.Schema) error {
	if reconciler.ShouldSkipImmutableValidation(schema) {
		return nil
	}

	if schema.Status.ShowOutput != nil {
		isTransient := schema.Status.ShowOutput.Kind == "TRANSIENT"
		if schema.Spec.Transient != isTransient {
			return fmt.Errorf("spec.transient is immutable after creation (current: %v, desired: %v)", isTransient, schema.Spec.Transient)
		}

		if schema.Status.ShowOutput.Name != "" && !strings.EqualFold(schema.Spec.Name, schema.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", schema.Status.ShowOutput.Name, schema.Spec.Name)
		}

		if schema.Status.ShowOutput.DatabaseName != "" && schema.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(schema.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, schema.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", schema.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.Schema, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.SchemaObservation]) (reconciler.AlterOptions, error) {
	did, err := reconciler.AssertIdentifier[snowflake.DatabaseObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, did, detail)
	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.Schema, obs *reconciler.Observation[*snowflake.SchemaObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.Schema) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.Schema, obs *reconciler.Observation[*snowflake.SchemaObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) SupportsCreateOrAlter() bool { return true }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.Schema, Service, *snowflake.SchemaObservation] = (*adapter)(nil)
