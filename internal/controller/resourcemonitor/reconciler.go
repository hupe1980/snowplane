// Package resourcemonitor implements the reconciler for ResourceMonitor resources.
package resourcemonitor

import (
	"context"
	"fmt"
	"sort"
	"strconv"
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
	finalizerName = "snowplane.hupe1980.github.io/resourcemonitor"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake resource monitors.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error)
	Create(ctx context.Context, opts snowflake.CreateResourceMonitorOptions) error
	Alter(ctx context.Context, opts snowflake.AlterResourceMonitorOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new ResourceMonitor reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.ResourceMonitor, Service] {
	a := &adapter{newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.ResourceMonitor, Service]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.ResourceMonitor, Service] {
	a := &adapter{newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.ResourceMonitor, Service]{
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

	return snowflake.NewResourceMonitorClient(sfC), cleanup, nil
}

func applyObservation(rm *snowplanev1alpha1.ResourceMonitor, obs *snowflake.ResourceMonitorObservation) {
	if obs.ShowOutput != nil {
		rm.Status.FullyQualifiedName = obs.ShowOutput.Name

		rm.Status.ShowOutput = &snowplanev1alpha1.ResourceMonitorShowOutput{
			CreatedOn:            obs.ShowOutput.CreatedOn,
			Name:                 obs.ShowOutput.Name,
			CreditQuota:          obs.ShowOutput.CreditQuota,
			UsedCredits:          obs.ShowOutput.UsedCredits,
			RemainingCredits:     obs.ShowOutput.RemainingCredits,
			Level:                obs.ShowOutput.Level,
			Frequency:            obs.ShowOutput.Frequency,
			StartTime:            obs.ShowOutput.StartTime,
			EndTime:              obs.ShowOutput.EndTime,
			NotifyAt:             obs.ShowOutput.NotifyAt,
			SuspendAt:            obs.ShowOutput.SuspendAt,
			SuspendImmediatelyAt: obs.ShowOutput.SuspendImmediatelyAt,
			NotifyUsers:          obs.ShowOutput.NotifyUsers,
		}
	}
}

func buildCreateOptions(rm *snowplanev1alpha1.ResourceMonitor, id snowflake.AccountObjectIdentifier) snowflake.CreateResourceMonitorOptions {
	opts := snowflake.CreateResourceMonitorOptions{
		Name:        id,
		CreditQuota: rm.Spec.CreditQuota,
	}

	if rm.Spec.Frequency != nil {
		f := string(*rm.Spec.Frequency)
		opts.Frequency = &f
	}

	opts.StartTimestamp = rm.Spec.StartTimestamp
	opts.EndTimestamp = rm.Spec.EndTimestamp
	opts.NotifyUsers = rm.Spec.NotifyUsers
	opts.Triggers = specTriggersToClient(rm.Spec.Triggers)

	return opts
}

func buildAlterOptions(rm *snowplanev1alpha1.ResourceMonitor, id snowflake.AccountObjectIdentifier, _ *snowflake.ResourceMonitorObservation) snowflake.AlterResourceMonitorOptions {
	opts := snowflake.AlterResourceMonitorOptions{Name: id}
	opts.UnsetFields = computeUnsetFields(rm)

	opts.CreditQuota = rm.Spec.CreditQuota

	if rm.Spec.Frequency != nil {
		f := string(*rm.Spec.Frequency)
		opts.Frequency = &f
	}

	opts.StartTimestamp = rm.Spec.StartTimestamp
	opts.EndTimestamp = rm.Spec.EndTimestamp

	if len(rm.Spec.NotifyUsers) > 0 {
		users := make([]string, len(rm.Spec.NotifyUsers))
		copy(users, rm.Spec.NotifyUsers)
		opts.NotifyUsers = &users
	}

	if len(rm.Spec.Triggers) > 0 {
		triggers := specTriggersToClient(rm.Spec.Triggers)
		opts.Triggers = &triggers
	}

	return opts
}

func computeUnsetFields(rm *snowplanev1alpha1.ResourceMonitor) []string {
	if len(rm.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(rm.Status.TrackedParameters))
	for _, f := range rm.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string
	// Note: Most resource monitor fields cannot be UNSET. The only meaningful
	// scenario is clearing notify_users or triggers. These are handled by
	// sending empty values in ALTER rather than UNSET.
	_ = managed

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.ResourceMonitorSpec) []string {
	var fields []string

	if spec.CreditQuota != nil {
		fields = append(fields, "CREDIT_QUOTA")
	}

	if spec.Frequency != nil {
		fields = append(fields, "FREQUENCY")
	}

	if spec.StartTimestamp != nil {
		fields = append(fields, "START_TIMESTAMP")
	}

	if spec.EndTimestamp != nil {
		fields = append(fields, "END_TIMESTAMP")
	}

	if len(spec.NotifyUsers) > 0 {
		fields = append(fields, "NOTIFY_USERS")
	}

	if len(spec.Triggers) > 0 {
		fields = append(fields, "TRIGGERS")
	}

	return fields
}

func detectDrift(rm *snowplanev1alpha1.ResourceMonitor, obs *snowflake.ResourceMonitorObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", rm.Spec.Name, obs.ShowOutput.Name, true)

		// Mutable fields.
		if rm.Spec.CreditQuota != nil {
			d.CompareStringValue("CREDIT_QUOTA", fmt.Sprintf("%d", *rm.Spec.CreditQuota), obs.ShowOutput.CreditQuota, false)
		}

		if rm.Spec.Frequency != nil {
			d.CompareStringValueFold("FREQUENCY", string(*rm.Spec.Frequency), obs.ShowOutput.Frequency, false)
		}

		// Compare triggers from SHOW output.
		specTriggers := normalizeTriggers(rm.Spec.Triggers)
		obsTriggers := buildObservedTriggers(obs.ShowOutput)
		d.CompareStringValue("TRIGGERS", specTriggers, obsTriggers, false)
	}

	return d.Result()
}

// specTriggersToClient converts spec triggers to client triggers.
func specTriggersToClient(triggers []snowplanev1alpha1.ResourceMonitorTrigger) []snowflake.ResourceMonitorTrigger {
	if len(triggers) == 0 {
		return nil
	}

	result := make([]snowflake.ResourceMonitorTrigger, len(triggers))
	for i, t := range triggers {
		result[i] = snowflake.ResourceMonitorTrigger{
			Threshold: t.Threshold,
			Action:    string(t.Action),
		}
	}

	return result
}

// normalizeTriggers creates a canonical string representation of spec triggers for comparison.
func normalizeTriggers(triggers []snowplanev1alpha1.ResourceMonitorTrigger) string {
	if len(triggers) == 0 {
		return ""
	}

	parts := make([]string, len(triggers))
	for i, t := range triggers {
		parts[i] = fmt.Sprintf("%d:%s", t.Threshold, t.Action)
	}

	sort.Strings(parts)

	return strings.Join(parts, ",")
}

// buildObservedTriggers reconstructs trigger info from SHOW output columns.
// SHOW RESOURCE MONITORS returns: notify_at (comma-separated percentages),
// suspend_at (single percentage), suspend_immediately_at (single percentage).
func buildObservedTriggers(show *snowflake.ResourceMonitorShowOutput) string {
	var parts []string

	// Parse notify_at: comma-separated percentages.
	if show.NotifyAt != "" {
		for _, pct := range strings.Split(show.NotifyAt, ",") {
			pct = strings.TrimSpace(pct)
			if pct != "" {
				if v, err := strconv.Atoi(pct); err == nil {
					parts = append(parts, fmt.Sprintf("%d:NOTIFY", v))
				}
			}
		}
	}

	// Parse suspend_at.
	if show.SuspendAt != "" {
		pct := strings.TrimSpace(show.SuspendAt)
		if v, err := strconv.Atoi(pct); err == nil {
			parts = append(parts, fmt.Sprintf("%d:SUSPEND", v))
		}
	}

	// Parse suspend_immediately_at.
	if show.SuspendImmediatelyAt != "" {
		pct := strings.TrimSpace(show.SuspendImmediatelyAt)
		if v, err := strconv.Atoi(pct); err == nil {
			parts = append(parts, fmt.Sprintf("%d:SUSPEND_IMMEDIATE", v))
		}
	}

	sort.Strings(parts)

	return strings.Join(parts, ",")
}
