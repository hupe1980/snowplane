// Package externaltable implements the reconciler for ExternalTable resources.
package externaltable

import (
	"context"
	"strings"

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
	finalizerName = "snowplane.hupe1980.github.io/external-table"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake external tables.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ExternalTableObservation, error)
	Create(ctx context.Context, opts snowflake.CreateExternalTableOptions) error
	Alter(ctx context.Context, opts snowflake.AlterExternalTableOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new ExternalTable reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.ExternalTable, Service, *snowflake.ExternalTableObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.ExternalTable, Service, *snowflake.ExternalTableObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.ExternalTable, Service, *snowflake.ExternalTableObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.ExternalTable, Service, *snowflake.ExternalTableObservation]{
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

	return snowflake.NewExternalTableClient(sfC), cleanup, nil
}

func applyObservation(et *snowplanev1alpha1.ExternalTable, obs *snowflake.ExternalTableObservation) {
	if obs.ShowOutput != nil {
		et.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		et.Status.DatabaseName = obs.ShowOutput.DatabaseName
		et.Status.SchemaName = obs.ShowOutput.SchemaName

		et.Status.ShowOutput = &snowplanev1alpha1.ExternalTableShowOutput{
			CreatedOn:           obs.ShowOutput.CreatedOn,
			Name:                obs.ShowOutput.Name,
			DatabaseName:        obs.ShowOutput.DatabaseName,
			SchemaName:          obs.ShowOutput.SchemaName,
			Invalid:             obs.ShowOutput.Invalid,
			InvalidReason:       obs.ShowOutput.InvalidReason,
			Owner:               obs.ShowOutput.Owner,
			Comment:             obs.ShowOutput.Comment,
			Stage:               obs.ShowOutput.Stage,
			Location:            obs.ShowOutput.Location,
			FileFormatName:      obs.ShowOutput.FileFormatName,
			FileFormatType:      obs.ShowOutput.FileFormatType,
			Cloud:               obs.ShowOutput.Cloud,
			Region:              obs.ShowOutput.Region,
			NotificationChannel: obs.ShowOutput.NotificationChannel,
			LastRefreshedOn:     obs.ShowOutput.LastRefreshedOn,
			TableFormat:         obs.ShowOutput.TableFormat,
			LastRefreshDetails:  obs.ShowOutput.LastRefreshDetails,
			OwnerRoleType:       obs.ShowOutput.OwnerRoleType,
		}
	}
}

func buildCreateOptions(et *snowplanev1alpha1.ExternalTable, id snowflake.SchemaObjectIdentifier) snowflake.CreateExternalTableOptions {
	opts := snowflake.CreateExternalTableOptions{
		Name:            id,
		Location:        et.Spec.Location,
		FileFormat:      et.Spec.FileFormat,
		PartitionBy:     et.Spec.PartitionBy,
		PartitionType:   et.Spec.PartitionType,
		Pattern:         et.Spec.Pattern,
		RefreshOnCreate: et.Spec.RefreshOnCreate,
		AutoRefresh:     et.Spec.AutoRefresh,
		AwsSnsTopic:     et.Spec.AwsSnsTopic,
		TableFormat:     et.Spec.TableFormat,
		Integration:     et.Spec.Integration,
		Comment:         et.Spec.Comment,
	}

	for _, col := range et.Spec.Columns {
		opts.Columns = append(opts.Columns, snowflake.ExternalTableColumnOpt{
			Name: col.Name,
			Type: col.Type,
			As:   col.As,
		})
	}

	return opts
}

func buildAlterOptions(et *snowplanev1alpha1.ExternalTable, id snowflake.SchemaObjectIdentifier, obs *snowflake.ExternalTableObservation) snowflake.AlterExternalTableOptions {
	opts := snowflake.AlterExternalTableOptions{Name: id}

	// AUTO_REFRESH is the only mutable field.
	// Since it uses nounset, ComputeUnset will never produce AUTO_REFRESH.
	// We only SET when spec differs from observed.
	if et.Spec.AutoRefresh != nil {
		if obs == nil || obs.ShowOutput == nil {
			opts.AutoRefresh = et.Spec.AutoRefresh
		} else {
			// Compare with SHOW output. auto_refresh is not directly in SHOW EXTERNAL TABLES,
			// so we always set if specified.
			opts.AutoRefresh = et.Spec.AutoRefresh
		}
	}

	return opts
}

func detectDrift(et *snowplanev1alpha1.ExternalTable, obs *snowflake.ExternalTableObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", et.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(et.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(et.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		// Comment is immutable but we still detect drift.
		if et.Spec.Comment != nil {
			observedComment := obs.ShowOutput.Comment
			d.CompareString("COMMENT", et.Spec.Comment, observedComment, true)
		}

		// Location drift.
		if et.Spec.Location != "" && obs.ShowOutput.Location != "" {
			// Location in SHOW output may differ in format from spec; normalize for comparison.
			if !strings.EqualFold(et.Spec.Location, obs.ShowOutput.Location) {
				d.CompareStringValue("LOCATION", et.Spec.Location, obs.ShowOutput.Location, true)
			}
		}
	}

	return d.Result()
}
