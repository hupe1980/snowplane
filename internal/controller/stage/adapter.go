package stage

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
	"github.com/hupe1980/snowplane/internal/tracked"
)

// adapter implements reconciler.ResourceAdapter for Stage.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "stage" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.Stage {
	return &snowplanev1alpha1.Stage{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, stage *snowplanev1alpha1.Stage) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, stage,
		stage.Namespace, stage.Spec.DatabaseRef, stage.Spec.DatabaseName, stage.Status.DatabaseName)
	if err != nil {
		return err
	}

	stage.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, stage,
		stage.Namespace, stage.Spec.SchemaRef, stage.Spec.SchemaName, stage.Status.SchemaName)
	if err != nil {
		return err
	}

	stage.Status.SchemaName = schemaFQN

	refresolver.SetDatabaseAndSchemaResolvedCondition(stage, stage.Spec.DatabaseRef, stage.Spec.DatabaseName, stage.Spec.SchemaRef, stage.Spec.SchemaName)

	return nil
}

func (a *adapter) BuildIdentifier(stage *snowplanev1alpha1.Stage) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(stage.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(stage.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, stage.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Stage{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				s, ok := o.(*snowplanev1alpha1.Stage)
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
			&snowplanev1alpha1.Stage{},
			".spec.schemaRef.name",
			func(o sigs.Object) []string {
				s, ok := o.(*snowplanev1alpha1.Stage)
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
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.StageList{} }, ".spec.databaseRef.name", "listing stages for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.StageList{} }, ".spec.schemaRef.name", "listing stages for schema watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.StageObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.StageObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.Stage, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)
	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterStageOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, stage *snowplanev1alpha1.Stage) error {
	if reconciler.ShouldSkipImmutableValidation(stage) {
		return nil
	}

	if stage.Status.ShowOutput != nil {
		if stage.Status.ShowOutput.Name != "" && !strings.EqualFold(stage.Spec.Name, stage.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", stage.Status.ShowOutput.Name, stage.Spec.Name)
		}

		// Stage type (internal/external) is immutable.
		if stage.Status.ShowOutput.Type != "" {
			wasExternal := strings.EqualFold(stage.Status.ShowOutput.Type, "EXTERNAL")
			isExternal := stage.IsExternal()

			if wasExternal != isExternal {
				return fmt.Errorf("spec.url is immutable after creation: cannot convert between internal and external stage types (current type: %s)", stage.Status.ShowOutput.Type)
			}
		}

		if stage.Status.ShowOutput.DatabaseName != "" && stage.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(stage.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, stage.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", stage.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if stage.Status.ShowOutput.SchemaName != "" && stage.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(stage.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, stage.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", stage.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}

	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.Stage, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.StageObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)
	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.Stage, obs *reconciler.Observation[*snowflake.StageObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.Stage) []string {
	return tracked.ComputeTracked(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.Stage, obs *reconciler.Observation[*snowflake.StageObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.Stage, Service, *snowflake.StageObservation] = (*adapter)(nil)
