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
	"github.com/hupe1980/snowplane/internal/controller/helpers"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/tracked"
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
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.ResourceMonitor, Service, *snowflake.ResourceMonitorObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewResourceMonitorClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.ResourceMonitor, Service, *snowflake.ResourceMonitorObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for ResourceMonitor resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.ResourceMonitor, Service, *snowflake.ResourceMonitorObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.ResourceMonitor, Service, *snowflake.ResourceMonitorObservation]{
		ResourceNameVal:  "resourcemonitor",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.ResourceMonitor { return &snowplanev1alpha1.ResourceMonitor{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.ResourceMonitor) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.ResourceMonitorObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.ResourceMonitor, id snowflake.AccountObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterResourceMonitorOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.ResourceMonitor, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.ResourceMonitorObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.ResourceMonitor, obs *reconciler.Observation[*snowflake.ResourceMonitorObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.ResourceMonitor, obs *reconciler.Observation[*snowflake.ResourceMonitorObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

// validateImmutableFields checks that immutable fields have not been changed.
func validateImmutableFields(_ context.Context, rm *snowplanev1alpha1.ResourceMonitor) error {
	if reconciler.ShouldSkipImmutableValidation(rm) {
		return nil
	}

	if rm.Status.ShowOutput != nil {
		if rm.Status.ShowOutput.Name != "" && !strings.EqualFold(rm.Spec.Name, rm.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", rm.Status.ShowOutput.Name, rm.Spec.Name)
		}
	}

	return nil
}

func applyObservation(rm *snowplanev1alpha1.ResourceMonitor, obs *snowflake.ResourceMonitorObservation) {
	if obs.ShowOutput != nil {
		rm.Status.FullyQualifiedName = obs.ShowOutput.Name

		rm.Status.ShowOutput = obs.ShowOutput
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

func buildAlterOptions(rm *snowplanev1alpha1.ResourceMonitor, id snowflake.AccountObjectIdentifier, obs *snowflake.ResourceMonitorObservation) snowflake.AlterResourceMonitorOptions {
	opts := snowflake.AlterResourceMonitorOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&rm.Spec, rm.Status.TrackedParameters)

	// Compare against observed values to avoid unnecessary ALTERs.
	if rm.Spec.CreditQuota != nil {
		if obs == nil || obs.ShowOutput == nil || fmt.Sprintf("%d", *rm.Spec.CreditQuota) != obs.ShowOutput.CreditQuota {
			opts.CreditQuota = rm.Spec.CreditQuota
		}
	}

	if rm.Spec.Frequency != nil {
		f := string(*rm.Spec.Frequency)
		if obs == nil || obs.ShowOutput == nil || !strings.EqualFold(f, obs.ShowOutput.Frequency) {
			opts.Frequency = &f
		}
	}

	// StartTimestamp and EndTimestamp — always send if specified.
	// Snowflake returns these in a different format than specified, making
	// reliable comparison impractical.
	opts.StartTimestamp = rm.Spec.StartTimestamp
	opts.EndTimestamp = rm.Spec.EndTimestamp

	if len(rm.Spec.NotifyUsers) > 0 {
		users := make([]string, len(rm.Spec.NotifyUsers))
		copy(users, rm.Spec.NotifyUsers)

		if obs != nil && obs.ShowOutput != nil {
			obsUsers := helpers.ParseCommaList(obs.ShowOutput.NotifyUsers)
			if !helpers.StringSlicesEqualFold(rm.Spec.NotifyUsers, obsUsers) {
				opts.NotifyUsers = &users
			}
		} else {
			opts.NotifyUsers = &users
		}
	}

	// Compare triggers against observed state.
	if len(rm.Spec.Triggers) > 0 {
		specTriggers := normalizeTriggers(rm.Spec.Triggers)
		obsTriggers := ""
		if obs != nil && obs.ShowOutput != nil {
			obsTriggers = buildObservedTriggers(obs.ShowOutput)
		}

		if specTriggers != obsTriggers {
			triggers := specTriggersToClient(rm.Spec.Triggers)
			opts.Triggers = &triggers
		}
	}

	return opts
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

		// NotifyUsers: compare spec list against comma-separated ShowOutput value.
		if len(rm.Spec.NotifyUsers) > 0 {
			obsUsers := helpers.ParseCommaList(obs.ShowOutput.NotifyUsers)
			d.CompareStringSliceFold("NOTIFY_USERS", rm.Spec.NotifyUsers, obsUsers, false)
		}
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
func buildObservedTriggers(show *snowplanev1alpha1.ResourceMonitorShowOutput) string {
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
