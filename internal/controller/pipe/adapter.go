package pipe

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

// adapter implements reconciler.ResourceAdapter for Pipe.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "pipe" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.Pipe {
	return &snowplanev1alpha1.Pipe{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, pipe *snowplanev1alpha1.Pipe) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, pipe,
		pipe.Namespace, pipe.Spec.DatabaseRef, pipe.Spec.DatabaseName, pipe.Status.DatabaseName)
	if err != nil {
		return err
	}

	pipe.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, pipe,
		pipe.Namespace, pipe.Spec.SchemaRef, pipe.Spec.SchemaName, pipe.Status.SchemaName)
	if err != nil {
		return err
	}

	pipe.Status.SchemaName = schemaFQN

	// Resolve optional Integration ref (immutable, references NotificationIntegration).
	if pipe.Spec.IntegrationRef != nil || pipe.Spec.IntegrationName != nil {
		integrationName, intErr := refresolver.PreReconcileSourceRef(ctx, a.client, a.recorder, pipe,
			pipe.Namespace, pipe.Spec.IntegrationRef, pipe.Spec.IntegrationName, pipe.Status.IntegrationName,
			"Integration",
			func() *snowplanev1alpha1.NotificationIntegration {
				return &snowplanev1alpha1.NotificationIntegration{}
			},
			snowplanev1alpha1.GroupVersion.WithKind("NotificationIntegration"),
			func(ni *snowplanev1alpha1.NotificationIntegration) string { return ni.Spec.Name },
		)
		if intErr != nil {
			return intErr
		}

		pipe.Status.IntegrationName = integrationName
	}

	// Resolve optional ErrorIntegration ref (mutable, references NotificationIntegration).
	if pipe.Spec.ErrorIntegrationRef != nil || pipe.Spec.ErrorIntegrationName != nil {
		errorIntName, eiErr := refresolver.PreReconcileSourceRef(ctx, a.client, a.recorder, pipe,
			pipe.Namespace, pipe.Spec.ErrorIntegrationRef, pipe.Spec.ErrorIntegrationName, pipe.Status.ErrorIntegrationName,
			"ErrorIntegration",
			func() *snowplanev1alpha1.NotificationIntegration {
				return &snowplanev1alpha1.NotificationIntegration{}
			},
			snowplanev1alpha1.GroupVersion.WithKind("NotificationIntegration"),
			func(ni *snowplanev1alpha1.NotificationIntegration) string { return ni.Spec.Name },
		)
		if eiErr != nil {
			return eiErr
		}

		pipe.Status.ErrorIntegrationName = errorIntName
	}

	// Build dynamic refs list for condition.
	refs := []refresolver.RefDescriptor{
		{KindLabel: "database", Ref: pipe.Spec.DatabaseRef, RawName: pipe.Spec.DatabaseName},
		{KindLabel: "schema", Ref: pipe.Spec.SchemaRef, RawName: pipe.Spec.SchemaName},
	}
	if pipe.Spec.IntegrationRef != nil || pipe.Spec.IntegrationName != nil {
		refs = append(refs, refresolver.RefDescriptor{KindLabel: "integration", Ref: pipe.Spec.IntegrationRef, RawName: pipe.Spec.IntegrationName})
	}
	if pipe.Spec.ErrorIntegrationRef != nil || pipe.Spec.ErrorIntegrationName != nil {
		refs = append(refs, refresolver.RefDescriptor{KindLabel: "errorIntegration", Ref: pipe.Spec.ErrorIntegrationRef, RawName: pipe.Spec.ErrorIntegrationName})
	}

	refresolver.SetAllReferencesResolvedCondition(pipe, refs...)

	return nil
}

func (a *adapter) BuildIdentifier(pipe *snowplanev1alpha1.Pipe) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(pipe.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(pipe.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, pipe.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Pipe{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				p, ok := o.(*snowplanev1alpha1.Pipe)
				if !ok || p.Spec.DatabaseRef == nil {
					return nil
				}

				return []string{p.Spec.DatabaseRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
		}

		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Pipe{},
			".spec.schemaRef.name",
			func(o sigs.Object) []string {
				p, ok := o.(*snowplanev1alpha1.Pipe)
				if !ok || p.Spec.SchemaRef == nil {
					return nil
				}

				return []string{p.Spec.SchemaRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.PipeList{} }, ".spec.databaseRef.name", "listing pipes for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.PipeList{} }, ".spec.schemaRef.name", "listing pipes for schema watch")),
		)

		// IntegrationRef index.
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Pipe{},
			".spec.integrationRef.name",
			func(o sigs.Object) []string {
				p, ok := o.(*snowplanev1alpha1.Pipe)
				if !ok || p.Spec.IntegrationRef == nil {
					return nil
				}

				return []string{p.Spec.IntegrationRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.integrationRef.name: %w", err)
		}

		// ErrorIntegrationRef index.
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Pipe{},
			".spec.errorIntegrationRef.name",
			func(o sigs.Object) []string {
				p, ok := o.(*snowplanev1alpha1.Pipe)
				if !ok || p.Spec.ErrorIntegrationRef == nil {
					return nil
				}

				return []string{p.Spec.ErrorIntegrationRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.errorIntegrationRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.NotificationIntegration{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.PipeList{} }, ".spec.integrationRef.name", "listing pipes for notification integration watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.PipeObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.PipeObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.Pipe, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)
	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterPipeOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, pipe *snowplanev1alpha1.Pipe) error {
	if reconciler.ShouldSkipImmutableValidation(pipe) {
		return nil
	}

	if pipe.Status.ShowOutput != nil {
		if pipe.Status.ShowOutput.Name != "" && !strings.EqualFold(pipe.Spec.Name, pipe.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", pipe.Status.ShowOutput.Name, pipe.Spec.Name)
		}

		if pipe.Status.ShowOutput.DatabaseName != "" && pipe.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(pipe.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, pipe.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", pipe.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if pipe.Status.ShowOutput.SchemaName != "" && pipe.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(pipe.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, pipe.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", pipe.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}

		// copyStatement is immutable — it defines the pipe's COPY INTO statement.
		if pipe.Status.ShowOutput.Definition != "" && pipe.Spec.CopyStatement != pipe.Status.ShowOutput.Definition {
			return fmt.Errorf("spec.copyStatement is immutable after creation (current: %q, desired: %q)", pipe.Status.ShowOutput.Definition, pipe.Spec.CopyStatement)
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.Pipe, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.PipeObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)
	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.Pipe, obs *reconciler.Observation[*snowflake.PipeObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.Pipe) []string {
	return tracked.ComputeTracked(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.Pipe, obs *reconciler.Observation[*snowflake.PipeObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.Pipe, Service, *snowflake.PipeObservation] = (*adapter)(nil)
