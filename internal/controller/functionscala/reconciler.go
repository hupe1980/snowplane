// Package functionscala implements the reconciler for FunctionScala resources.
package functionscala

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
	finalizerName = "snowplane.hupe1980.github.io/functionscala"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake functions.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) (*snowflake.FunctionObservation, error)
	Create(ctx context.Context, opts snowflake.CreateFunctionOptions) error
	Alter(ctx context.Context, opts snowflake.AlterFunctionOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new FunctionScala reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.FunctionScala, Service, *snowflake.FunctionObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewFunctionClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.FunctionScala, Service, *snowflake.FunctionObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for FunctionScala resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.FunctionScala, Service, *snowflake.FunctionObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.FunctionScala, Service, *snowflake.FunctionObservation]{
		ResourceNameVal:  "functionscala",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.FunctionScala { return &snowplanev1alpha1.FunctionScala{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.FunctionScala) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName)

			argTypes := make([]string, len(obj.Spec.Arguments))
			for i, arg := range obj.Spec.Arguments {
				argTypes[i] = arg.Type
			}

			return snowflake.NewCallableIdentifier(dbName, schemaName, obj.Spec.Name, argTypes), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.CallableIdentifier) (*snowflake.FunctionObservation, error) {
				return svc.Observe(ctx, id.SchemaObjectIdentifier, id.ArgTypes())
			},
			func(obs *snowflake.FunctionObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.FunctionScala, id snowflake.CallableIdentifier) error {
			opts := buildCreateOptions(obj, id)
			opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterFunctionOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.CallableIdentifier) error {
			return svc.Drop(ctx, id.SchemaObjectIdentifier, id.ArgTypes())
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.FunctionScala, id snowflake.CallableIdentifier, obs *reconciler.Observation[*snowflake.FunctionObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.FunctionScala, obs *reconciler.Observation[*snowflake.FunctionObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.FunctionScala, obs *reconciler.Observation[*snowflake.FunctionObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		SupportsCoA: true,
		PreReconcileFn: func(ctx context.Context, obj *snowplanev1alpha1.FunctionScala) error {
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
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.FunctionScala{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					obj, ok := o.(*snowplanev1alpha1.FunctionScala)
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
				&snowplanev1alpha1.FunctionScala{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					obj, ok := o.(*snowplanev1alpha1.FunctionScala)
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.FunctionScalaList{} }, ".spec.databaseRef.name", "listing functionscalas for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.FunctionScalaList{} }, ".spec.schemaRef.name", "listing functionscalas for schema watch")),
			)

			return nil
		},
	}
}

// validateImmutableFields checks that immutable fields have not changed.
func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.FunctionScala) error {
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

func applyObservation(obj *snowplanev1alpha1.FunctionScala, obs *snowflake.FunctionObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		obj.Status.DatabaseName = obs.ShowOutput.DatabaseName
		obj.Status.SchemaName = obs.ShowOutput.SchemaName

		obj.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.FunctionScala, id snowflake.CallableIdentifier) snowflake.CreateFunctionOptions {
	args := make([]snowflake.FunctionArgument, len(obj.Spec.Arguments))
	for i, a := range obj.Spec.Arguments {
		args[i] = snowflake.FunctionArgument{Name: a.Name, Type: a.Type}
	}

	handler := obj.Spec.Handler
	runtimeVersion := obj.Spec.RuntimeVersion

	// Snowpark package goes as the first item in Packages.
	packages := make([]string, 0, 1+len(obj.Spec.Packages))
	packages = append(packages, obj.Spec.SnowparkPackage)
	packages = append(packages, obj.Spec.Packages...)

	// Map SecretBinding slice to map[string]string.
	var secrets map[string]string
	if len(obj.Spec.Secrets) > 0 {
		secrets = make(map[string]string, len(obj.Spec.Secrets))
		for _, s := range obj.Spec.Secrets {
			secrets[s.VariableName] = s.SecretName
		}
	}

	opts := snowflake.CreateFunctionOptions{
		Name:                       id.SchemaObjectIdentifier,
		Arguments:                  args,
		Returns:                    obj.Spec.Returns,
		Language:                   "SCALA",
		Body:                       obj.Spec.Body,
		Handler:                    &handler,
		RuntimeVersion:             &runtimeVersion,
		Packages:                   packages,
		Imports:                    obj.Spec.Imports,
		TargetPath:                 obj.Spec.TargetPath,
		ExternalAccessIntegrations: obj.Spec.ExternalAccessIntegrations,
		Secrets:                    secrets,
		Volatility:                 obj.Spec.Volatility,
		NullInputBehavior:          obj.Spec.NullInputBehavior,
		Secure:                     obj.Spec.Secure,
		Comment:                    obj.Spec.Comment,
	}

	return opts
}

func buildAlterOptions(obj *snowplanev1alpha1.FunctionScala, id snowflake.CallableIdentifier, obs *snowflake.FunctionObservation) snowflake.AlterFunctionOptions {
	opts := snowflake.AlterFunctionOptions{
		Name:     id.SchemaObjectIdentifier,
		ArgTypes: id.ArgTypes(),
	}
	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	// Secure: detect toggle against observed state.
	if obs != nil && obs.ShowOutput != nil {
		if obj.Spec.Secure != obs.ShowOutput.IsSecure {
			secure := obj.Spec.Secure
			opts.Secure = &secure
		}
	}

	if obj.Spec.Comment != nil {
		if obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Description {
			opts.Comment = obj.Spec.Comment
		}
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.FunctionScala, obs *snowflake.FunctionObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Description, false)
		d.CompareBoolValue("IS_SECURE", obj.Spec.Secure, obs.ShowOutput.IsSecure, false)
	}

	return d.Result()
}
