// Package schema implements the reconciler for Schema resources.
package schema

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
	finalizerName = "snowplane.hupe1980.github.io/schema"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake schemas.
type Service interface {
	Observe(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error)
	Create(ctx context.Context, opts snowflake.CreateSchemaOptions) error
	Alter(ctx context.Context, opts snowflake.AlterSchemaOptions) error
	Drop(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error
	DropCascade(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
// When useRole is non-empty the factory pins a connection, switches to that
// role, and returns a cleanup function that restores the original role.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Schema reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Schema, Service, *snowflake.SchemaObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewSchemaClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory. This is intended for integration tests that
// inject mock Snowflake services while still going through SetupWithManager.
func NewReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.Schema, Service, *snowflake.SchemaObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for Schema resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.Schema, Service, *snowflake.SchemaObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.Schema, Service, *snowflake.SchemaObservation]{
		ResourceNameVal:  "schema",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.Schema { return &snowplanev1alpha1.Schema{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(schema *snowplanev1alpha1.Schema) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(schema.Status.DatabaseName)
			return snowflake.NewDatabaseObjectIdentifier(dbName, schema.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.SchemaObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.Schema, id snowflake.DatabaseObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterSchemaOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.DatabaseObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		DropCascadeFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.DatabaseObjectIdentifier) error {
			return svc.DropCascade(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.Schema, id snowflake.DatabaseObjectIdentifier, obs *reconciler.Observation[*snowflake.SchemaObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.Schema, obs *reconciler.Observation[*snowflake.SchemaObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.Schema, obs *reconciler.Observation[*snowflake.SchemaObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		SupportsCoA:      true,
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, schema *snowplanev1alpha1.Schema) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, schema,
				schema.Namespace, schema.Spec.DatabaseRef, schema.Spec.DatabaseName, schema.Status.DatabaseName)
			if err != nil {
				return err
			}

			schema.Status.DatabaseName = dbFQN

			refresolver.SetDatabaseResolvedCondition(schema, schema.Spec.DatabaseRef, schema.Spec.DatabaseName, dbFQN)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.Schema{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					sch, ok := o.(*snowplanev1alpha1.Schema)
					if !ok || sch.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{sch.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.SchemaList{} }, ".spec.databaseRef.name", "listing schemas for database watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, schema *snowplanev1alpha1.Schema) error {
	if reconciler.ShouldSkipImmutableValidation(schema) {
		return nil
	}

	if schema.Status.ShowOutput != nil {
		isTransient := schema.Status.ShowOutput.Kind == "TRANSIENT"
		if schema.Spec.Transient != isTransient {
			return fmt.Errorf("spec.transient is immutable after creation (current: %v, desired: %v)", isTransient, schema.Spec.Transient)
		}

		if schema.Status.ShowOutput.Name != "" && !strings.EqualFold(schema.Spec.Name, schema.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", schema.Status.ShowOutput.Name, schema.Spec.Name)
		}

		if schema.Status.ShowOutput.DatabaseName != "" && schema.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(schema.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, schema.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", schema.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

	}

	return nil
}

func applyObservation(schema *snowplanev1alpha1.Schema, obs *snowflake.SchemaObservation) {
	if obs.ShowOutput != nil {
		schema.Status.FullyQualifiedName = snowflake.NewDatabaseObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		schema.Status.DatabaseName = obs.ShowOutput.DatabaseName

		schema.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(schema *snowplanev1alpha1.Schema, id snowflake.DatabaseObjectIdentifier) snowflake.CreateSchemaOptions {
	opts := snowflake.CreateSchemaOptions{
		Name:                       id,
		Comment:                    schema.Spec.Comment,
		DataRetentionTimeInDays:    schema.Spec.DataRetentionTimeInDays,
		MaxDataExtensionTimeInDays: schema.Spec.MaxDataExtensionTimeInDays,
		Transient:                  schema.Spec.Transient,
		ManagedAccess:              schema.Spec.ManagedAccess,
		DefaultDDLCollation:        schema.Spec.DefaultDDLCollation,
		ReplaceInvalidCharacters:   schema.Spec.ReplaceInvalidCharacters,
	}

	if schema.Spec.StorageSerializationPolicy != nil {
		s := string(*schema.Spec.StorageSerializationPolicy)
		opts.StorageSerializationPolicy = &s
	}

	if schema.Spec.LogLevel != nil {
		s := string(*schema.Spec.LogLevel)
		opts.LogLevel = &s
	}

	if schema.Spec.MetricLevel != nil {
		s := string(*schema.Spec.MetricLevel)
		opts.MetricLevel = &s
	}

	if schema.Spec.TraceLevel != nil {
		s := string(*schema.Spec.TraceLevel)
		opts.TraceLevel = &s
	}

	return opts
}

func buildAlterOptions(schema *snowplanev1alpha1.Schema, id snowflake.DatabaseObjectIdentifier, obs *snowflake.SchemaObservation) snowflake.AlterSchemaOptions {
	opts := snowflake.AlterSchemaOptions{Name: id}

	// Detect fields that were previously managed but are now nil → UNSET.
	// This must happen before the obs.Parameters nil guard because UNSET
	// decisions are based on spec vs. TrackedParameters, not observed parameters.
	opts.UnsetFields = tracked.ComputeUnset(&schema.Spec, schema.Status.TrackedParameters)

	if schema.Spec.Comment != nil {
		if obs.ShowOutput == nil || *schema.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = schema.Spec.Comment
		}
	}

	// Diff ManagedAccess: compare desired vs observed.
	if obs.ShowOutput != nil {
		observed := snowflake.IsManagedAccess(obs.ShowOutput)
		if schema.Spec.ManagedAccess != observed {
			opts.SetManagedAccess = &schema.Spec.ManagedAccess
		}
	}

	if obs.Parameters == nil {
		return opts
	}

	p := obs.Parameters

	if schema.Spec.DataRetentionTimeInDays != nil {
		if p.DataRetentionTimeInDays == nil || *schema.Spec.DataRetentionTimeInDays != *p.DataRetentionTimeInDays {
			opts.DataRetentionTimeInDays = schema.Spec.DataRetentionTimeInDays
		}
	}

	if schema.Spec.MaxDataExtensionTimeInDays != nil {
		if p.MaxDataExtensionTimeInDays == nil || *schema.Spec.MaxDataExtensionTimeInDays != *p.MaxDataExtensionTimeInDays {
			opts.MaxDataExtensionTimeInDays = schema.Spec.MaxDataExtensionTimeInDays
		}
	}

	if schema.Spec.DefaultDDLCollation != nil && *schema.Spec.DefaultDDLCollation != p.DefaultDDLCollation {
		opts.DefaultDDLCollation = schema.Spec.DefaultDDLCollation
	}

	if schema.Spec.ReplaceInvalidCharacters != nil {
		if p.ReplaceInvalidCharacters == nil || *schema.Spec.ReplaceInvalidCharacters != *p.ReplaceInvalidCharacters {
			opts.ReplaceInvalidCharacters = schema.Spec.ReplaceInvalidCharacters
		}
	}

	if schema.Spec.StorageSerializationPolicy != nil {
		s := string(*schema.Spec.StorageSerializationPolicy)
		if s != p.StorageSerializationPolicy {
			opts.StorageSerializationPolicy = &s
		}
	}

	if schema.Spec.LogLevel != nil {
		s := string(*schema.Spec.LogLevel)
		if s != p.LogLevel {
			opts.LogLevel = &s
		}
	}

	if schema.Spec.MetricLevel != nil {
		s := string(*schema.Spec.MetricLevel)
		if s != p.MetricLevel {
			opts.MetricLevel = &s
		}
	}

	if schema.Spec.TraceLevel != nil {
		s := string(*schema.Spec.TraceLevel)
		if s != p.TraceLevel {
			opts.TraceLevel = &s
		}
	}

	return opts
}

// detectDrift compares desired spec against the observed state and
// returns a structured drift result.
func detectDrift(schema *snowplanev1alpha1.Schema, obs *snowflake.SchemaObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", schema.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(schema.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)

		isTransient := obs.ShowOutput.Kind == "TRANSIENT"
		d.CompareBoolValue("TRANSIENT", schema.Spec.Transient, isTransient, true)

		// Mutable fields.
		d.CompareString("COMMENT", schema.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareBoolValue("MANAGED_ACCESS", schema.Spec.ManagedAccess, snowflake.IsManagedAccess(obs.ShowOutput), false)
	}

	if obs.Parameters != nil {
		p := obs.Parameters
		d.CompareInt32("DATA_RETENTION_TIME_IN_DAYS", schema.Spec.DataRetentionTimeInDays, p.DataRetentionTimeInDays, false)
		d.CompareInt32("MAX_DATA_EXTENSION_TIME_IN_DAYS", schema.Spec.MaxDataExtensionTimeInDays, p.MaxDataExtensionTimeInDays, false)
		d.CompareString("DEFAULT_DDL_COLLATION", schema.Spec.DefaultDDLCollation, p.DefaultDDLCollation, false)
		d.CompareBool("REPLACE_INVALID_CHARACTERS", schema.Spec.ReplaceInvalidCharacters, p.ReplaceInvalidCharacters, false)
		d.CompareStringFold("STORAGE_SERIALIZATION_POLICY", drift.PtrStringFrom(schema.Spec.StorageSerializationPolicy), p.StorageSerializationPolicy, false)
		d.CompareStringFold("LOG_LEVEL", drift.PtrStringFrom(schema.Spec.LogLevel), p.LogLevel, false)
		d.CompareStringFold("METRIC_LEVEL", drift.PtrStringFrom(schema.Spec.MetricLevel), p.MetricLevel, false)
		d.CompareStringFold("TRACE_LEVEL", drift.PtrStringFrom(schema.Spec.TraceLevel), p.TraceLevel, false)
	}

	return d.Result()
}
