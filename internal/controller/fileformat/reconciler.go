// Package fileformat implements the reconciler for FileFormat resources.
package fileformat

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
	finalizerName = "snowplane.hupe1980.github.io/fileformat"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake file formats.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error)
	Create(ctx context.Context, opts snowflake.CreateFileFormatOptions) error
	Alter(ctx context.Context, opts snowflake.AlterFileFormatOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new FileFormat reconciler.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.FileFormat, Service, *snowflake.FileFormatObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewFileFormatClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.FileFormat, Service, *snowflake.FileFormatObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for FileFormat resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.FileFormat, Service, *snowflake.FileFormatObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.FileFormat, Service, *snowflake.FileFormatObservation]{
		ResourceNameVal:  "fileformat",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.FileFormat { return &snowplanev1alpha1.FileFormat{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(ff *snowplanev1alpha1.FileFormat) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(ff.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(ff.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, ff.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.FileFormatObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.FileFormat, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterFileFormatOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.FileFormat, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.FileFormatObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.FileFormat, obs *reconciler.Observation[*snowflake.FileFormatObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.FileFormat, obs *reconciler.Observation[*snowflake.FileFormatObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		SupportsCoA: true,
		PreReconcileFn: func(ctx context.Context, ff *snowplanev1alpha1.FileFormat) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, ff,
				ff.Namespace, ff.Spec.DatabaseRef, ff.Spec.DatabaseName, ff.Status.DatabaseName)
			if err != nil {
				return err
			}

			ff.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, ff,
				ff.Namespace, ff.Spec.SchemaRef, ff.Spec.SchemaName, ff.Status.SchemaName)
			if err != nil {
				return err
			}

			ff.Status.SchemaName = schemaFQN

			refresolver.SetDatabaseAndSchemaResolvedCondition(ff, ff.Spec.DatabaseRef, ff.Spec.DatabaseName, ff.Spec.SchemaRef, ff.Spec.SchemaName)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.FileFormat{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.FileFormat)
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
				&snowplanev1alpha1.FileFormat{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.FileFormat)
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.FileFormatList{} }, ".spec.databaseRef.name", "listing fileformats for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.FileFormatList{} }, ".spec.schemaRef.name", "listing fileformats for schema watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, ff *snowplanev1alpha1.FileFormat) error {
	if reconciler.ShouldSkipImmutableValidation(ff) {
		return nil
	}

	if ff.Status.ShowOutput != nil {
		if ff.Status.ShowOutput.Name != "" && !strings.EqualFold(ff.Spec.Name, ff.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", ff.Status.ShowOutput.Name, ff.Spec.Name)
		}

		if ff.Status.ShowOutput.DatabaseName != "" && ff.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(ff.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, ff.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", ff.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if ff.Status.ShowOutput.SchemaName != "" && ff.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(ff.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, ff.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", ff.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}

		if ff.Status.ShowOutput.Type != "" && !strings.EqualFold(string(ff.Spec.Type), ff.Status.ShowOutput.Type) {
			return fmt.Errorf("spec.type is immutable after creation (current: %q, desired: %q)", ff.Status.ShowOutput.Type, ff.Spec.Type)
		}
	}

	return nil
}

func applyObservation(ff *snowplanev1alpha1.FileFormat, obs *snowflake.FileFormatObservation) {
	if obs.ShowOutput != nil {
		ff.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		ff.Status.DatabaseName = obs.ShowOutput.DatabaseName
		ff.Status.SchemaName = obs.ShowOutput.SchemaName

		ff.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(ff *snowplanev1alpha1.FileFormat, id snowflake.SchemaObjectIdentifier) snowflake.CreateFileFormatOptions {
	return snowflake.CreateFileFormatOptions{
		Name:                       id,
		Type:                       string(ff.Spec.Type),
		FieldDelimiter:             ff.Spec.FieldDelimiter,
		RecordDelimiter:            ff.Spec.RecordDelimiter,
		SkipHeader:                 ff.Spec.SkipHeader,
		FieldOptionallyEnclosedBy:  ff.Spec.FieldOptionallyEnclosedBy,
		Escape:                     ff.Spec.Escape,
		EscapeUnenclosedField:      ff.Spec.EscapeUnenclosedField,
		EmptyFieldAsNull:           ff.Spec.EmptyFieldAsNull,
		NullIf:                     ff.Spec.NullIf,
		ErrorOnColumnCountMismatch: ff.Spec.ErrorOnColumnCountMismatch,
		SkipBlankLines:             ff.Spec.SkipBlankLines,
		ParseHeader:                ff.Spec.ParseHeader,
		Encoding:                   ff.Spec.Encoding,
		Compression:                ff.Spec.Compression,
		StripOuterArray:            ff.Spec.StripOuterArray,
		StripNullValues:            ff.Spec.StripNullValues,
		EnableOctal:                ff.Spec.EnableOctal,
		AllowDuplicate:             ff.Spec.AllowDuplicate,
		BinaryAsText:               ff.Spec.BinaryAsText,
		UseLogicalType:             ff.Spec.UseLogicalType,
		SnappyCompression:          ff.Spec.SnappyCompression,
		PreserveSpace:              ff.Spec.PreserveSpace,
		StripOuterElement:          ff.Spec.StripOuterElement,
		DisableAutoConvert:         ff.Spec.DisableAutoConvert,
		DisableSnowflakeData:       ff.Spec.DisableSnowflakeData,
		ReplaceInvalidCharacters:   ff.Spec.ReplaceInvalidCharacters,
		SkipByteOrderMark:          ff.Spec.SkipByteOrderMark,
		IgnoreUtf8Errors:           ff.Spec.IgnoreUtf8Errors,
		DateFormat:                 ff.Spec.DateFormat,
		TimeFormat:                 ff.Spec.TimeFormat,
		TimestampFormat:            ff.Spec.TimestampFormat,
		BinaryFormat:               ff.Spec.BinaryFormat,
		TrimSpace:                  ff.Spec.TrimSpace,
		Comment:                    ff.Spec.Comment,
	}
}

func buildAlterOptions(ff *snowplanev1alpha1.FileFormat, id snowflake.SchemaObjectIdentifier, obs *snowflake.FileFormatObservation) snowflake.AlterFileFormatOptions {
	opts := snowflake.AlterFileFormatOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&ff.Spec, ff.Status.TrackedParameters)

	// FileFormat SHOW output does not expose per-field values, so we always
	// send the full spec for mutable fields to converge to the desired state.
	// This is consistent with how StorageIntegration handles fields where
	// observation lacks per-field granularity.
	opts.FieldDelimiter = ff.Spec.FieldDelimiter
	opts.RecordDelimiter = ff.Spec.RecordDelimiter
	opts.SkipHeader = ff.Spec.SkipHeader
	opts.FieldOptionallyEnclosedBy = ff.Spec.FieldOptionallyEnclosedBy
	opts.Escape = ff.Spec.Escape
	opts.EscapeUnenclosedField = ff.Spec.EscapeUnenclosedField
	opts.EmptyFieldAsNull = ff.Spec.EmptyFieldAsNull
	opts.ErrorOnColumnCountMismatch = ff.Spec.ErrorOnColumnCountMismatch
	opts.SkipBlankLines = ff.Spec.SkipBlankLines
	opts.ParseHeader = ff.Spec.ParseHeader
	opts.Encoding = ff.Spec.Encoding
	opts.Compression = ff.Spec.Compression
	opts.StripOuterArray = ff.Spec.StripOuterArray
	opts.StripNullValues = ff.Spec.StripNullValues
	opts.EnableOctal = ff.Spec.EnableOctal
	opts.AllowDuplicate = ff.Spec.AllowDuplicate
	opts.BinaryAsText = ff.Spec.BinaryAsText
	opts.UseLogicalType = ff.Spec.UseLogicalType
	opts.SnappyCompression = ff.Spec.SnappyCompression
	opts.PreserveSpace = ff.Spec.PreserveSpace
	opts.StripOuterElement = ff.Spec.StripOuterElement
	opts.DisableAutoConvert = ff.Spec.DisableAutoConvert
	opts.DisableSnowflakeData = ff.Spec.DisableSnowflakeData
	opts.ReplaceInvalidCharacters = ff.Spec.ReplaceInvalidCharacters
	opts.SkipByteOrderMark = ff.Spec.SkipByteOrderMark
	opts.IgnoreUtf8Errors = ff.Spec.IgnoreUtf8Errors
	opts.DateFormat = ff.Spec.DateFormat
	opts.TimeFormat = ff.Spec.TimeFormat
	opts.TimestampFormat = ff.Spec.TimestampFormat
	opts.BinaryFormat = ff.Spec.BinaryFormat
	opts.TrimSpace = ff.Spec.TrimSpace

	// Comment is available in SHOW output — compare before sending.
	if ff.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *ff.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = ff.Spec.Comment
		}
	}

	if len(ff.Spec.NullIf) > 0 {
		nullIf := make([]string, len(ff.Spec.NullIf))
		copy(nullIf, ff.Spec.NullIf)
		opts.NullIf = &nullIf
	}

	return opts
}

func detectDrift(ff *snowplanev1alpha1.FileFormat, obs *snowflake.FileFormatObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", ff.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(ff.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(ff.Status.SchemaName), obs.ShowOutput.SchemaName, true)
		d.CompareStringValueFold("TYPE", string(ff.Spec.Type), obs.ShowOutput.Type, true)

		// Mutable fields.
		d.CompareString("COMMENT", ff.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	return d.Result()
}
