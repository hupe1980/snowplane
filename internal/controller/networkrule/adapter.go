package networkrule

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

// adapter implements reconciler.ResourceAdapter for NetworkRule.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "networkrule" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.NetworkRule {
	return &snowplanev1alpha1.NetworkRule{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, nr *snowplanev1alpha1.NetworkRule) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, nr,
		nr.Namespace, nr.Spec.DatabaseRef, nr.Spec.DatabaseName, nr.Status.DatabaseName)
	if err != nil {
		return err
	}

	nr.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, nr,
		nr.Namespace, nr.Spec.SchemaRef, nr.Spec.SchemaName, nr.Status.SchemaName)
	if err != nil {
		return err
	}

	nr.Status.SchemaName = schemaFQN

	refresolver.SetDatabaseAndSchemaResolvedCondition(nr, nr.Spec.DatabaseRef, nr.Spec.DatabaseName, nr.Spec.SchemaRef, nr.Spec.SchemaName)

	return nil
}

func (a *adapter) BuildIdentifier(nr *snowplanev1alpha1.NetworkRule) reconciler.Identifier {
	dbName := snowflake.ParseDatabaseNameFromFQN(nr.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(nr.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, nr.Spec.Name)
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.NetworkRule{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				nr, ok := o.(*snowplanev1alpha1.NetworkRule)
				if !ok || nr.Spec.DatabaseRef == nil {
					return nil
				}

				return []string{nr.Spec.DatabaseRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
		}

		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.NetworkRule{},
			".spec.schemaRef.name",
			func(o sigs.Object) []string {
				nr, ok := o.(*snowplanev1alpha1.NetworkRule)
				if !ok || nr.Spec.SchemaRef == nil {
					return nil
				}

				return []string{nr.Spec.SchemaRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.NetworkRuleList{} }, ".spec.databaseRef.name", "listing network rules for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.NetworkRuleList{} }, ".spec.schemaRef.name", "listing network rules for schema watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.NetworkRuleObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.NetworkRuleObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.NetworkRule, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)
	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterNetworkRuleOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, nr *snowplanev1alpha1.NetworkRule) error {
	if reconciler.ShouldSkipImmutableValidation(nr) {
		return nil
	}

	if nr.Status.ShowOutput != nil {
		if nr.Status.ShowOutput.Name != "" && !strings.EqualFold(nr.Spec.Name, nr.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", nr.Status.ShowOutput.Name, nr.Spec.Name)
		}

		if nr.Status.ShowOutput.DatabaseName != "" && nr.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(nr.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, nr.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", nr.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if nr.Status.ShowOutput.SchemaName != "" && nr.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(nr.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, nr.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", nr.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}

		if nr.Status.ShowOutput.Type != "" && !strings.EqualFold(string(nr.Spec.Type), nr.Status.ShowOutput.Type) {
			return fmt.Errorf("spec.type is immutable after creation (current: %q, desired: %q)", nr.Status.ShowOutput.Type, nr.Spec.Type)
		}

		if nr.Status.ShowOutput.Mode != "" && !strings.EqualFold(string(nr.Spec.Mode), nr.Status.ShowOutput.Mode) {
			return fmt.Errorf("spec.mode is immutable after creation (current: %q, desired: %q)", nr.Status.ShowOutput.Mode, nr.Spec.Mode)
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.NetworkRule, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.NetworkRuleObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)
	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.NetworkRule, obs *reconciler.Observation[*snowflake.NetworkRuleObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.NetworkRule) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.NetworkRule, obs *reconciler.Observation[*snowflake.NetworkRuleObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) PostCreate(_ *snowplanev1alpha1.NetworkRule)                                    {}
func (a *adapter) PostUpdate(_ *snowplanev1alpha1.NetworkRule, _ bool, _ reconciler.AlterOptions) {}
func (a *adapter) SupportsCreateOrAlter() bool                                                    { return false }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.NetworkRule, Service, *snowflake.NetworkRuleObservation] = (*adapter)(nil)
