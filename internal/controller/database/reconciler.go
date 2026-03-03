// Package database implements the reconciler for Database resources.
package database

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
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/database"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake databases.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error)
	Create(ctx context.Context, opts snowflake.CreateDatabaseOptions) error
	Alter(ctx context.Context, opts snowflake.AlterDatabaseOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
	DropCascade(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
// When useRole is non-empty the factory pins a connection, switches to that
// role, and returns a cleanup function that restores the original role.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Database reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Database, Service, *snowflake.DatabaseObservation] {
	a := &adapter{newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Database, Service, *snowflake.DatabaseObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.Database, Service, *snowflake.DatabaseObservation] {
	a := &adapter{newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Database, Service, *snowflake.DatabaseObservation]{
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

	return snowflake.NewDatabaseClient(sfC), cleanup, nil
}

func applyObservation(db *snowplanev1alpha1.Database, obs *snowflake.DatabaseObservation) {
	if obs.ShowOutput != nil {
		db.Status.FullyQualifiedName = snowflake.NewAccountObjectIdentifier(obs.ShowOutput.Name).FullyQualifiedName()

		db.Status.ShowOutput = &snowplanev1alpha1.DatabaseShowOutput{
			CreatedOn:     obs.ShowOutput.CreatedOn,
			Name:          obs.ShowOutput.Name,
			Kind:          obs.ShowOutput.Kind,
			Comment:       obs.ShowOutput.Comment,
			Owner:         obs.ShowOutput.Owner,
			RetentionTime: obs.ShowOutput.RetentionTime,
		}
	}
}

func buildCreateOptions(db *snowplanev1alpha1.Database, id snowflake.AccountObjectIdentifier) snowflake.CreateDatabaseOptions {
	opts := snowflake.CreateDatabaseOptions{
		Name:                       id,
		Comment:                    db.Spec.Comment,
		DataRetentionTimeInDays:    db.Spec.DataRetentionTimeInDays,
		MaxDataExtensionTimeInDays: db.Spec.MaxDataExtensionTimeInDays,
		Transient:                  db.Spec.Transient,
		Catalog:                    db.Spec.Catalog,
		ExternalVolume:             db.Spec.ExternalVolume,
		ReplaceInvalidCharacters:   db.Spec.ReplaceInvalidCharacters,
		DefaultDDLCollation:        db.Spec.DefaultDDLCollation,
	}

	if db.Spec.StorageSerializationPolicy != nil {
		s := string(*db.Spec.StorageSerializationPolicy)
		opts.StorageSerializationPolicy = &s
	}

	if db.Spec.LogLevel != nil {
		s := string(*db.Spec.LogLevel)
		opts.LogLevel = &s
	}

	if db.Spec.MetricLevel != nil {
		s := string(*db.Spec.MetricLevel)
		opts.MetricLevel = &s
	}

	if db.Spec.TraceLevel != nil {
		s := string(*db.Spec.TraceLevel)
		opts.TraceLevel = &s
	}

	return opts
}

func buildAlterOptions(db *snowplanev1alpha1.Database, id snowflake.AccountObjectIdentifier, obs *snowflake.DatabaseObservation) snowflake.AlterDatabaseOptions {
	opts := snowflake.AlterDatabaseOptions{Name: id}

	// Detect fields that were previously managed but are now nil → UNSET.
	// This must happen before the obs.Parameters nil guard because UNSET
	// decisions are based on spec vs. TrackedParameters, not observed parameters.
	opts.UnsetFields = tracked.ComputeUnset(&db.Spec, db.Status.TrackedParameters)

	if db.Spec.Comment != nil {
		if obs.ShowOutput == nil || *db.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = db.Spec.Comment
		}
	}

	if obs.Parameters == nil {
		return opts
	}

	p := obs.Parameters

	if db.Spec.DataRetentionTimeInDays != nil {
		if p.DataRetentionTimeInDays == nil || *db.Spec.DataRetentionTimeInDays != *p.DataRetentionTimeInDays {
			opts.DataRetentionTimeInDays = db.Spec.DataRetentionTimeInDays
		}
	}

	if db.Spec.MaxDataExtensionTimeInDays != nil {
		if p.MaxDataExtensionTimeInDays == nil || *db.Spec.MaxDataExtensionTimeInDays != *p.MaxDataExtensionTimeInDays {
			opts.MaxDataExtensionTimeInDays = db.Spec.MaxDataExtensionTimeInDays
		}
	}

	if db.Spec.DefaultDDLCollation != nil && *db.Spec.DefaultDDLCollation != p.DefaultDDLCollation {
		opts.DefaultDDLCollation = db.Spec.DefaultDDLCollation
	}

	if db.Spec.ReplaceInvalidCharacters != nil {
		if p.ReplaceInvalidCharacters == nil || *db.Spec.ReplaceInvalidCharacters != *p.ReplaceInvalidCharacters {
			opts.ReplaceInvalidCharacters = db.Spec.ReplaceInvalidCharacters
		}
	}

	if db.Spec.Catalog != nil && *db.Spec.Catalog != p.Catalog {
		opts.Catalog = db.Spec.Catalog
	}

	if db.Spec.ExternalVolume != nil && *db.Spec.ExternalVolume != p.ExternalVolume {
		opts.ExternalVolume = db.Spec.ExternalVolume
	}

	if db.Spec.StorageSerializationPolicy != nil {
		s := string(*db.Spec.StorageSerializationPolicy)
		if s != p.StorageSerializationPolicy {
			opts.StorageSerializationPolicy = &s
		}
	}

	if db.Spec.LogLevel != nil {
		s := string(*db.Spec.LogLevel)
		if s != p.LogLevel {
			opts.LogLevel = &s
		}
	}

	if db.Spec.MetricLevel != nil {
		s := string(*db.Spec.MetricLevel)
		if s != p.MetricLevel {
			opts.MetricLevel = &s
		}
	}

	if db.Spec.TraceLevel != nil {
		s := string(*db.Spec.TraceLevel)
		if s != p.TraceLevel {
			opts.TraceLevel = &s
		}
	}

	return opts
}

// detectDrift compares desired spec against the observed state and
// returns a structured drift result.
func detectDrift(db *snowplanev1alpha1.Database, obs *snowflake.DatabaseObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", db.Spec.Name, obs.ShowOutput.Name, true)

		isTransient := obs.ShowOutput.Kind == "TRANSIENT"
		d.CompareBoolValue("TRANSIENT", db.Spec.Transient, isTransient, true)

		// Mutable fields.
		d.CompareString("COMMENT", db.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	if obs.Parameters != nil {
		p := obs.Parameters
		d.CompareInt32("DATA_RETENTION_TIME_IN_DAYS", db.Spec.DataRetentionTimeInDays, p.DataRetentionTimeInDays, false)
		d.CompareInt32("MAX_DATA_EXTENSION_TIME_IN_DAYS", db.Spec.MaxDataExtensionTimeInDays, p.MaxDataExtensionTimeInDays, false)
		d.CompareString("DEFAULT_DDL_COLLATION", db.Spec.DefaultDDLCollation, p.DefaultDDLCollation, false)
		d.CompareBool("REPLACE_INVALID_CHARACTERS", db.Spec.ReplaceInvalidCharacters, p.ReplaceInvalidCharacters, false)
		d.CompareString("CATALOG", db.Spec.Catalog, p.Catalog, false)
		d.CompareString("EXTERNAL_VOLUME", db.Spec.ExternalVolume, p.ExternalVolume, false)
		d.CompareStringFold("STORAGE_SERIALIZATION_POLICY", drift.PtrStringFrom(db.Spec.StorageSerializationPolicy), p.StorageSerializationPolicy, false)
		d.CompareStringFold("LOG_LEVEL", drift.PtrStringFrom(db.Spec.LogLevel), p.LogLevel, false)
		d.CompareStringFold("METRIC_LEVEL", drift.PtrStringFrom(db.Spec.MetricLevel), p.MetricLevel, false)
		d.CompareStringFold("TRACE_LEVEL", drift.PtrStringFrom(db.Spec.TraceLevel), p.TraceLevel, false)
	}

	return d.Result()
}
