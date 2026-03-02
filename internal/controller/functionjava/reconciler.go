// Package functionjava implements the reconciler for FunctionJava resources.
package functionjava

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
	finalizerName = "snowplane.hupe1980.github.io/functionjava"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake functions.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) (*snowflake.FunctionObservation, error)
	Create(ctx context.Context, opts snowflake.CreateFunctionOptions) error
	Alter(ctx context.Context, opts snowflake.AlterFunctionOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new FunctionJava reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.FunctionJava, Service, *snowflake.FunctionObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.FunctionJava, Service, *snowflake.FunctionObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.FunctionJava, Service, *snowflake.FunctionObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.FunctionJava, Service, *snowflake.FunctionObservation]{
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

	return snowflake.NewFunctionClient(sfC), cleanup, nil
}

func applyObservation(obj *snowplanev1alpha1.FunctionJava, obs *snowflake.FunctionObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		obj.Status.DatabaseName = obs.ShowOutput.DatabaseName
		obj.Status.SchemaName = obs.ShowOutput.SchemaName

		obj.Status.ShowOutput = &snowplanev1alpha1.FunctionShowOutput{
			CreatedOn:    obs.ShowOutput.CreatedOn,
			Name:         obs.ShowOutput.Name,
			DatabaseName: obs.ShowOutput.DatabaseName,
			SchemaName:   obs.ShowOutput.SchemaName,
			Arguments:    obs.ShowOutput.Arguments,
			Description:  obs.ShowOutput.Description,
			Language:     obs.ShowOutput.Language,
			IsSecure:     obs.ShowOutput.IsSecure,
			Owner:        obs.ShowOutput.Owner,
		}
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.FunctionJava, id snowflake.CallableIdentifier) snowflake.CreateFunctionOptions {
	args := make([]snowflake.FunctionArgument, len(obj.Spec.Arguments))
	for i, a := range obj.Spec.Arguments {
		args[i] = snowflake.FunctionArgument{Name: a.Name, Type: a.Type}
	}

	handler := obj.Spec.Handler
	runtimeVersion := obj.Spec.RuntimeVersion

	// Snowpark package goes as the first item in Packages.
	packages := make([]string, 0, 1+len(obj.Spec.Packages))
	packages = append(packages, obj.Spec.SnowparkPackage)
	packages = append(packages, obj.Spec.Packages...)

	// Map SecretBinding slice to map[string]string.
	var secrets map[string]string
	if len(obj.Spec.Secrets) > 0 {
		secrets = make(map[string]string, len(obj.Spec.Secrets))
		for _, s := range obj.Spec.Secrets {
			secrets[s.VariableName] = s.SecretName
		}
	}

	opts := snowflake.CreateFunctionOptions{
		Name:                       id.SchemaObjectIdentifier,
		Arguments:                  args,
		Returns:                    obj.Spec.Returns,
		Language:                   "JAVA",
		Body:                       obj.Spec.Body,
		Handler:                    &handler,
		RuntimeVersion:             &runtimeVersion,
		Packages:                   packages,
		Imports:                    obj.Spec.Imports,
		TargetPath:                 obj.Spec.TargetPath,
		ExternalAccessIntegrations: obj.Spec.ExternalAccessIntegrations,
		Secrets:                    secrets,
		Volatility:                 obj.Spec.Volatility,
		NullInputBehavior:          obj.Spec.NullInputBehavior,
		Secure:                     obj.Spec.Secure,
		Comment:                    obj.Spec.Comment,
	}

	return opts
}

func buildAlterOptions(obj *snowplanev1alpha1.FunctionJava, id snowflake.CallableIdentifier, obs *snowflake.FunctionObservation) snowflake.AlterFunctionOptions {
	opts := snowflake.AlterFunctionOptions{
		Name:     id.SchemaObjectIdentifier,
		ArgTypes: id.ArgTypes(),
	}
	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	// Secure: detect toggle against observed state.
	if obs != nil && obs.ShowOutput != nil {
		if obj.Spec.Secure != obs.ShowOutput.IsSecure {
			secure := obj.Spec.Secure
			opts.Secure = &secure
		}
	}

	if obj.Spec.Comment != nil {
		if obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Description {
			opts.Comment = obj.Spec.Comment
		}
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.FunctionJava, obs *snowflake.FunctionObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Description, false)
		d.CompareBoolValue("IS_SECURE", obj.Spec.Secure, obs.ShowOutput.IsSecure, false)
	}

	return d.Result()
}
