// Package pipe implements the reconciler for Pipe resources.
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
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/pipe"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake pipes.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.PipeObservation, error)
	Create(ctx context.Context, opts snowflake.CreatePipeOptions) error
	Alter(ctx context.Context, opts snowflake.AlterPipeOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Pipe reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Pipe, Service, *snowflake.PipeObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewPipeClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.Pipe, Service, *snowflake.PipeObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for Pipe resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.Pipe, Service, *snowflake.PipeObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.Pipe, Service, *snowflake.PipeObservation]{
		ResourceNameVal:  "pipe",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.Pipe { return &snowplanev1alpha1.Pipe{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(pipe *snowplanev1alpha1.Pipe) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(pipe.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(pipe.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, pipe.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.PipeObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.PipeObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.Pipe, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterPipeOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.Pipe, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.PipeObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.Pipe, obs *reconciler.Observation[*snowflake.PipeObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.Pipe, obs *reconciler.Observation[*snowflake.PipeObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, pipe *snowplanev1alpha1.Pipe) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, pipe,
				pipe.Namespace, pipe.Spec.DatabaseRef, pipe.Spec.DatabaseName, pipe.Status.DatabaseName)
			if err != nil {
				return err
			}

			pipe.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, pipe,
				pipe.Namespace, pipe.Spec.SchemaRef, pipe.Spec.SchemaName, pipe.Status.SchemaName)
			if err != nil {
				return err
			}

			pipe.Status.SchemaName = schemaFQN

			// Resolve optional Integration ref (immutable, references QueueNotificationIntegration).
			if pipe.Spec.IntegrationRef != nil || pipe.Spec.IntegrationName != nil {
				integrationName, intErr := refresolver.PreReconcileSourceRef(ctx, c, recorder, pipe,
					pipe.Namespace, pipe.Spec.IntegrationRef, pipe.Spec.IntegrationName, pipe.Status.IntegrationName,
					"Integration",
					func() *snowplanev1alpha1.QueueNotificationIntegration {
						return &snowplanev1alpha1.QueueNotificationIntegration{}
					},
					snowplanev1alpha1.GroupVersion.WithKind("QueueNotificationIntegration"),
					func(ni *snowplanev1alpha1.QueueNotificationIntegration) string { return ni.Spec.Name },
				)
				if intErr != nil {
					return intErr
				}

				pipe.Status.IntegrationName = integrationName
			}

			// Resolve optional ErrorIntegration ref (mutable, references QueueNotificationIntegration).
			if pipe.Spec.ErrorIntegrationRef != nil || pipe.Spec.ErrorIntegrationName != nil {
				errorIntName, eiErr := refresolver.PreReconcileSourceRef(ctx, c, recorder, pipe,
					pipe.Namespace, pipe.Spec.ErrorIntegrationRef, pipe.Spec.ErrorIntegrationName, pipe.Status.ErrorIntegrationName,
					"ErrorIntegration",
					func() *snowplanev1alpha1.QueueNotificationIntegration {
						return &snowplanev1alpha1.QueueNotificationIntegration{}
					},
					snowplanev1alpha1.GroupVersion.WithKind("QueueNotificationIntegration"),
					func(ni *snowplanev1alpha1.QueueNotificationIntegration) string { return ni.Spec.Name },
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
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.PipeList{} }, ".spec.databaseRef.name", "listing pipes for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.PipeList{} }, ".spec.schemaRef.name", "listing pipes for schema watch")),
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
				&snowplanev1alpha1.QueueNotificationIntegration{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.PipeList{} }, ".spec.integrationRef.name", "listing pipes for queue notification integration watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, pipe *snowplanev1alpha1.Pipe) error {
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

func applyObservation(pipe *snowplanev1alpha1.Pipe, obs *snowflake.PipeObservation) {
	if obs.ShowOutput != nil {
		pipe.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		pipe.Status.DatabaseName = obs.ShowOutput.DatabaseName
		pipe.Status.SchemaName = obs.ShowOutput.SchemaName
		pipe.Status.NotificationChannel = obs.ShowOutput.NotificationChannel

		pipe.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(pipe *snowplanev1alpha1.Pipe, id snowflake.SchemaObjectIdentifier) snowflake.CreatePipeOptions {
	opts := snowflake.CreatePipeOptions{
		Name:          id,
		CopyStatement: pipe.Spec.CopyStatement,
		AutoIngest:    pipe.Spec.AutoIngest,
		AwsSnsTopic:   pipe.Spec.AwsSnsTopic,
		Comment:       pipe.Spec.Comment,
	}

	if pipe.Status.IntegrationName != "" {
		v := pipe.Status.IntegrationName
		opts.Integration = &v
	}

	if pipe.Status.ErrorIntegrationName != "" {
		v := pipe.Status.ErrorIntegrationName
		opts.ErrorIntegration = &v
	}

	return opts
}

func buildAlterOptions(pipe *snowplanev1alpha1.Pipe, id snowflake.SchemaObjectIdentifier, obs *snowflake.PipeObservation) snowflake.AlterPipeOptions {
	opts := snowflake.AlterPipeOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&pipe.Spec, pipe.Status.TrackedParameters)

	// Comment: set if changed.
	if pipe.Spec.Comment != nil {
		if obs.ShowOutput == nil || *pipe.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = pipe.Spec.Comment
		}
	}

	// ErrorIntegration: set if changed.
	if pipe.Status.ErrorIntegrationName != "" {
		if obs.ShowOutput == nil || pipe.Status.ErrorIntegrationName != obs.ShowOutput.ErrorIntegration {
			v := pipe.Status.ErrorIntegrationName
			opts.ErrorIntegration = &v
		}
	}

	return opts
}

func detectDrift(pipe *snowplanev1alpha1.Pipe, obs *snowflake.PipeObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", pipe.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(pipe.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(pipe.Status.SchemaName), obs.ShowOutput.SchemaName, true)
		d.CompareStringValue("DEFINITION", pipe.Spec.CopyStatement, obs.ShowOutput.Definition, true)
		if pipe.Status.IntegrationName != "" {
			v := pipe.Status.IntegrationName
			d.CompareString("INTEGRATION", &v, obs.ShowOutput.Integration, true)
		}
		d.CompareString("AWS_SNS_TOPIC", pipe.Spec.AwsSnsTopic, obs.ShowOutput.AwsSnsTopic, true)

		// Mutable fields.
		d.CompareString("COMMENT", pipe.Spec.Comment, obs.ShowOutput.Comment, false)
		if pipe.Status.ErrorIntegrationName != "" {
			v := pipe.Status.ErrorIntegrationName
			d.CompareString("ERROR_INTEGRATION", &v, obs.ShowOutput.ErrorIntegration, false)
		}
	}

	return d.Result()
}
