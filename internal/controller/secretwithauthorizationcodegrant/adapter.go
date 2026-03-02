package secretwithauthorizationcodegrant

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

// adapter implements reconciler.ResourceAdapter for SecretWithAuthorizationCodeGrant.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "secretwithauthorizationcodegrant" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.SecretWithAuthorizationCodeGrant {
	return &snowplanev1alpha1.SecretWithAuthorizationCodeGrant{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, obj *snowplanev1alpha1.SecretWithAuthorizationCodeGrant) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, obj,
		obj.Namespace, obj.Spec.DatabaseRef, obj.Spec.DatabaseName, obj.Status.DatabaseName)
	if err != nil {
		return err
	}

	obj.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, obj,
		obj.Namespace, obj.Spec.SchemaRef, obj.Spec.SchemaName, obj.Status.SchemaName)
	if err != nil {
		return err
	}

	obj.Status.SchemaName = schemaFQN

	refresolver.SetDatabaseAndSchemaResolvedCondition(obj, obj.Spec.DatabaseRef, obj.Spec.DatabaseName, obj.Spec.SchemaRef, obj.Spec.SchemaName)

	return nil
}

func (a *adapter) BuildIdentifier(obj *snowplanev1alpha1.SecretWithAuthorizationCodeGrant) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, obj.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.SecretWithAuthorizationCodeGrant{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				obj, ok := o.(*snowplanev1alpha1.SecretWithAuthorizationCodeGrant)
				if !ok || obj.Spec.DatabaseRef == nil {
					return nil
				}

				return []string{obj.Spec.DatabaseRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
		}

		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.SecretWithAuthorizationCodeGrant{},
			".spec.schemaRef.name",
			func(o sigs.Object) []string {
				obj, ok := o.(*snowplanev1alpha1.SecretWithAuthorizationCodeGrant)
				if !ok || obj.Spec.SchemaRef == nil {
					return nil
				}

				return []string{obj.Spec.SchemaRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.SecretWithAuthorizationCodeGrantList{} }, ".spec.databaseRef.name", "listing secrets for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.SecretWithAuthorizationCodeGrantList{} }, ".spec.schemaRef.name", "listing secrets for schema watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.SecretObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.SecretObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.SecretWithAuthorizationCodeGrant, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterSecretOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, obj *snowplanev1alpha1.SecretWithAuthorizationCodeGrant) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ShowOutput != nil {
		if obj.Status.ShowOutput.Name != nil && *obj.Status.ShowOutput.Name != "" && !strings.EqualFold(obj.Spec.Name, *obj.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", *obj.Status.ShowOutput.Name, obj.Spec.Name)
		}

		if obj.Status.ShowOutput.DatabaseName != nil && obj.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, *obj.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", *obj.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if obj.Status.ShowOutput.SchemaName != nil && obj.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, *obj.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", *obj.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.SecretWithAuthorizationCodeGrant, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.SecretObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	opts := buildAlterOptions(obj, sid, obs.Detail)

	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.SecretWithAuthorizationCodeGrant, obs *reconciler.Observation[*snowflake.SecretObservation]) {
	applyObservation(obj, obs.Detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.SecretWithAuthorizationCodeGrant) []string {
	return tracked.ComputeTracked(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.SecretWithAuthorizationCodeGrant, obs *reconciler.Observation[*snowflake.SecretObservation]) *drift.Result {
	return detectDrift(obj, obs.Detail)
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.SecretWithAuthorizationCodeGrant, Service, *snowflake.SecretObservation] = (*adapter)(nil)
