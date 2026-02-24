// Package schema implements the reconciler for Schema resources.
package schema

import (
	"context"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
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
}

// ServiceFactory creates a Service from a Snowflake client.
// When useRole is non-empty the factory pins a connection, switches to that
// role, and returns a cleanup function that restores the original role.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Schema reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Schema, Service, *snowflake.SchemaObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Schema, Service, *snowflake.SchemaObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory. This is intended for integration tests that
// inject mock Snowflake services while still going through SetupWithManager.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.Schema, Service, *snowflake.SchemaObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Schema, Service, *snowflake.SchemaObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// defaultServiceFactory is the production ServiceFactory used by NewReconciler.
func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
	if err != nil {
		return nil, nil, err
	}

	return snowflake.NewSchemaClient(sfC), cleanup, nil
}

func applyObservation(schema *snowplanev1alpha1.Schema, obs *snowflake.SchemaObservation) {
	if obs.ShowOutput != nil {
		schema.Status.FullyQualifiedName = snowflake.NewDatabaseObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		schema.Status.DatabaseName = obs.ShowOutput.DatabaseName

		schema.Status.ShowOutput = &snowplanev1alpha1.SchemaShowOutput{
			CreatedOn:     obs.ShowOutput.CreatedOn,
			Name:          obs.ShowOutput.Name,
			DatabaseName:  obs.ShowOutput.DatabaseName,
			Kind:          obs.ShowOutput.Kind,
			Comment:       obs.ShowOutput.Comment,
			Owner:         obs.ShowOutput.Owner,
			RetentionTime: obs.ShowOutput.RetentionTime,
			Options:       obs.ShowOutput.Options,
		}
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
	opts.UnsetFields = computeUnsetFields(schema)

	if schema.Spec.Comment != nil {
		if obs.ShowOutput == nil || *schema.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = schema.Spec.Comment
		}
	}

	// Diff ManagedAccess: compare desired vs observed.
	if obs.ShowOutput != nil {
		observed := obs.ShowOutput.IsManagedAccess()
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

// computeUnsetFields returns the Snowflake parameter names that were
// previously SET (tracked in status.TrackedParameters) but are now nil in the spec.
func computeUnsetFields(schema *snowplanev1alpha1.Schema) []string {
	if len(schema.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(schema.Status.TrackedParameters))
	for _, f := range schema.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if schema.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	if schema.Spec.DataRetentionTimeInDays == nil && managed["DATA_RETENTION_TIME_IN_DAYS"] {
		unset = append(unset, "DATA_RETENTION_TIME_IN_DAYS")
	}

	if schema.Spec.MaxDataExtensionTimeInDays == nil && managed["MAX_DATA_EXTENSION_TIME_IN_DAYS"] {
		unset = append(unset, "MAX_DATA_EXTENSION_TIME_IN_DAYS")
	}

	if schema.Spec.DefaultDDLCollation == nil && managed["DEFAULT_DDL_COLLATION"] {
		unset = append(unset, "DEFAULT_DDL_COLLATION")
	}

	if schema.Spec.ReplaceInvalidCharacters == nil && managed["REPLACE_INVALID_CHARACTERS"] {
		unset = append(unset, "REPLACE_INVALID_CHARACTERS")
	}

	if schema.Spec.StorageSerializationPolicy == nil && managed["STORAGE_SERIALIZATION_POLICY"] {
		unset = append(unset, "STORAGE_SERIALIZATION_POLICY")
	}

	if schema.Spec.LogLevel == nil && managed["LOG_LEVEL"] {
		unset = append(unset, "LOG_LEVEL")
	}

	if schema.Spec.MetricLevel == nil && managed["METRIC_LEVEL"] {
		unset = append(unset, "METRIC_LEVEL")
	}

	if schema.Spec.TraceLevel == nil && managed["TRACE_LEVEL"] {
		unset = append(unset, "TRACE_LEVEL")
	}

	return unset
}

// computeTrackedParameters returns the Snowflake parameter names that are
// actively managed (non-nil) in the schema spec.
func computeTrackedParameters(spec *snowplanev1alpha1.SchemaSpec) []string {
	var fields []string

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	if spec.DataRetentionTimeInDays != nil {
		fields = append(fields, "DATA_RETENTION_TIME_IN_DAYS")
	}

	if spec.MaxDataExtensionTimeInDays != nil {
		fields = append(fields, "MAX_DATA_EXTENSION_TIME_IN_DAYS")
	}

	if spec.DefaultDDLCollation != nil {
		fields = append(fields, "DEFAULT_DDL_COLLATION")
	}

	if spec.ReplaceInvalidCharacters != nil {
		fields = append(fields, "REPLACE_INVALID_CHARACTERS")
	}

	if spec.StorageSerializationPolicy != nil {
		fields = append(fields, "STORAGE_SERIALIZATION_POLICY")
	}

	if spec.LogLevel != nil {
		fields = append(fields, "LOG_LEVEL")
	}

	if spec.MetricLevel != nil {
		fields = append(fields, "METRIC_LEVEL")
	}

	if spec.TraceLevel != nil {
		fields = append(fields, "TRACE_LEVEL")
	}

	return fields
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
		d.CompareBoolValue("MANAGED_ACCESS", schema.Spec.ManagedAccess, obs.ShowOutput.IsManagedAccess(), false)
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
