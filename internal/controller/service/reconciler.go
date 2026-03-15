// Package service implements the reconciler for Service (SPCS) resources.
package service

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
	finalizerName = "snowplane.hupe1980.github.io/service"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// SnowflakeService defines operations the reconciler needs against Snowflake services.
// Named SnowflakeService to avoid collision with the CRD type Service.
type SnowflakeService interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ServiceObservation, error)
	Create(ctx context.Context, opts snowflake.CreateServiceOptions) error
	Alter(ctx context.Context, opts snowflake.AlterServiceOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a SnowflakeService from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (SnowflakeService, func(context.Context), error)

// NewReconciler returns a new Service reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Service, SnowflakeService, *snowflake.ServiceObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) SnowflakeService {
			return snowflake.NewServiceClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.Service, SnowflakeService, *snowflake.ServiceObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for Service resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.Service, SnowflakeService, *snowflake.ServiceObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.Service, SnowflakeService, *snowflake.ServiceObservation]{
		ResourceNameVal:  "service",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.Service { return &snowplanev1alpha1.Service{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.Service) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc SnowflakeService, id snowflake.SchemaObjectIdentifier) (*snowflake.ServiceObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.ServiceObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc SnowflakeService, obj *snowplanev1alpha1.Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Create(ctx, buildCreateOptions(obj, id))
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc SnowflakeService, opts *snowflake.AlterServiceOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc SnowflakeService, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.Service, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.ServiceObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.Service, obs *reconciler.Observation[*snowflake.ServiceObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.Service, obs *reconciler.Observation[*snowflake.ServiceObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, obj *snowplanev1alpha1.Service) error {
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

			// Resolve optional compute pool ref.
			if obj.Spec.ComputePoolRef != nil || obj.Spec.ComputePoolName != nil {
				cpName, cpErr := refresolver.PreReconcileSourceRef(ctx, c, recorder, obj,
					obj.Namespace, obj.Spec.ComputePoolRef, obj.Spec.ComputePoolName, obj.Status.ComputePoolName,
					"ComputePool",
					func() *snowplanev1alpha1.ComputePool {
						return &snowplanev1alpha1.ComputePool{}
					},
					snowplanev1alpha1.GroupVersion.WithKind("ComputePool"),
					func(cp *snowplanev1alpha1.ComputePool) string { return cp.Spec.Name },
				)
				if cpErr != nil {
					return cpErr
				}

				obj.Status.ComputePoolName = cpName
			}

			refs := []refresolver.RefDescriptor{
				{KindLabel: "Database", Ref: obj.Spec.DatabaseRef, RawName: obj.Spec.DatabaseName},
				{KindLabel: "Schema", Ref: obj.Spec.SchemaRef, RawName: obj.Spec.SchemaName},
			}

			if obj.Spec.ComputePoolRef != nil || obj.Spec.ComputePoolName != nil {
				refs = append(refs, refresolver.RefDescriptor{KindLabel: "ComputePool", Ref: obj.Spec.ComputePoolRef, RawName: obj.Spec.ComputePoolName})
			}

			refresolver.SetAllReferencesResolvedCondition(obj, refs...)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.Service{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.Service)
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
				&snowplanev1alpha1.Service{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.Service)
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.ServiceList{} }, ".spec.databaseRef.name", "listing services for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.ServiceList{} }, ".spec.schemaRef.name", "listing services for schema watch")),
			)

			// ComputePoolRef index.
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.Service{},
				".spec.computePoolRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.Service)
					if !ok || s.Spec.ComputePoolRef == nil {
						return nil
					}

					return []string{s.Spec.ComputePoolRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.computePoolRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.ComputePool{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.ServiceList{} }, ".spec.computePoolRef.name", "listing services for compute pool watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, svc *snowplanev1alpha1.Service) error {
	if reconciler.ShouldSkipImmutableValidation(svc) {
		return nil
	}

	if svc.Status.ShowOutput != nil {
		if svc.Status.ShowOutput.Name != "" && !strings.EqualFold(svc.Spec.Name, svc.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", svc.Status.ShowOutput.Name, svc.Spec.Name)
		}

		if svc.Status.ShowOutput.DatabaseName != "" && svc.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(svc.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, svc.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", svc.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if svc.Status.ShowOutput.SchemaName != "" && svc.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(svc.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, svc.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", svc.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}

		// ComputePool is immutable — embedded in CREATE SERVICE statement.
		if svc.Status.ShowOutput.ComputePool != "" && svc.Status.ComputePoolName != "" {
			if !strings.EqualFold(svc.Status.ComputePoolName, svc.Status.ShowOutput.ComputePool) {
				return fmt.Errorf("spec.computePool is immutable after creation (current: %q, resolved: %q)", svc.Status.ShowOutput.ComputePool, svc.Status.ComputePoolName)
			}
		}
	}

	return nil
}

func applyObservation(svc *snowplanev1alpha1.Service, obs *snowflake.ServiceObservation) {
	if obs.ShowOutput != nil {
		svc.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		svc.Status.DatabaseName = obs.ShowOutput.DatabaseName
		svc.Status.SchemaName = obs.ShowOutput.SchemaName
		svc.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(svc *snowplanev1alpha1.Service, id snowflake.SchemaObjectIdentifier) snowflake.CreateServiceOptions {
	// Prefer resolved status name (from ref resolution), fall back to spec.
	computePool := svc.Status.ComputePoolName
	if computePool == "" && svc.Spec.ComputePoolName != nil {
		computePool = *svc.Spec.ComputePoolName
	}

	var specPtr, specRefPtr *string

	if svc.Spec.Specification != "" {
		s := svc.Spec.Specification
		specPtr = &s
	}

	if svc.Spec.SpecificationReference != "" {
		s := svc.Spec.SpecificationReference
		specRefPtr = &s
	}

	return snowflake.CreateServiceOptions{
		Name:                       id,
		ComputePool:                computePool,
		Specification:              specPtr,
		SpecificationReference:     specRefPtr,
		MinInstances:               svc.Spec.MinInstances,
		MaxInstances:               svc.Spec.MaxInstances,
		AutoResume:                 svc.Spec.AutoResume,
		ExternalAccessIntegrations: svc.Spec.ExternalAccessIntegrations,
		Comment:                    svc.Spec.Comment,
	}
}

func buildAlterOptions(svc *snowplanev1alpha1.Service, id snowflake.SchemaObjectIdentifier, obs *snowflake.ServiceObservation) snowflake.AlterServiceOptions {
	opts := snowflake.AlterServiceOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&svc.Spec, svc.Status.TrackedParameters)

	if obs.ShowOutput != nil {
		if svc.Spec.MinInstances != nil && *svc.Spec.MinInstances != obs.ShowOutput.MinInstances {
			opts.MinInstances = svc.Spec.MinInstances
		}

		if svc.Spec.MaxInstances != nil && *svc.Spec.MaxInstances != obs.ShowOutput.MaxInstances {
			opts.MaxInstances = svc.Spec.MaxInstances
		}

		if svc.Spec.AutoResume != nil {
			current := strings.EqualFold(obs.ShowOutput.AutoResume, "true")
			if *svc.Spec.AutoResume != current {
				opts.AutoResume = svc.Spec.AutoResume
			}
		}

		if svc.Spec.Comment != nil && *svc.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = svc.Spec.Comment
		}

		// ExternalAccessIntegrations: always send when set since SHOW SERVICES
		// does not expose EAI values for comparison.
		if len(svc.Spec.ExternalAccessIntegrations) > 0 {
			opts.ExternalAccessIntegrations = svc.Spec.ExternalAccessIntegrations
		}
	}

	return opts
}

func detectDrift(svc *snowplanev1alpha1.Service, obs *snowflake.ServiceObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", svc.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(svc.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(svc.Status.SchemaName), obs.ShowOutput.SchemaName, true)
		d.CompareStringValueFold("COMPUTE_POOL", svc.Status.ComputePoolName, obs.ShowOutput.ComputePool, true)
		d.CompareInt32Value("MIN_INSTANCES", svc.Spec.MinInstances, obs.ShowOutput.MinInstances, false)
		d.CompareInt32Value("MAX_INSTANCES", svc.Spec.MaxInstances, obs.ShowOutput.MaxInstances, false)

		if svc.Spec.AutoResume != nil {
			obsResume := strings.EqualFold(obs.ShowOutput.AutoResume, "true")
			d.CompareBool("AUTO_RESUME", svc.Spec.AutoResume, &obsResume, false)
		}

		d.CompareString("COMMENT", svc.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	return d.Result()
}
