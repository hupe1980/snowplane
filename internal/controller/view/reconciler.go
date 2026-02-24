// Package view implements the reconciler for View resources.
package view

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
	finalizerName = "snowplane.hupe1980.github.io/view"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake views.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error)
	Create(ctx context.Context, opts snowflake.CreateViewOptions) error
	Alter(ctx context.Context, opts snowflake.AlterViewOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new View reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.View, Service, *snowflake.ViewObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.View, Service, *snowflake.ViewObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.View, Service, *snowflake.ViewObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.View, Service, *snowflake.ViewObservation]{
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

	return snowflake.NewViewClient(sfC), cleanup, nil
}

func applyObservation(view *snowplanev1alpha1.View, obs *snowflake.ViewObservation) {
	if obs.ShowOutput != nil {
		view.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		view.Status.DatabaseName = obs.ShowOutput.DatabaseName
		view.Status.SchemaName = obs.ShowOutput.SchemaName

		view.Status.ShowOutput = &snowplanev1alpha1.ViewShowOutput{
			CreatedOn:      obs.ShowOutput.CreatedOn,
			Name:           obs.ShowOutput.Name,
			DatabaseName:   obs.ShowOutput.DatabaseName,
			SchemaName:     obs.ShowOutput.SchemaName,
			Comment:        obs.ShowOutput.Comment,
			Owner:          obs.ShowOutput.Owner,
			IsSecure:       obs.ShowOutput.IsSecure,
			Text:           obs.ShowOutput.Text,
			ChangeTracking: obs.ShowOutput.ChangeTracking,
		}
	}
}

func buildCreateOptions(view *snowplanev1alpha1.View, id snowflake.SchemaObjectIdentifier) snowflake.CreateViewOptions {
	return snowflake.CreateViewOptions{
		Name:           id,
		Statement:      view.Spec.Statement,
		Secure:         view.Spec.Secure,
		Comment:        view.Spec.Comment,
		ChangeTracking: view.Spec.ChangeTracking,
	}
}

func buildAlterOptions(view *snowplanev1alpha1.View, id snowflake.SchemaObjectIdentifier, obs *snowflake.ViewObservation) snowflake.AlterViewOptions {
	opts := snowflake.AlterViewOptions{Name: id}
	opts.UnsetFields = computeUnsetFields(view)

	// Detect statement change → requires CREATE OR REPLACE VIEW (R9-1).
	if obs.ShowOutput != nil && view.Spec.Statement != obs.ShowOutput.Text {
		opts.ReplaceStatement = &snowflake.ReplaceViewStatement{
			Statement:      view.Spec.Statement,
			Secure:         view.Spec.Secure,
			Comment:        view.Spec.Comment,
			ChangeTracking: view.Spec.ChangeTracking,
		}

		// When replacing, all fields are carried by the CREATE OR REPLACE
		// statement; no additional ALTER is needed.
		return opts
	}

	if view.Spec.Comment != nil {
		if obs.ShowOutput == nil || *view.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = view.Spec.Comment
		}
	}

	if obs.ShowOutput != nil {
		// Secure toggle: compare bool values.
		desiredSecure := view.Spec.Secure
		observedSecure := obs.ShowOutput.IsSecure

		if desiredSecure != observedSecure {
			opts.Secure = &desiredSecure
		}

		// Change tracking: compare bool values.
		if view.Spec.ChangeTracking != nil {
			if *view.Spec.ChangeTracking != obs.ShowOutput.ChangeTracking {
				opts.ChangeTracking = view.Spec.ChangeTracking
			}
		}
	}

	return opts
}

func computeUnsetFields(view *snowplanev1alpha1.View) []string {
	if len(view.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(view.Status.TrackedParameters))
	for _, f := range view.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if view.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	if view.Spec.ChangeTracking == nil && managed["CHANGE_TRACKING"] {
		unset = append(unset, "CHANGE_TRACKING")
	}

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.ViewSpec) []string {
	var fields []string

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	if spec.ChangeTracking != nil {
		fields = append(fields, "CHANGE_TRACKING")
	}

	return fields
}

func detectDrift(view *snowplanev1alpha1.View, obs *snowflake.ViewObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", view.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(view.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(view.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		// Mutable fields.
		d.CompareStringValue("STATEMENT", view.Spec.Statement, obs.ShowOutput.Text, false)
		d.CompareString("COMMENT", view.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareBoolValue("IS_SECURE", view.Spec.Secure, obs.ShowOutput.IsSecure, false)

		if view.Spec.ChangeTracking != nil {
			d.CompareBoolValue("CHANGE_TRACKING", *view.Spec.ChangeTracking, obs.ShowOutput.ChangeTracking, false)
		}
	}

	return d.Result()
}
