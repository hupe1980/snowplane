// Package fileformat implements the reconciler for FileFormat resources.
package fileformat

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
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.FileFormat, Service, *snowflake.FileFormatObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.FileFormat, Service, *snowflake.FileFormatObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.FileFormat, Service, *snowflake.FileFormatObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.FileFormat, Service, *snowflake.FileFormatObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// defaultServiceFactory is the production ServiceFactory.
func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
	if err != nil {
		return nil, nil, err
	}

	return snowflake.NewFileFormatClient(sfC), cleanup, nil
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

		ff.Status.ShowOutput = &snowplanev1alpha1.FileFormatShowOutput{
			CreatedOn:    obs.ShowOutput.CreatedOn,
			Name:         obs.ShowOutput.Name,
			DatabaseName: obs.ShowOutput.DatabaseName,
			SchemaName:   obs.ShowOutput.SchemaName,
			Owner:        obs.ShowOutput.Owner,
			Comment:      obs.ShowOutput.Comment,
			Type:         obs.ShowOutput.Type,
		}
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
