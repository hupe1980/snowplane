// Package oauthintegrationforpartnerapplications implements the reconciler for OAuthIntegrationForPartnerApplications resources.
package oauthintegrationforpartnerapplications

import (
	"context"
	"fmt"
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
	finalizerName = "snowplane.hupe1980.github.io/oauthintegrationforpartnerapplications"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.OAuthIntegrationForPartnerApplicationsObservation, error)
	Create(ctx context.Context, opts snowflake.CreateOAuthIntegrationForPartnerApplicationsOptions) error
	Alter(ctx context.Context, opts snowflake.AlterOAuthIntegrationForPartnerApplicationsOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new OAuthIntegrationForPartnerApplications reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.OAuthIntegrationForPartnerApplications, Service, *snowflake.OAuthIntegrationForPartnerApplicationsObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewOAuthIntegrationForPartnerApplicationsClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.OAuthIntegrationForPartnerApplications, Service, *snowflake.OAuthIntegrationForPartnerApplicationsObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.OAuthIntegrationForPartnerApplications, Service, *snowflake.OAuthIntegrationForPartnerApplicationsObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.OAuthIntegrationForPartnerApplications, Service, *snowflake.OAuthIntegrationForPartnerApplicationsObservation]{
		ResourceNameVal:  "oauthintegrationforpartnerapplications",
		FinalizerNameVal: finalizerName,
		NewObjectFn: func() *snowplanev1alpha1.OAuthIntegrationForPartnerApplications {
			return &snowplanev1alpha1.OAuthIntegrationForPartnerApplications{}
		},
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.OAuthIntegrationForPartnerApplications) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.OAuthIntegrationForPartnerApplicationsObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.OAuthIntegrationForPartnerApplicationsObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.OAuthIntegrationForPartnerApplications, id snowflake.AccountObjectIdentifier) error {
			return svc.Create(ctx, buildCreateOptions(obj, id))
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterOAuthIntegrationForPartnerApplicationsOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.OAuthIntegrationForPartnerApplications, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.OAuthIntegrationForPartnerApplicationsObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.OAuthIntegrationForPartnerApplications, obs *reconciler.Observation[*snowflake.OAuthIntegrationForPartnerApplicationsObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.OAuthIntegrationForPartnerApplications, obs *reconciler.Observation[*snowflake.OAuthIntegrationForPartnerApplicationsObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.OAuthIntegrationForPartnerApplications) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ShowOutput != nil {
		if obj.Status.ShowOutput.Name != "" && !strings.EqualFold(obj.Spec.Name, obj.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", obj.Status.ShowOutput.Name, obj.Spec.Name)
		}
	}

	if obj.Status.DescribeOutput != nil {
		if current := helpers.DescribeValue(obj.Status.DescribeOutput, "OAUTH_CLIENT"); current != "" && !strings.EqualFold(obj.Spec.OAuthClient, current) {
			return fmt.Errorf("spec.oauthClient is immutable after creation (current: %q, desired: %q)", current, obj.Spec.OAuthClient)
		}
	}

	return nil
}

func applyObservation(obj *snowplanev1alpha1.OAuthIntegrationForPartnerApplications, obs *snowflake.OAuthIntegrationForPartnerApplicationsObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = obs.ShowOutput.Name
		obj.Status.ShowOutput = obs.ShowOutput
	}

	if obs.DescribeOutput != nil {
		obj.Status.DescribeOutput = obs.DescribeOutput
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.OAuthIntegrationForPartnerApplications, id snowflake.AccountObjectIdentifier) snowflake.CreateOAuthIntegrationForPartnerApplicationsOptions {
	return snowflake.CreateOAuthIntegrationForPartnerApplicationsOptions{
		Name:                      id,
		Enabled:                   obj.Spec.Enabled,
		OAuthClient:               obj.Spec.OAuthClient,
		OAuthRedirectURI:          obj.Spec.OAuthRedirectURI,
		OAuthUseSecondaryRoles:    obj.Spec.OAuthUseSecondaryRoles,
		OAuthIssueRefreshTokens:   obj.Spec.OAuthIssueRefreshTokens,
		OAuthRefreshTokenValidity: obj.Spec.OAuthRefreshTokenValidity,
		BlockedRolesList:          obj.Spec.BlockedRolesList,
		Comment:                   obj.Spec.Comment,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.OAuthIntegrationForPartnerApplications, id snowflake.AccountObjectIdentifier, obs *snowflake.OAuthIntegrationForPartnerApplicationsObservation) snowflake.AlterOAuthIntegrationForPartnerApplicationsOptions {
	opts := snowflake.AlterOAuthIntegrationForPartnerApplicationsOptions{
		Name: id,
	}
	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	if obj.Spec.Enabled != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Enabled != obs.ShowOutput.Enabled {
			opts.Enabled = obj.Spec.Enabled
		}
	}

	if obj.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	if obs != nil && obs.DescribeOutput != nil {
		dm := obs.DescribeOutput

		if obj.Spec.OAuthRedirectURI != nil && *obj.Spec.OAuthRedirectURI != helpers.DescribeValue(dm, "OAUTH_REDIRECT_URI") {
			opts.OAuthRedirectURI = obj.Spec.OAuthRedirectURI
		}

		if obj.Spec.OAuthUseSecondaryRoles != nil && !strings.EqualFold(*obj.Spec.OAuthUseSecondaryRoles, helpers.DescribeValue(dm, "OAUTH_USE_SECONDARY_ROLES")) {
			opts.OAuthUseSecondaryRoles = obj.Spec.OAuthUseSecondaryRoles
		}

		if obj.Spec.OAuthIssueRefreshTokens != nil && !strings.EqualFold(helpers.BoolToString(*obj.Spec.OAuthIssueRefreshTokens), helpers.DescribeValue(dm, "OAUTH_ISSUE_REFRESH_TOKENS")) {
			opts.OAuthIssueRefreshTokens = obj.Spec.OAuthIssueRefreshTokens
		}

		if obj.Spec.OAuthRefreshTokenValidity != nil && fmt.Sprintf("%d", *obj.Spec.OAuthRefreshTokenValidity) != helpers.DescribeValue(dm, "OAUTH_REFRESH_TOKEN_VALIDITY") {
			opts.OAuthRefreshTokenValidity = obj.Spec.OAuthRefreshTokenValidity
		}

		desired := helpers.ParseCommaListFromMap(dm, "BLOCKED_ROLES_LIST")
		if !helpers.StringSlicesEqualFold(obj.Spec.BlockedRolesList, desired) {
			list := make([]string, len(obj.Spec.BlockedRolesList))
			copy(list, obj.Spec.BlockedRolesList)
			opts.BlockedRolesList = &list
		}
	} else {
		opts.OAuthRedirectURI = obj.Spec.OAuthRedirectURI
		opts.OAuthUseSecondaryRoles = obj.Spec.OAuthUseSecondaryRoles
		opts.OAuthIssueRefreshTokens = obj.Spec.OAuthIssueRefreshTokens
		opts.OAuthRefreshTokenValidity = obj.Spec.OAuthRefreshTokenValidity

		if len(obj.Spec.BlockedRolesList) > 0 {
			list := make([]string, len(obj.Spec.BlockedRolesList))
			copy(list, obj.Spec.BlockedRolesList)
			opts.BlockedRolesList = &list
		}
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.OAuthIntegrationForPartnerApplications, obs *snowflake.OAuthIntegrationForPartnerApplicationsObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)

		if obj.Spec.Enabled != nil {
			obsEnabled := obs.ShowOutput.Enabled
			d.CompareBool("ENABLED", obj.Spec.Enabled, &obsEnabled, false)
		}
	}

	if obs.DescribeOutput != nil {
		d.CompareStringValueFold("OAUTH_CLIENT", obj.Spec.OAuthClient, helpers.DescribeValue(obs.DescribeOutput, "OAUTH_CLIENT"), true)
		d.CompareStringValueFold("OAUTH_USE_SECONDARY_ROLES", helpers.StringValueOrEmpty(obj.Spec.OAuthUseSecondaryRoles), helpers.DescribeValue(obs.DescribeOutput, "OAUTH_USE_SECONDARY_ROLES"), false)

		if obj.Spec.OAuthRedirectURI != nil {
			d.CompareStringValue("OAUTH_REDIRECT_URI", *obj.Spec.OAuthRedirectURI, helpers.DescribeValue(obs.DescribeOutput, "OAUTH_REDIRECT_URI"), false)
		}

		if obj.Spec.OAuthIssueRefreshTokens != nil {
			obsVal := helpers.DescribeValue(obs.DescribeOutput, "OAUTH_ISSUE_REFRESH_TOKENS")
			d.CompareStringValueFold("OAUTH_ISSUE_REFRESH_TOKENS", helpers.BoolToString(*obj.Spec.OAuthIssueRefreshTokens), obsVal, false)
		}

		if obj.Spec.OAuthRefreshTokenValidity != nil {
			obsVal := helpers.DescribeValue(obs.DescribeOutput, "OAUTH_REFRESH_TOKEN_VALIDITY")
			d.CompareStringValue("OAUTH_REFRESH_TOKEN_VALIDITY", fmt.Sprintf("%d", *obj.Spec.OAuthRefreshTokenValidity), obsVal, false)
		}

		helpers.CompareListFromDescribeMap(d, "BLOCKED_ROLES_LIST", obj.Spec.BlockedRolesList, obs.DescribeOutput, false)
	}

	return d.Result()
}
