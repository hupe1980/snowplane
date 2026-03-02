package authenticationpolicy

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

// adapter implements reconciler.ResourceAdapter for AuthenticationPolicy.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "authenticationpolicy" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.AuthenticationPolicy {
	return &snowplanev1alpha1.AuthenticationPolicy{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, ap *snowplanev1alpha1.AuthenticationPolicy) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, ap,
		ap.Namespace, ap.Spec.DatabaseRef, ap.Spec.DatabaseName, ap.Status.DatabaseName)
	if err != nil {
		return err
	}

	ap.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, ap,
		ap.Namespace, ap.Spec.SchemaRef, ap.Spec.SchemaName, ap.Status.SchemaName)
	if err != nil {
		return err
	}

	ap.Status.SchemaName = schemaFQN

	refresolver.SetDatabaseAndSchemaResolvedCondition(ap, ap.Spec.DatabaseRef, ap.Spec.DatabaseName, ap.Spec.SchemaRef, ap.Spec.SchemaName)

	return nil
}

func (a *adapter) BuildIdentifier(ap *snowplanev1alpha1.AuthenticationPolicy) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(ap.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(ap.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, ap.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.AuthenticationPolicy{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				ap, ok := o.(*snowplanev1alpha1.AuthenticationPolicy)
				if !ok || ap.Spec.DatabaseRef == nil {
					return nil
				}

				return []string{ap.Spec.DatabaseRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
		}

		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.AuthenticationPolicy{},
			".spec.schemaRef.name",
			func(o sigs.Object) []string {
				ap, ok := o.(*snowplanev1alpha1.AuthenticationPolicy)
				if !ok || ap.Spec.SchemaRef == nil {
					return nil
				}

				return []string{ap.Spec.SchemaRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.AuthenticationPolicyList{} }, ".spec.databaseRef.name", "listing authentication policies for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.AuthenticationPolicyList{} }, ".spec.schemaRef.name", "listing authentication policies for schema watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.AuthenticationPolicyObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.AuthenticationPolicyObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.AuthenticationPolicy, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)
	opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterAuthenticationPolicyOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, ap *snowplanev1alpha1.AuthenticationPolicy) error {
	if reconciler.ShouldSkipImmutableValidation(ap) {
		return nil
	}

	if ap.Status.ShowOutput != nil {
		if ap.Status.ShowOutput.Name != "" && !strings.EqualFold(ap.Spec.Name, ap.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", ap.Status.ShowOutput.Name, ap.Spec.Name)
		}

		if ap.Status.ShowOutput.DatabaseName != "" && ap.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(ap.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, ap.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", ap.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if ap.Status.ShowOutput.SchemaName != "" && ap.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(ap.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, ap.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", ap.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.AuthenticationPolicy, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.AuthenticationPolicyObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)

	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.AuthenticationPolicy, obs *reconciler.Observation[*snowflake.AuthenticationPolicyObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.AuthenticationPolicy) []string {
	return tracked.ComputeTracked(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.AuthenticationPolicy, obs *reconciler.Observation[*snowflake.AuthenticationPolicyObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) SupportsCreateOrAlter() bool { return true }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.AuthenticationPolicy, Service, *snowflake.AuthenticationPolicyObservation] = (*adapter)(nil)
