// Package externalfunction implements the reconciler for ExternalFunction resources.
package externalfunction

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
	finalizerName = "snowplane.hupe1980.github.io/externalfunction"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake external functions.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ExternalFunctionObservation, error)
	Create(ctx context.Context, opts snowflake.CreateExternalFunctionOptions) error
	Alter(ctx context.Context, opts snowflake.AlterExternalFunctionOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new ExternalFunction reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.ExternalFunction, Service, *snowflake.ExternalFunctionObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewExternalFunctionClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.ExternalFunction, Service, *snowflake.ExternalFunctionObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for ExternalFunction resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.ExternalFunction, Service, *snowflake.ExternalFunctionObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.ExternalFunction, Service, *snowflake.ExternalFunctionObservation]{
		ResourceNameVal:  "externalfunction",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.ExternalFunction { return &snowplanev1alpha1.ExternalFunction{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.ExternalFunction) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName)

			argTypes := make([]string, len(obj.Spec.Args))
			for i, a := range obj.Spec.Args {
				argTypes[i] = a.Type
			}

			return snowflake.NewCallableIdentifier(dbName, schemaName, obj.Spec.Name, argTypes), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.CallableIdentifier) (*snowflake.ExternalFunctionObservation, error) {
				return svc.Observe(ctx, id.SchemaObjectIdentifier)
			},
			func(obs *snowflake.ExternalFunctionObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.ExternalFunction, id snowflake.CallableIdentifier) error {
			return svc.Create(ctx, buildCreateOptions(obj, id.SchemaObjectIdentifier))
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterExternalFunctionOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.CallableIdentifier) error {
			return svc.Drop(ctx, id.SchemaObjectIdentifier, id.ArgTypes())
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.ExternalFunction, id snowflake.CallableIdentifier, obs *reconciler.Observation[*snowflake.ExternalFunctionObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id.SchemaObjectIdentifier, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.ExternalFunction, obs *reconciler.Observation[*snowflake.ExternalFunctionObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.ExternalFunction, obs *reconciler.Observation[*snowflake.ExternalFunctionObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, obj *snowplanev1alpha1.ExternalFunction) error {
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

			// Resolve optional API integration ref.
			if obj.Spec.APIIntegrationRef != nil || obj.Spec.APIIntegrationName != nil {
				aiName, aiErr := refresolver.PreReconcileSourceRef(ctx, c, recorder, obj,
					obj.Namespace, obj.Spec.APIIntegrationRef, obj.Spec.APIIntegrationName, obj.Status.APIIntegrationName,
					"APIIntegration",
					func() *snowplanev1alpha1.APIIntegration {
						return &snowplanev1alpha1.APIIntegration{}
					},
					snowplanev1alpha1.GroupVersion.WithKind("APIIntegration"),
					func(ai *snowplanev1alpha1.APIIntegration) string { return ai.Spec.Name },
				)
				if aiErr != nil {
					return aiErr
				}

				obj.Status.APIIntegrationName = aiName
			}

			refs := []refresolver.RefDescriptor{
				{KindLabel: "Database", Ref: obj.Spec.DatabaseRef, RawName: obj.Spec.DatabaseName},
				{KindLabel: "Schema", Ref: obj.Spec.SchemaRef, RawName: obj.Spec.SchemaName},
			}

			if obj.Spec.APIIntegrationRef != nil || obj.Spec.APIIntegrationName != nil {
				refs = append(refs, refresolver.RefDescriptor{KindLabel: "APIIntegration", Ref: obj.Spec.APIIntegrationRef, RawName: obj.Spec.APIIntegrationName})
			}

			refresolver.SetAllReferencesResolvedCondition(obj, refs...)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.ExternalFunction{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					ef, ok := o.(*snowplanev1alpha1.ExternalFunction)
					if !ok || ef.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{ef.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.ExternalFunction{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					ef, ok := o.(*snowplanev1alpha1.ExternalFunction)
					if !ok || ef.Spec.SchemaRef == nil {
						return nil
					}

					return []string{ef.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.ExternalFunctionList{} }, ".spec.databaseRef.name", "listing external functions for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.ExternalFunctionList{} }, ".spec.schemaRef.name", "listing external functions for schema watch")),
			)

			// APIIntegrationRef index.
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.ExternalFunction{},
				".spec.apiIntegrationRef.name",
				func(o sigs.Object) []string {
					ef, ok := o.(*snowplanev1alpha1.ExternalFunction)
					if !ok || ef.Spec.APIIntegrationRef == nil {
						return nil
					}

					return []string{ef.Spec.APIIntegrationRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.apiIntegrationRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.APIIntegration{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.ExternalFunctionList{} }, ".spec.apiIntegrationRef.name", "listing external functions for API integration watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, ef *snowplanev1alpha1.ExternalFunction) error {
	if reconciler.ShouldSkipImmutableValidation(ef) {
		return nil
	}

	if ef.Status.ShowOutput != nil {
		if ef.Status.ShowOutput.Name != "" && !strings.EqualFold(ef.Spec.Name, ef.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", ef.Status.ShowOutput.Name, ef.Spec.Name)
		}

		if ef.Status.ShowOutput.DatabaseName != "" && ef.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(ef.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, ef.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", ef.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if ef.Status.ShowOutput.SchemaName != "" && ef.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(ef.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, ef.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", ef.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}

		// URL is immutable — embedded in AS clause of CREATE EXTERNAL FUNCTION.
		if ef.Status.URL != "" && ef.Spec.URL != ef.Status.URL {
			return fmt.Errorf("spec.url is immutable after creation (current: %q, desired: %q)", ef.Status.URL, ef.Spec.URL)
		}
	}

	return nil
}

func applyObservation(ef *snowplanev1alpha1.ExternalFunction, obs *snowflake.ExternalFunctionObservation) {
	if obs.ShowOutput != nil {
		ef.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		ef.Status.DatabaseName = obs.ShowOutput.DatabaseName
		ef.Status.SchemaName = obs.ShowOutput.SchemaName
		ef.Status.ShowOutput = obs.ShowOutput

		// Cache URL on first observation for immutable field validation.
		// SHOW EXTERNAL FUNCTIONS does not return the URL, so we cache it from spec.
		if ef.Status.URL == "" && ef.Spec.URL != "" {
			ef.Status.URL = ef.Spec.URL
		}
	}
}

func buildCreateOptions(ef *snowplanev1alpha1.ExternalFunction, id snowflake.SchemaObjectIdentifier) snowflake.CreateExternalFunctionOptions {
	// Prefer resolved status name (from ref resolution), fall back to spec.
	apiIntegration := ef.Status.APIIntegrationName
	if apiIntegration == "" && ef.Spec.APIIntegrationName != nil {
		apiIntegration = *ef.Spec.APIIntegrationName
	}

	return snowflake.CreateExternalFunctionOptions{
		Name:               id,
		Args:               ef.Spec.Args,
		ReturnType:         ef.Spec.ReturnType,
		ReturnNullValues:   ef.Spec.ReturnNullValues,
		ReturnBehavior:     ef.Spec.ReturnBehavior,
		APIIntegration:     apiIntegration,
		URL:                ef.Spec.URL,
		Headers:            ef.Spec.Headers,
		MaxBatchRows:       ef.Spec.MaxBatchRows,
		Compression:        ef.Spec.Compression,
		RequestTranslator:  ef.Spec.RequestTranslator,
		ResponseTranslator: ef.Spec.ResponseTranslator,
		Comment:            ef.Spec.Comment,
	}
}

func buildAlterOptions(ef *snowplanev1alpha1.ExternalFunction, id snowflake.SchemaObjectIdentifier, obs *snowflake.ExternalFunctionObservation) snowflake.AlterExternalFunctionOptions {
	opts := snowflake.AlterExternalFunctionOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&ef.Spec, ef.Status.TrackedParameters)

	// Build arg type list for ALTER signature.
	argTypes := make([]string, len(ef.Spec.Args))
	for i, a := range ef.Spec.Args {
		argTypes[i] = a.Type
	}

	opts.ArgTypes = argTypes

	if ef.Spec.Comment != nil {
		if obs.ShowOutput == nil || *ef.Spec.Comment != obs.ShowOutput.Description {
			opts.Comment = ef.Spec.Comment
		}
	}

	return opts
}

func detectDrift(ef *snowplanev1alpha1.ExternalFunction, obs *snowflake.ExternalFunctionObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", ef.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(ef.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(ef.Status.SchemaName), obs.ShowOutput.SchemaName, true)
		d.CompareString("COMMENT", ef.Spec.Comment, obs.ShowOutput.Description, false)
	}

	return d.Result()
}
