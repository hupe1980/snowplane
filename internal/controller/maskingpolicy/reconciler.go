// Package maskingpolicy implements the reconciler for MaskingPolicy resources.
package maskingpolicy

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
	finalizerName = "snowplane.hupe1980.github.io/maskingpolicy"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake masking policies.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error)
	Create(ctx context.Context, opts snowflake.CreateMaskingPolicyOptions) error
	Alter(ctx context.Context, opts snowflake.AlterMaskingPolicyOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new MaskingPolicy reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicy, Service] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicy, Service]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicy, Service] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicy, Service]{
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

	return snowflake.NewMaskingPolicyClient(sfC), cleanup, nil
}

func applyObservation(mp *snowplanev1alpha1.MaskingPolicy, obs *snowflake.MaskingPolicyObservation) {
	if obs.ShowOutput != nil {
		mp.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()

		mp.Status.ShowOutput = &snowplanev1alpha1.MaskingPolicyShowOutput{
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

func buildCreateOptions(mp *snowplanev1alpha1.MaskingPolicy, id snowflake.SchemaObjectIdentifier) snowflake.CreateMaskingPolicyOptions {
	sig := make([]snowflake.MaskingPolicyArgument, len(mp.Spec.Signature))
	for i, arg := range mp.Spec.Signature {
		sig[i] = snowflake.MaskingPolicyArgument{
			Name: arg.Name,
			Type: arg.Type,
		}
	}

	return snowflake.CreateMaskingPolicyOptions{
		Name:                id,
		Signature:           sig,
		Body:                mp.Spec.Body,
		ExemptOtherPolicies: mp.Spec.ExemptOtherPolicies,
		Comment:             mp.Spec.Comment,
	}
}

func buildAlterOptions(mp *snowplanev1alpha1.MaskingPolicy, id snowflake.SchemaObjectIdentifier, _ *snowflake.MaskingPolicyObservation) snowflake.AlterMaskingPolicyOptions {
	opts := snowflake.AlterMaskingPolicyOptions{Name: id}
	opts.UnsetFields = computeUnsetFields(mp)

	// Body is always sent to ensure convergence.
	body := mp.Spec.Body
	opts.Body = &body

	if mp.Spec.Comment != nil {
		opts.Comment = mp.Spec.Comment
	}

	return opts
}

func computeUnsetFields(mp *snowplanev1alpha1.MaskingPolicy) []string {
	if len(mp.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(mp.Status.TrackedParameters))
	for _, f := range mp.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if mp.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.MaskingPolicySpec) []string {
	var fields []string

	// Body is always tracked since it's required.
	fields = append(fields, "BODY")

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	if spec.ExemptOtherPolicies != nil {
		fields = append(fields, "EXEMPT_OTHER_POLICIES")
	}

	return fields
}

func detectDrift(mp *snowplanev1alpha1.MaskingPolicy, obs *snowflake.MaskingPolicyObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", mp.Spec.Name, obs.ShowOutput.Name, true)

		// Mutable fields.
		d.CompareString("COMMENT", mp.Spec.Comment, obs.ShowOutput.Comment, false)

		// Note: Body is not available in SHOW output, so drift detection for body
		// relies on spec-hash comparison. Comment drift is detectable from SHOW.
	}

	return d.Result()
}
