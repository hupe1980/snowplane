// Package alert implements the reconciler for Alert resources.
package alert

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
	finalizerName = "snowplane.hupe1980.github.io/alert"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake alerts.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error)
	Create(ctx context.Context, opts snowflake.CreateAlertOptions) error
	Alter(ctx context.Context, opts snowflake.AlterAlertOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Alert reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Alert, Service, *snowflake.AlertObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Alert, Service, *snowflake.AlertObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.Alert, Service, *snowflake.AlertObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Alert, Service, *snowflake.AlertObservation]{
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

	return snowflake.NewAlertClient(sfC), cleanup, nil
}

func applyObservation(alert *snowplanev1alpha1.Alert, obs *snowflake.AlertObservation) {
	if obs.ShowOutput != nil {
		alert.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		alert.Status.DatabaseName = obs.ShowOutput.DatabaseName
		alert.Status.SchemaName = obs.ShowOutput.SchemaName

		alert.Status.ShowOutput = &snowplanev1alpha1.AlertShowOutput{
			CreatedOn:    obs.ShowOutput.CreatedOn,
			Name:         obs.ShowOutput.Name,
			DatabaseName: obs.ShowOutput.DatabaseName,
			SchemaName:   obs.ShowOutput.SchemaName,
			Owner:        obs.ShowOutput.Owner,
			Comment:      obs.ShowOutput.Comment,
			Warehouse:    obs.ShowOutput.Warehouse,
			Schedule:     obs.ShowOutput.Schedule,
			State:        obs.ShowOutput.State,
			Condition:    obs.ShowOutput.Condition,
			Action:       obs.ShowOutput.Action,
		}
	}
}

func buildCreateOptions(alert *snowplanev1alpha1.Alert, id snowflake.SchemaObjectIdentifier) snowflake.CreateAlertOptions {
	return snowflake.CreateAlertOptions{
		Name:      id,
		Warehouse: alert.Spec.Warehouse,
		Schedule:  alert.Spec.Schedule,
		Comment:   alert.Spec.Comment,
		Condition: alert.Spec.Condition,
		Action:    alert.Spec.Action,
	}
}

func buildAlterOptions(alert *snowplanev1alpha1.Alert, id snowflake.SchemaObjectIdentifier, obs *snowflake.AlertObservation) snowflake.AlterAlertOptions {
	opts := snowflake.AlterAlertOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&alert.Spec, alert.Status.TrackedParameters)

	if alert.Spec.Comment != nil {
		if obs.ShowOutput == nil || *alert.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = alert.Spec.Comment
		}
	}

	if obs.ShowOutput != nil {
		// Pass current state so buildAlterAlertStatements can auto-suspend
		// before modifying condition/action/schedule/warehouse.
		opts.CurrentState = obs.ShowOutput.State

		// Schedule changes.
		if alert.Spec.Schedule != nil && *alert.Spec.Schedule != obs.ShowOutput.Schedule {
			opts.Schedule = alert.Spec.Schedule
		}

		// Warehouse changes.
		if alert.Spec.Warehouse != nil && *alert.Spec.Warehouse != obs.ShowOutput.Warehouse {
			opts.Warehouse = alert.Spec.Warehouse
		}

		// Condition changes.
		if alert.Spec.Condition != obs.ShowOutput.Condition {
			cond := alert.Spec.Condition
			opts.Condition = &cond
		}

		// Action changes.
		if alert.Spec.Action != obs.ShowOutput.Action {
			action := alert.Spec.Action
			opts.Action = &action
		}

		// Suspend/resume state changes.
		if alert.Spec.Suspend != nil {
			isSuspended := obs.ShowOutput.State == "suspended"
			if *alert.Spec.Suspend != isSuspended {
				opts.Suspend = alert.Spec.Suspend
			}
		}
	}

	return opts
}

func detectDrift(alert *snowplanev1alpha1.Alert, obs *snowflake.AlertObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", alert.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(alert.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(alert.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		// Mutable fields from SHOW output.
		d.CompareString("COMMENT", alert.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareString("SCHEDULE", alert.Spec.Schedule, obs.ShowOutput.Schedule, false)
		d.CompareString("WAREHOUSE", alert.Spec.Warehouse, obs.ShowOutput.Warehouse, false)
		d.CompareStringValue("CONDITION", alert.Spec.Condition, obs.ShowOutput.Condition, false)
		d.CompareStringValue("ACTION", alert.Spec.Action, obs.ShowOutput.Action, false)
	}

	return d.Result()
}
