package passwordpolicy

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

// adapter implements reconciler.ResourceAdapter for PasswordPolicy.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "passwordpolicy" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.PasswordPolicy {
	return &snowplanev1alpha1.PasswordPolicy{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, pp *snowplanev1alpha1.PasswordPolicy) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, pp,
		pp.Namespace, pp.Spec.DatabaseRef, pp.Spec.DatabaseName, pp.Status.DatabaseName)
	if err != nil {
		return err
	}

	pp.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, pp,
		pp.Namespace, pp.Spec.SchemaRef, pp.Spec.SchemaName, pp.Status.SchemaName)
	if err != nil {
		return err
	}

	pp.Status.SchemaName = schemaFQN

	refresolver.SetDatabaseAndSchemaResolvedCondition(pp, pp.Spec.DatabaseRef, pp.Spec.DatabaseName, pp.Spec.SchemaRef, pp.Spec.SchemaName)

	return nil
}

func (a *adapter) BuildIdentifier(pp *snowplanev1alpha1.PasswordPolicy) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(pp.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(pp.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, pp.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.PasswordPolicy{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				pp, ok := o.(*snowplanev1alpha1.PasswordPolicy)
				if !ok || pp.Spec.DatabaseRef == nil {
					return nil
				}

				return []string{pp.Spec.DatabaseRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
		}

		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.PasswordPolicy{},
			".spec.schemaRef.name",
			func(o sigs.Object) []string {
				pp, ok := o.(*snowplanev1alpha1.PasswordPolicy)
				if !ok || pp.Spec.SchemaRef == nil {
					return nil
				}

				return []string{pp.Spec.SchemaRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.PasswordPolicyList{} }, ".spec.databaseRef.name", "listing password policies for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.PasswordPolicyList{} }, ".spec.schemaRef.name", "listing password policies for schema watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.PasswordPolicyObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.PasswordPolicyObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.PasswordPolicy, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)
	opts.UseCreateOrAlter = snowplanev1alpha1.IsCreateOrAlter(obj.GetAnnotations())

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterPasswordPolicyOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, pp *snowplanev1alpha1.PasswordPolicy) error {
	if reconciler.ShouldSkipImmutableValidation(pp) {
		return nil
	}

	if pp.Status.ShowOutput != nil {
		if pp.Status.ShowOutput.Name != "" && !strings.EqualFold(pp.Spec.Name, pp.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", pp.Status.ShowOutput.Name, pp.Spec.Name)
		}

		if pp.Status.ShowOutput.DatabaseName != "" && pp.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(pp.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, pp.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", pp.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if pp.Status.ShowOutput.SchemaName != "" && pp.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(pp.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, pp.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", pp.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.PasswordPolicy, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.PasswordPolicyObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)
	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.PasswordPolicy, obs *reconciler.Observation[*snowflake.PasswordPolicyObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.PasswordPolicy) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.PasswordPolicy, obs *reconciler.Observation[*snowflake.PasswordPolicyObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) PostCreate(_ *snowplanev1alpha1.PasswordPolicy) {}
func (a *adapter) PostUpdate(_ *snowplanev1alpha1.PasswordPolicy, _ bool, _ reconciler.AlterOptions) {
}
func (a *adapter) SupportsCreateOrAlter() bool { return true }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.PasswordPolicy, Service, *snowflake.PasswordPolicyObservation] = (*adapter)(nil)
