package rowaccesspolicy

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

// adapter implements reconciler.ResourceAdapter for RowAccessPolicy.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "rowaccesspolicy" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.RowAccessPolicy {
	return &snowplanev1alpha1.RowAccessPolicy{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, rap *snowplanev1alpha1.RowAccessPolicy) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, rap,
		rap.Namespace, rap.Spec.DatabaseRef, rap.Spec.DatabaseName, rap.Status.DatabaseName)
	if err != nil {
		return err
	}

	rap.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, rap,
		rap.Namespace, rap.Spec.SchemaRef, rap.Spec.SchemaName, rap.Status.SchemaName)
	if err != nil {
		return err
	}

	rap.Status.SchemaName = schemaFQN

	refresolver.SetDatabaseAndSchemaResolvedCondition(rap, rap.Spec.DatabaseRef, rap.Spec.DatabaseName, rap.Spec.SchemaRef, rap.Spec.SchemaName)

	return nil
}

func (a *adapter) BuildIdentifier(rap *snowplanev1alpha1.RowAccessPolicy) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(rap.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(rap.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, rap.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.RowAccessPolicy{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				rap, ok := o.(*snowplanev1alpha1.RowAccessPolicy)
				if !ok || rap.Spec.DatabaseRef == nil {
					return nil
				}

				return []string{rap.Spec.DatabaseRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
		}

		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.RowAccessPolicy{},
			".spec.schemaRef.name",
			func(o sigs.Object) []string {
				rap, ok := o.(*snowplanev1alpha1.RowAccessPolicy)
				if !ok || rap.Spec.SchemaRef == nil {
					return nil
				}

				return []string{rap.Spec.SchemaRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.RowAccessPolicyList{} }, ".spec.databaseRef.name", "listing row access policies for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.RowAccessPolicyList{} }, ".spec.schemaRef.name", "listing row access policies for schema watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.RowAccessPolicyObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.RowAccessPolicyObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.RowAccessPolicy, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)
	opts.UseCreateOrAlter = snowplanev1alpha1.IsCreateOrAlter(obj.GetAnnotations())

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterRowAccessPolicyOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, rap *snowplanev1alpha1.RowAccessPolicy) error {
	if reconciler.ShouldSkipImmutableValidation(rap) {
		return nil
	}

	if rap.Status.ShowOutput != nil {
		if rap.Status.ShowOutput.Name != "" && !strings.EqualFold(rap.Spec.Name, rap.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", rap.Status.ShowOutput.Name, rap.Spec.Name)
		}

		if rap.Status.ShowOutput.DatabaseName != "" && rap.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(rap.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, rap.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", rap.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if rap.Status.ShowOutput.SchemaName != "" && rap.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(rap.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, rap.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", rap.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.RowAccessPolicy, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.RowAccessPolicyObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)
	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.RowAccessPolicy, obs *reconciler.Observation[*snowflake.RowAccessPolicyObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.RowAccessPolicy) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.RowAccessPolicy, obs *reconciler.Observation[*snowflake.RowAccessPolicyObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) PostCreate(_ *snowplanev1alpha1.RowAccessPolicy) {}
func (a *adapter) PostUpdate(_ *snowplanev1alpha1.RowAccessPolicy, _ bool, _ reconciler.AlterOptions) {
}
func (a *adapter) SupportsCreateOrAlter() bool { return true }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.RowAccessPolicy, Service, *snowflake.RowAccessPolicyObservation] = (*adapter)(nil)
