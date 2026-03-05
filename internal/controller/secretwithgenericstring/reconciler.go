// Package secretwithgenericstring implements the reconciler for
// SecretWithGenericString resources.
package secretwithgenericstring

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
	finalizerName = "snowplane.hupe1980.github.io/secretwithgenericstring"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake secrets.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error)
	Create(ctx context.Context, opts snowflake.CreateSecretOptions) error
	Alter(ctx context.Context, opts snowflake.AlterSecretOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new SecretWithGenericString reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.SecretWithGenericString, Service, *snowflake.SecretObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewSecretClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.SecretWithGenericString, Service, *snowflake.SecretObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for SecretWithGenericString resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.SecretWithGenericString, Service, *snowflake.SecretObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.SecretWithGenericString, Service, *snowflake.SecretObservation]{
		ResourceNameVal:  "secretwithgenericstring",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.SecretWithGenericString { return &snowplanev1alpha1.SecretWithGenericString{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.SecretWithGenericString) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.SecretObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.SecretWithGenericString, id snowflake.SchemaObjectIdentifier) error {
			return svc.Create(ctx, buildCreateOptions(obj, id))
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterSecretOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.SecretWithGenericString, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.SecretObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.SecretWithGenericString, obs *reconciler.Observation[*snowflake.SecretObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.SecretWithGenericString, obs *reconciler.Observation[*snowflake.SecretObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		PreReconcileFn: func(ctx context.Context, obj *snowplanev1alpha1.SecretWithGenericString) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, obj,
				obj.Namespace, obj.Spec.DatabaseRef, obj.Spec.DatabaseName, obj.Status.DatabaseName)
			if err != nil {
				return err
			}

			obj.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, obj,
				obj.Namespace, obj.Spec.SchemaRef, obj.Spec.SchemaName, obj.Status.SchemaName)
			if err != nil {
				return err
			}

			obj.Status.SchemaName = schemaFQN

			refresolver.SetDatabaseAndSchemaResolvedCondition(obj, obj.Spec.DatabaseRef, obj.Spec.DatabaseName, obj.Spec.SchemaRef, obj.Spec.SchemaName)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(ctx, &snowplanev1alpha1.SecretWithGenericString{}, ".spec.databaseRef.name",
				func(o sigs.Object) []string {
					obj, ok := o.(*snowplanev1alpha1.SecretWithGenericString)
					if !ok || obj.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{obj.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(ctx, &snowplanev1alpha1.SecretWithGenericString{}, ".spec.schemaRef.name",
				func(o sigs.Object) []string {
					obj, ok := o.(*snowplanev1alpha1.SecretWithGenericString)
					if !ok || obj.Spec.SchemaRef == nil {
						return nil
					}

					return []string{obj.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			bldr.Watches(&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.SecretWithGenericStringList{} }, ".spec.databaseRef.name", "listing secrets for database watch")))
			bldr.Watches(&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.SecretWithGenericStringList{} }, ".spec.schemaRef.name", "listing secrets for schema watch")))

			return nil
		},
	}
}

// validateImmutableFields checks that immutable fields have not changed.
func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.SecretWithGenericString) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ShowOutput != nil {
		if obj.Status.ShowOutput.Name != "" && !strings.EqualFold(obj.Spec.Name, obj.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", obj.Status.ShowOutput.Name, obj.Spec.Name)
		}

		if obj.Status.ShowOutput.DatabaseName != "" && obj.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, obj.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", obj.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if obj.Status.ShowOutput.SchemaName != "" && obj.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, obj.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", obj.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func applyObservation(obj *snowplanev1alpha1.SecretWithGenericString, obs *snowflake.SecretObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName, obs.ShowOutput.SchemaName, obs.ShowOutput.Name,
		).FullyQualifiedName()

		obj.Status.ShowOutput = obs.ShowOutput
	}

	if obs.DescribeOutput != nil {
		obj.Status.DescribeOutput = &snowplanev1alpha1.SecretDescribeOutput{
			SecretType:                  stringPtrOrNil(obs.DescribeOutput["secret_type"]),
			Username:                    stringPtrOrNil(obs.DescribeOutput["username"]),
			OAuthAccessTokenExpiryTime:  stringPtrOrNil(obs.DescribeOutput["oauth_access_token_expiry_time"]),
			OAuthRefreshTokenExpiryTime: stringPtrOrNil(obs.DescribeOutput["oauth_refresh_token_expiry_time"]),
			OAuthScopes:                 stringPtrOrNil(obs.DescribeOutput["oauth_scopes"]),
			IntegrationName:             stringPtrOrNil(obs.DescribeOutput["integration_name"]),
			Comment:                     stringPtrOrNil(obs.DescribeOutput["comment"]),
		}
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.SecretWithGenericString, id snowflake.SchemaObjectIdentifier) snowflake.CreateSecretOptions {
	return snowflake.CreateSecretOptions{
		Name:         id,
		SecretType:   snowflake.SecretTypeGenericString,
		SecretString: obj.Spec.SecretString,
		Comment:      obj.Spec.Comment,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.SecretWithGenericString, id snowflake.SchemaObjectIdentifier, obs *snowflake.SecretObservation) snowflake.AlterSecretOptions {
	opts := snowflake.AlterSecretOptions{
		Name:       id,
		SecretType: snowflake.SecretTypeGenericString,
	}
	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	secretString := obj.Spec.SecretString
	opts.SecretString = &secretString

	if obj.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.SecretWithGenericString, obs *snowflake.SecretObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	return d.Result()
}

func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
