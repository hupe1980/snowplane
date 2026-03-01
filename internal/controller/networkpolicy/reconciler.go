// Package networkpolicy implements the reconciler for NetworkPolicy resources.
package networkpolicy

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
	finalizerName = "snowplane.hupe1980.github.io/networkpolicy"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake network policies.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error)
	Create(ctx context.Context, opts snowflake.CreateNetworkPolicyOptions) error
	Alter(ctx context.Context, opts snowflake.AlterNetworkPolicyOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new NetworkPolicy reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.NetworkPolicy, Service, *snowflake.NetworkPolicyObservation] {
	a := &adapter{newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.NetworkPolicy, Service, *snowflake.NetworkPolicyObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.NetworkPolicy, Service, *snowflake.NetworkPolicyObservation] {
	a := &adapter{newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.NetworkPolicy, Service, *snowflake.NetworkPolicyObservation]{
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

	return snowflake.NewNetworkPolicyClient(sfC), cleanup, nil
}

func applyObservation(np *snowplanev1alpha1.NetworkPolicy, obs *snowflake.NetworkPolicyObservation) {
	if obs.ShowOutput != nil {
		np.Status.FullyQualifiedName = obs.ShowOutput.Name

		np.Status.ShowOutput = &snowplanev1alpha1.NetworkPolicyShowOutput{
			CreatedOn:              obs.ShowOutput.CreatedOn,
			Name:                   obs.ShowOutput.Name,
			Comment:                obs.ShowOutput.Comment,
			EntriesInAllowedIPList: obs.ShowOutput.EntriesInAllowedIPList,
			EntriesInBlockedIPList: obs.ShowOutput.EntriesInBlockedIPList,
		}
	}
}

func buildCreateOptions(np *snowplanev1alpha1.NetworkPolicy, id snowflake.AccountObjectIdentifier) snowflake.CreateNetworkPolicyOptions {
	return snowflake.CreateNetworkPolicyOptions{
		Name:                   id,
		AllowedIPList:          np.Spec.AllowedIPList,
		BlockedIPList:          np.Spec.BlockedIPList,
		AllowedNetworkRuleList: np.Spec.AllowedNetworkRuleList,
		BlockedNetworkRuleList: np.Spec.BlockedNetworkRuleList,
		Comment:                np.Spec.Comment,
	}
}

func buildAlterOptions(np *snowplanev1alpha1.NetworkPolicy, id snowflake.AccountObjectIdentifier, obs *snowflake.NetworkPolicyObservation) snowflake.AlterNetworkPolicyOptions {
	opts := snowflake.AlterNetworkPolicyOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&np.Spec, np.Status.TrackedParameters)

	// For IP lists we always SET the full list to converge.
	// SHOW NETWORK POLICIES only shows counts, not actual IPs, so we
	// always send the full spec to Snowflake on each alter.
	if len(np.Spec.AllowedIPList) > 0 {
		ips := make([]string, len(np.Spec.AllowedIPList))
		copy(ips, np.Spec.AllowedIPList)
		opts.AllowedIPList = &ips
	}

	if len(np.Spec.BlockedIPList) > 0 {
		ips := make([]string, len(np.Spec.BlockedIPList))
		copy(ips, np.Spec.BlockedIPList)
		opts.BlockedIPList = &ips
	}

	if len(np.Spec.AllowedNetworkRuleList) > 0 {
		rules := make([]string, len(np.Spec.AllowedNetworkRuleList))
		copy(rules, np.Spec.AllowedNetworkRuleList)
		opts.AllowedNetworkRuleList = &rules
	}

	if len(np.Spec.BlockedNetworkRuleList) > 0 {
		rules := make([]string, len(np.Spec.BlockedNetworkRuleList))
		copy(rules, np.Spec.BlockedNetworkRuleList)
		opts.BlockedNetworkRuleList = &rules
	}

	if np.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *np.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = np.Spec.Comment
		}
	}

	return opts
}

func detectDrift(np *snowplanev1alpha1.NetworkPolicy, obs *snowflake.NetworkPolicyObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", np.Spec.Name, obs.ShowOutput.Name, true)

		// Mutable fields.
		d.CompareString("COMMENT", np.Spec.Comment, obs.ShowOutput.Comment, false)

		// Note: SHOW NETWORK POLICIES only returns entry counts for IP lists
		// (e.g. "2"), not actual IP addresses. Meaningful IP drift detection is
		// not possible from SHOW output alone. We rely on spec-hash comparison
		// to detect user-side changes, which triggers a full-list ALTER SET.
	}

	return d.Result()
}
