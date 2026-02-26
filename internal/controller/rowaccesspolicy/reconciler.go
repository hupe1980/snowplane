// Package rowaccesspolicy implements the reconciler for RowAccessPolicy resources.
package rowaccesspolicy

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
	finalizerName = "snowplane.hupe1980.github.io/rowaccesspolicy"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake row access policies.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error)
	Create(ctx context.Context, opts snowflake.CreateRowAccessPolicyOptions) error
	Alter(ctx context.Context, opts snowflake.AlterRowAccessPolicyOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new RowAccessPolicy reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.RowAccessPolicy, Service, *snowflake.RowAccessPolicyObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.RowAccessPolicy, Service, *snowflake.RowAccessPolicyObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.RowAccessPolicy, Service, *snowflake.RowAccessPolicyObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.RowAccessPolicy, Service, *snowflake.RowAccessPolicyObservation]{
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

	return snowflake.NewRowAccessPolicyClient(sfC), cleanup, nil
}

func applyObservation(rap *snowplanev1alpha1.RowAccessPolicy, obs *snowflake.RowAccessPolicyObservation) {
	if obs.ShowOutput != nil {
		rap.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()

		rap.Status.ShowOutput = &snowplanev1alpha1.RowAccessPolicyShowOutput{
			CreatedOn:    obs.ShowOutput.CreatedOn,
			Name:         obs.ShowOutput.Name,
			DatabaseName: obs.ShowOutput.DatabaseName,
			SchemaName:   obs.ShowOutput.SchemaName,
			Kind:         obs.ShowOutput.Kind,
			Owner:        obs.ShowOutput.Owner,
			Comment:      obs.ShowOutput.Comment,
		}
	}
}

func buildCreateOptions(rap *snowplanev1alpha1.RowAccessPolicy, id snowflake.SchemaObjectIdentifier) snowflake.CreateRowAccessPolicyOptions {
	sig := make([]snowflake.RowAccessPolicyArgument, len(rap.Spec.Signature))
	for i, arg := range rap.Spec.Signature {
		sig[i] = snowflake.RowAccessPolicyArgument{
			Name: arg.Name,
			Type: arg.Type,
		}
	}

	return snowflake.CreateRowAccessPolicyOptions{
		Name:      id,
		Signature: sig,
		Body:      rap.Spec.Body,
		Comment:   rap.Spec.Comment,
	}
}

func buildAlterOptions(rap *snowplanev1alpha1.RowAccessPolicy, id snowflake.SchemaObjectIdentifier, obs *snowflake.RowAccessPolicyObservation) snowflake.AlterRowAccessPolicyOptions {
	opts := snowflake.AlterRowAccessPolicyOptions{Name: id}
	opts.UnsetFields = computeUnsetFields(rap)

	// Body is always sent to ensure convergence (not in SHOW output).
	body := rap.Spec.Body
	opts.Body = &body

	if rap.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *rap.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = rap.Spec.Comment
		}
	}

	return opts
}

func computeUnsetFields(rap *snowplanev1alpha1.RowAccessPolicy) []string {
	if len(rap.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(rap.Status.TrackedParameters))
	for _, f := range rap.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if rap.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.RowAccessPolicySpec) []string {
	var fields []string

	// Body is always tracked since it's required.
	fields = append(fields, "BODY")

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	return fields
}

func detectDrift(rap *snowplanev1alpha1.RowAccessPolicy, obs *snowflake.RowAccessPolicyObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", rap.Spec.Name, obs.ShowOutput.Name, true)

		// Mutable fields.
		d.CompareString("COMMENT", rap.Spec.Comment, obs.ShowOutput.Comment, false)

		// Note: Body is not available in SHOW output, so drift detection for body
		// relies on spec-hash comparison.
	}

	return d.Result()
}
