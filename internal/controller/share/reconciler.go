// Package share implements the reconciler for Share resources.
package share

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
	finalizerName = "snowplane.hupe1980.github.io/share"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake shares.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ShareObservation, error)
	Create(ctx context.Context, opts snowflake.CreateShareOptions) error
	Alter(ctx context.Context, opts snowflake.AlterShareOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Share reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Share, Service, *snowflake.ShareObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewShareClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.Share, Service, *snowflake.ShareObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for Share resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.Share, Service, *snowflake.ShareObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.Share, Service, *snowflake.ShareObservation]{
		ResourceNameVal:  "share",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.Share { return &snowplanev1alpha1.Share{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.Share) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.ShareObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.ShareObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.Share, id snowflake.AccountObjectIdentifier) error {
			return svc.Create(ctx, buildCreateOptions(obj, id))
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterShareOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.Share, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.ShareObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.Share, obs *reconciler.Observation[*snowflake.ShareObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.Share, obs *reconciler.Observation[*snowflake.ShareObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

func validateImmutableFields(_ context.Context, share *snowplanev1alpha1.Share) error {
	if reconciler.ShouldSkipImmutableValidation(share) {
		return nil
	}

	if share.Status.ShowOutput != nil {
		if share.Status.ShowOutput.Name != "" && !strings.EqualFold(share.Spec.Name, share.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", share.Status.ShowOutput.Name, share.Spec.Name)
		}
	}

	return nil
}

func applyObservation(share *snowplanev1alpha1.Share, obs *snowflake.ShareObservation) {
	if obs.ShowOutput != nil {
		share.Status.FullyQualifiedName = snowflake.NewAccountObjectIdentifier(obs.ShowOutput.Name).FullyQualifiedName()
		share.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(share *snowplanev1alpha1.Share, id snowflake.AccountObjectIdentifier) snowflake.CreateShareOptions {
	return snowflake.CreateShareOptions{
		Name:    id,
		Comment: share.Spec.Comment,
	}
}

func buildAlterOptions(share *snowplanev1alpha1.Share, id snowflake.AccountObjectIdentifier, obs *snowflake.ShareObservation) snowflake.AlterShareOptions {
	opts := snowflake.AlterShareOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&share.Spec, share.Status.TrackedParameters)

	if share.Spec.Comment != nil {
		if obs.ShowOutput == nil || *share.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = share.Spec.Comment
		}
	}

	// Diff accounts: spec.accounts vs observed "To" field.
	if obs.ShowOutput != nil {
		desiredAccounts := normalizeAccounts(share.Spec.Accounts)
		currentAccounts := normalizeAccounts(parseAccountsFromTo(obs.ShowOutput.To))

		for _, a := range desiredAccounts {
			if !containsAccount(currentAccounts, a) {
				opts.AddAccounts = append(opts.AddAccounts, a)
			}
		}

		for _, a := range currentAccounts {
			if !containsAccount(desiredAccounts, a) {
				opts.RemAccounts = append(opts.RemAccounts, a)
			}
		}
	}

	return opts
}

// parseAccountsFromTo splits the comma-separated "To" field from SHOW SHARES.
func parseAccountsFromTo(to string) []string {
	if to == "" {
		return nil
	}

	parts := strings.Split(to, ",")
	accounts := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			accounts = append(accounts, p)
		}
	}

	return accounts
}

// normalizeAccounts upper-cases and sorts account names for deterministic comparison.
func normalizeAccounts(accounts []string) []string {
	out := make([]string, len(accounts))
	for i, a := range accounts {
		out[i] = strings.ToUpper(strings.TrimSpace(a))
	}

	sort.Strings(out)

	return out
}

// containsAccount checks if accounts contains the given account (case-insensitive).
func containsAccount(accounts []string, account string) bool {
	for _, a := range accounts {
		if strings.EqualFold(a, account) {
			return true
		}
	}

	return false
}

func detectDrift(share *snowplanev1alpha1.Share, obs *snowflake.ShareObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", share.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", share.Spec.Comment, obs.ShowOutput.Comment, false)

		// Check account drift.
		desiredAccounts := normalizeAccounts(share.Spec.Accounts)
		currentAccounts := normalizeAccounts(parseAccountsFromTo(obs.ShowOutput.To))

		d.CompareStringValue("ACCOUNTS", strings.Join(desiredAccounts, ","), strings.Join(currentAccounts, ","), false)
	}

	return d.Result()
}
