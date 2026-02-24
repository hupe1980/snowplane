// Package pipe implements the reconciler for Pipe resources.
package pipe

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
	finalizerName = "snowplane.hupe1980.github.io/pipe"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake pipes.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.PipeObservation, error)
	Create(ctx context.Context, opts snowflake.CreatePipeOptions) error
	Alter(ctx context.Context, opts snowflake.AlterPipeOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Pipe reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Pipe, Service, *snowflake.PipeObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Pipe, Service, *snowflake.PipeObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.Pipe, Service, *snowflake.PipeObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Pipe, Service, *snowflake.PipeObservation]{
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

	return snowflake.NewPipeClient(sfC), cleanup, nil
}

func applyObservation(pipe *snowplanev1alpha1.Pipe, obs *snowflake.PipeObservation) {
	if obs.ShowOutput != nil {
		pipe.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		pipe.Status.DatabaseName = obs.ShowOutput.DatabaseName
		pipe.Status.SchemaName = obs.ShowOutput.SchemaName
		pipe.Status.NotificationChannel = obs.ShowOutput.NotificationChannel

		pipe.Status.ShowOutput = &snowplanev1alpha1.PipeShowOutput{
			CreatedOn:           obs.ShowOutput.CreatedOn,
			Name:                obs.ShowOutput.Name,
			DatabaseName:        obs.ShowOutput.DatabaseName,
			SchemaName:          obs.ShowOutput.SchemaName,
			Owner:               obs.ShowOutput.Owner,
			Comment:             obs.ShowOutput.Comment,
			Definition:          obs.ShowOutput.Definition,
			NotificationChannel: obs.ShowOutput.NotificationChannel,
			Integration:         obs.ShowOutput.Integration,
			ErrorIntegration:    obs.ShowOutput.ErrorIntegration,
			AwsSnsTopic:         obs.ShowOutput.AwsSnsTopic,
		}
	}
}

func buildCreateOptions(pipe *snowplanev1alpha1.Pipe, id snowflake.SchemaObjectIdentifier) snowflake.CreatePipeOptions {
	return snowflake.CreatePipeOptions{
		Name:             id,
		CopyStatement:    pipe.Spec.CopyStatement,
		AutoIngest:       pipe.Spec.AutoIngest,
		Integration:      pipe.Spec.Integration,
		AwsSnsTopic:      pipe.Spec.AwsSnsTopic,
		ErrorIntegration: pipe.Spec.ErrorIntegration,
		Comment:          pipe.Spec.Comment,
	}
}

func buildAlterOptions(pipe *snowplanev1alpha1.Pipe, id snowflake.SchemaObjectIdentifier, obs *snowflake.PipeObservation) snowflake.AlterPipeOptions {
	opts := snowflake.AlterPipeOptions{Name: id}
	opts.UnsetFields = computeUnsetFields(pipe)

	// Comment: set if changed.
	if pipe.Spec.Comment != nil {
		if obs.ShowOutput == nil || *pipe.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = pipe.Spec.Comment
		}
	}

	// ErrorIntegration: set if changed.
	if pipe.Spec.ErrorIntegration != nil {
		if obs.ShowOutput == nil || *pipe.Spec.ErrorIntegration != obs.ShowOutput.ErrorIntegration {
			opts.ErrorIntegration = pipe.Spec.ErrorIntegration
		}
	}

	return opts
}

func computeUnsetFields(pipe *snowplanev1alpha1.Pipe) []string {
	if len(pipe.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(pipe.Status.TrackedParameters))
	for _, f := range pipe.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if pipe.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	if pipe.Spec.ErrorIntegration == nil && managed["ERROR_INTEGRATION"] {
		unset = append(unset, "ERROR_INTEGRATION")
	}

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.PipeSpec) []string {
	var fields []string

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	if spec.ErrorIntegration != nil {
		fields = append(fields, "ERROR_INTEGRATION")
	}

	return fields
}

func detectDrift(pipe *snowplanev1alpha1.Pipe, obs *snowflake.PipeObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", pipe.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(pipe.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(pipe.Status.SchemaName), obs.ShowOutput.SchemaName, true)
		d.CompareStringValue("DEFINITION", pipe.Spec.CopyStatement, obs.ShowOutput.Definition, true)
		d.CompareString("INTEGRATION", pipe.Spec.Integration, obs.ShowOutput.Integration, true)
		d.CompareString("AWS_SNS_TOPIC", pipe.Spec.AwsSnsTopic, obs.ShowOutput.AwsSnsTopic, true)

		// Mutable fields.
		d.CompareString("COMMENT", pipe.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareString("ERROR_INTEGRATION", pipe.Spec.ErrorIntegration, obs.ShowOutput.ErrorIntegration, false)
	}

	return d.Result()
}
