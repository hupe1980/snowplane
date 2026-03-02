package sequence

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

// adapter implements reconciler.ResourceAdapter for Sequence.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "sequence" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.Sequence {
	return &snowplanev1alpha1.Sequence{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, seq *snowplanev1alpha1.Sequence) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, seq,
		seq.Namespace, seq.Spec.DatabaseRef, seq.Spec.DatabaseName, seq.Status.DatabaseName)
	if err != nil {
		return err
	}

	seq.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, seq,
		seq.Namespace, seq.Spec.SchemaRef, seq.Spec.SchemaName, seq.Status.SchemaName)
	if err != nil {
		return err
	}

	seq.Status.SchemaName = schemaFQN

	refresolver.SetDatabaseAndSchemaResolvedCondition(seq, seq.Spec.DatabaseRef, seq.Spec.DatabaseName, seq.Spec.SchemaRef, seq.Spec.SchemaName)

	return nil
}

func (a *adapter) BuildIdentifier(seq *snowplanev1alpha1.Sequence) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(seq.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(seq.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, seq.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
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
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.SequenceList{} }, ".spec.databaseRef.name", "listing sequences for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.SequenceList{} }, ".spec.schemaRef.name", "listing sequences for schema watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.SequenceObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.SequenceObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.Sequence, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)
	opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterSequenceOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, seq *snowplanev1alpha1.Sequence) error {
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

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.Sequence, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.SequenceObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)

	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.Sequence, obs *reconciler.Observation[*snowflake.SequenceObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.Sequence) []string {
	return tracked.ComputeTracked(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.Sequence, obs *reconciler.Observation[*snowflake.SequenceObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) SupportsCreateOrAlter() bool { return true }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.Sequence, Service, *snowflake.SequenceObservation] = (*adapter)(nil)
