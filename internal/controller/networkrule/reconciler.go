// Package networkrule implements the reconciler for NetworkRule resources.
package networkrule

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
	finalizerName = "snowplane.hupe1980.github.io/networkrule"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake network rules.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.NetworkRuleObservation, error)
	Create(ctx context.Context, opts snowflake.CreateNetworkRuleOptions) error
	Alter(ctx context.Context, opts snowflake.AlterNetworkRuleOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new NetworkRule reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.NetworkRule, Service, *snowflake.NetworkRuleObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.NetworkRule, Service, *snowflake.NetworkRuleObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.NetworkRule, Service, *snowflake.NetworkRuleObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.NetworkRule, Service, *snowflake.NetworkRuleObservation]{
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

	return snowflake.NewNetworkRuleClient(sfC), cleanup, nil
}

func applyObservation(nr *snowplanev1alpha1.NetworkRule, obs *snowflake.NetworkRuleObservation) {
	if obs.ShowOutput != nil {
		nr.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()

		nr.Status.ShowOutput = &snowplanev1alpha1.NetworkRuleShowOutput{
			CreatedOn:    obs.ShowOutput.CreatedOn,
			Name:         obs.ShowOutput.Name,
			DatabaseName: obs.ShowOutput.DatabaseName,
			SchemaName:   obs.ShowOutput.SchemaName,
			Owner:        obs.ShowOutput.Owner,
			Type:         obs.ShowOutput.Type,
			Mode:         obs.ShowOutput.Mode,
			Comment:      obs.ShowOutput.Comment,
		}
	}
}

func buildCreateOptions(nr *snowplanev1alpha1.NetworkRule, id snowflake.SchemaObjectIdentifier) snowflake.CreateNetworkRuleOptions {
	return snowflake.CreateNetworkRuleOptions{
		Name:      id,
		Type:      string(nr.Spec.Type),
		Mode:      string(nr.Spec.Mode),
		ValueList: nr.Spec.ValueList,
		Comment:   nr.Spec.Comment,
	}
}

func buildAlterOptions(nr *snowplanev1alpha1.NetworkRule, id snowflake.SchemaObjectIdentifier, _ *snowflake.NetworkRuleObservation) snowflake.AlterNetworkRuleOptions {
	opts := snowflake.AlterNetworkRuleOptions{Name: id}
	opts.UnsetFields = computeUnsetFields(nr)

	// ValueList is always sent on update to ensure convergence.
	valueList := make([]string, len(nr.Spec.ValueList))
	copy(valueList, nr.Spec.ValueList)
	opts.ValueList = &valueList

	if nr.Spec.Comment != nil {
		opts.Comment = nr.Spec.Comment
	}

	return opts
}

func computeUnsetFields(nr *snowplanev1alpha1.NetworkRule) []string {
	if len(nr.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(nr.Status.TrackedParameters))
	for _, f := range nr.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if nr.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.NetworkRuleSpec) []string {
	var fields []string

	// ValueList is always tracked since it's required.
	fields = append(fields, "VALUE_LIST")

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	return fields
}

func detectDrift(nr *snowplanev1alpha1.NetworkRule, obs *snowflake.NetworkRuleObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", nr.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("TYPE", string(nr.Spec.Type), obs.ShowOutput.Type, true)
		d.CompareStringValueFold("MODE", string(nr.Spec.Mode), obs.ShowOutput.Mode, true)

		// Mutable fields.
		d.CompareString("COMMENT", nr.Spec.Comment, obs.ShowOutput.Comment, false)

		// Note: VALUE_LIST is not available in SHOW output; drift detection for
		// value list relies on DESCRIBE output or spec-hash comparison.
	}

	// If DESCRIBE output contains value_list, compare it.
	if obs.DescribeOutput != nil {
		if descValues, ok := obs.DescribeOutput["value_list"]; ok {
			specValues := strings.Join(nr.Spec.ValueList, ",")
			d.CompareStringValueFold("VALUE_LIST", specValues, descValues, false)
		}
	}

	return d.Result()
}
