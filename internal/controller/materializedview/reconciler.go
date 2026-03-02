// Package materializedview implements the reconciler for MaterializedView resources.
package materializedview

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
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/materializedview"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake materialized views.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error)
	Create(ctx context.Context, opts snowflake.CreateMaterializedViewOptions) error
	Alter(ctx context.Context, opts snowflake.AlterMaterializedViewOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new MaterializedView reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.MaterializedView, Service, *snowflake.MaterializedViewObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.MaterializedView, Service, *snowflake.MaterializedViewObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.MaterializedView, Service, *snowflake.MaterializedViewObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.MaterializedView, Service, *snowflake.MaterializedViewObservation]{
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

	return snowflake.NewMaterializedViewClient(sfC), cleanup, nil
}

func applyObservation(mv *snowplanev1alpha1.MaterializedView, obs *snowflake.MaterializedViewObservation) {
	if obs.ShowOutput != nil {
		mv.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		mv.Status.DatabaseName = obs.ShowOutput.DatabaseName
		mv.Status.SchemaName = obs.ShowOutput.SchemaName

		mv.Status.ShowOutput = &snowplanev1alpha1.MaterializedViewShowOutput{
			CreatedOn:           obs.ShowOutput.CreatedOn,
			Name:                obs.ShowOutput.Name,
			DatabaseName:        obs.ShowOutput.DatabaseName,
			SchemaName:          obs.ShowOutput.SchemaName,
			ClusterBy:           obs.ShowOutput.ClusterBy,
			Rows:                obs.ShowOutput.Rows,
			Bytes:               obs.ShowOutput.Bytes,
			SourceDatabaseName:  obs.ShowOutput.SourceDatabaseName,
			SourceSchemaName:    obs.ShowOutput.SourceSchemaName,
			SourceTableName:     obs.ShowOutput.SourceTableName,
			RefreshedOn:         obs.ShowOutput.RefreshedOn,
			CompactedOn:         obs.ShowOutput.CompactedOn,
			Owner:               obs.ShowOutput.Owner,
			Invalid:             obs.ShowOutput.Invalid,
			InvalidReason:       obs.ShowOutput.InvalidReason,
			BehindBy:            obs.ShowOutput.BehindBy,
			Comment:             obs.ShowOutput.Comment,
			Text:                obs.ShowOutput.Text,
			IsSecure:            obs.ShowOutput.IsSecure,
			AutomaticClustering: obs.ShowOutput.AutomaticClustering,
			OwnerRoleType:       obs.ShowOutput.OwnerRoleType,
		}
	}
}

func buildCreateOptions(mv *snowplanev1alpha1.MaterializedView, id snowflake.SchemaObjectIdentifier) snowflake.CreateMaterializedViewOptions {
	return snowflake.CreateMaterializedViewOptions{
		Name:      id,
		Statement: mv.Spec.Statement,
		Secure:    mv.Spec.Secure,
		Comment:   mv.Spec.Comment,
		ClusterBy: mv.Spec.ClusterBy,
	}
}

func buildAlterOptions(mv *snowplanev1alpha1.MaterializedView, id snowflake.SchemaObjectIdentifier, obs *snowflake.MaterializedViewObservation) snowflake.AlterMaterializedViewOptions {
	opts := snowflake.AlterMaterializedViewOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&mv.Spec, mv.Status.TrackedParameters)

	if mv.Spec.Comment != nil {
		if obs.ShowOutput == nil || *mv.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = mv.Spec.Comment
		}
	}

	if obs.ShowOutput != nil {
		// Secure toggle: compare bool values.
		desiredSecure := mv.Spec.Secure
		observedSecure := strings.EqualFold(obs.ShowOutput.IsSecure, "true")

		if desiredSecure != observedSecure {
			opts.Secure = &desiredSecure
		}
	}

	return opts
}

func detectDrift(mv *snowplanev1alpha1.MaterializedView, obs *snowflake.MaterializedViewObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", mv.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(mv.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(mv.Status.SchemaName), obs.ShowOutput.SchemaName, true)
		d.CompareStringValue("STATEMENT", mv.Spec.Statement, obs.ShowOutput.Text, true)

		// Mutable fields.
		d.CompareString("COMMENT", mv.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareBoolValue("IS_SECURE", mv.Spec.Secure, strings.EqualFold(obs.ShowOutput.IsSecure, "true"), false)
	}

	return d.Result()
}
