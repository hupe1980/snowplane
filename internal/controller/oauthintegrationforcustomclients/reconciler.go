// Package oauthintegrationforcustomclients implements the reconciler for OAuthIntegrationForCustomClients resources.
package oauthintegrationforcustomclients

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
	finalizerName = "snowplane.hupe1980.github.io/oauthintegrationforcustomclients"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake OAuth integrations for custom clients.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.OAuthIntegrationForCustomClientsObservation, error)
	Create(ctx context.Context, opts snowflake.CreateOAuthIntegrationForCustomClientsOptions) error
	Alter(ctx context.Context, opts snowflake.AlterOAuthIntegrationForCustomClientsOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new OAuthIntegrationForCustomClients reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.OAuthIntegrationForCustomClients, Service, *snowflake.OAuthIntegrationForCustomClientsObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewOAuthIntegrationForCustomClientsClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.OAuthIntegrationForCustomClients, Service, *snowflake.OAuthIntegrationForCustomClientsObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.OAuthIntegrationForCustomClients, Service, *snowflake.OAuthIntegrationForCustomClientsObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.OAuthIntegrationForCustomClients, Service, *snowflake.OAuthIntegrationForCustomClientsObservation]{
		ResourceNameVal:  "oauthintegrationforcustomclients",
		FinalizerNameVal: finalizerName,
		NewObjectFn: func() *snowplanev1alpha1.OAuthIntegrationForCustomClients {
			return &snowplanev1alpha1.OAuthIntegrationForCustomClients{}
		},
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.OAuthIntegrationForCustomClients) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.OAuthIntegrationForCustomClientsObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.OAuthIntegrationForCustomClientsObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.OAuthIntegrationForCustomClients, id snowflake.AccountObjectIdentifier) error {
			return svc.Create(ctx, buildCreateOptions(obj, id))
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterOAuthIntegrationForCustomClientsOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.OAuthIntegrationForCustomClients, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.OAuthIntegrationForCustomClientsObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.OAuthIntegrationForCustomClients, obs *reconciler.Observation[*snowflake.OAuthIntegrationForCustomClientsObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.OAuthIntegrationForCustomClients, obs *reconciler.Observation[*snowflake.OAuthIntegrationForCustomClientsObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.OAuthIntegrationForCustomClients) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ShowOutput != nil {
		if obj.Status.ShowOutput.Name != "" && !strings.EqualFold(obj.Spec.Name, obj.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", obj.Status.ShowOutput.Name, obj.Spec.Name)
		}
	}

	if obj.Status.DescribeOutput != nil {
		if v, ok := obj.Status.DescribeOutput["OAUTH_CLIENT_TYPE"]; ok && v != "" {
			if !strings.EqualFold(obj.Spec.OAuthClientType, v) {
				return fmt.Errorf("spec.oauthClientType is immutable after creation (current: %q, desired: %q)", v, obj.Spec.OAuthClientType)
			}
		}
	}

	return nil
}

func applyObservation(obj *snowplanev1alpha1.OAuthIntegrationForCustomClients, obs *snowflake.OAuthIntegrationForCustomClientsObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = obs.ShowOutput.Name
		obj.Status.ShowOutput = obs.ShowOutput
	}

	if obs.DescribeOutput != nil {
		obj.Status.DescribeOutput = obs.DescribeOutput
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.OAuthIntegrationForCustomClients, id snowflake.AccountObjectIdentifier) snowflake.CreateOAuthIntegrationForCustomClientsOptions {
	return snowflake.CreateOAuthIntegrationForCustomClientsOptions{
		Name:                        id,
		Enabled:                     obj.Spec.Enabled,
		OAuthClientType:             obj.Spec.OAuthClientType,
		OAuthRedirectURI:            obj.Spec.OAuthRedirectURI,
		OAuthAllowNonTLSRedirectURI: obj.Spec.OAuthAllowNonTLSRedirectURI,
		OAuthEnforcePKCE:            obj.Spec.OAuthEnforcePKCE,
		OAuthUseSecondaryRoles:      obj.Spec.OAuthUseSecondaryRoles,
		PreAuthorizedRolesList:      obj.Spec.PreAuthorizedRolesList,
		BlockedRolesList:            obj.Spec.BlockedRolesList,
		OAuthIssueRefreshTokens:     obj.Spec.OAuthIssueRefreshTokens,
		OAuthRefreshTokenValidity:   obj.Spec.OAuthRefreshTokenValidity,
		NetworkPolicy:               obj.Spec.NetworkPolicy,
		OAuthClientRSAPublicKey:     obj.Spec.OAuthClientRSAPublicKey,
		OAuthClientRSAPublicKey2:    obj.Spec.OAuthClientRSAPublicKey2,
		Comment:                     obj.Spec.Comment,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.OAuthIntegrationForCustomClients, id snowflake.AccountObjectIdentifier, obs *snowflake.OAuthIntegrationForCustomClientsObservation) snowflake.AlterOAuthIntegrationForCustomClientsOptions {
	opts := snowflake.AlterOAuthIntegrationForCustomClientsOptions{
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

		if obj.Spec.OAuthRedirectURI != helpers.DescribeValue(dm, "OAUTH_REDIRECT_URI") {
			opts.OAuthRedirectURI = &obj.Spec.OAuthRedirectURI
		}

		if obj.Spec.OAuthAllowNonTLSRedirectURI != nil && !strings.EqualFold(helpers.BoolToString(*obj.Spec.OAuthAllowNonTLSRedirectURI), helpers.DescribeValue(dm, "OAUTH_ALLOW_NON_TLS_REDIRECT_URI")) {
			opts.OAuthAllowNonTLSRedirectURI = obj.Spec.OAuthAllowNonTLSRedirectURI
		}

		if obj.Spec.OAuthEnforcePKCE != nil && !strings.EqualFold(helpers.BoolToString(*obj.Spec.OAuthEnforcePKCE), helpers.DescribeValue(dm, "OAUTH_ENFORCE_PKCE")) {
			opts.OAuthEnforcePKCE = obj.Spec.OAuthEnforcePKCE
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

		if obj.Spec.NetworkPolicy != nil && *obj.Spec.NetworkPolicy != helpers.DescribeValue(dm, "NETWORK_POLICY") {
			opts.NetworkPolicy = obj.Spec.NetworkPolicy
		}

		if obj.Spec.OAuthClientRSAPublicKey != nil && *obj.Spec.OAuthClientRSAPublicKey != helpers.DescribeValue(dm, "OAUTH_CLIENT_RSA_PUBLIC_KEY") {
			opts.OAuthClientRSAPublicKey = obj.Spec.OAuthClientRSAPublicKey
		}

		if obj.Spec.OAuthClientRSAPublicKey2 != nil && *obj.Spec.OAuthClientRSAPublicKey2 != helpers.DescribeValue(dm, "OAUTH_CLIENT_RSA_PUBLIC_KEY_2") {
			opts.OAuthClientRSAPublicKey2 = obj.Spec.OAuthClientRSAPublicKey2
		}

		actualPreAuth := helpers.ParseCommaListFromMap(dm, "PRE_AUTHORIZED_ROLES_LIST")
		if !helpers.StringSlicesEqualFold(obj.Spec.PreAuthorizedRolesList, actualPreAuth) {
			list := make([]string, len(obj.Spec.PreAuthorizedRolesList))
			copy(list, obj.Spec.PreAuthorizedRolesList)
			opts.PreAuthorizedRolesList = &list
		}

		actualBlocked := helpers.ParseCommaListFromMap(dm, "BLOCKED_ROLES_LIST")
		if !helpers.StringSlicesEqualFold(obj.Spec.BlockedRolesList, actualBlocked) {
			list := make([]string, len(obj.Spec.BlockedRolesList))
			copy(list, obj.Spec.BlockedRolesList)
			opts.BlockedRolesList = &list
		}
	} else {
		opts.OAuthRedirectURI = &obj.Spec.OAuthRedirectURI
		opts.OAuthAllowNonTLSRedirectURI = obj.Spec.OAuthAllowNonTLSRedirectURI
		opts.OAuthEnforcePKCE = obj.Spec.OAuthEnforcePKCE
		opts.OAuthUseSecondaryRoles = obj.Spec.OAuthUseSecondaryRoles
		opts.OAuthIssueRefreshTokens = obj.Spec.OAuthIssueRefreshTokens
		opts.OAuthRefreshTokenValidity = obj.Spec.OAuthRefreshTokenValidity
		opts.NetworkPolicy = obj.Spec.NetworkPolicy
		opts.OAuthClientRSAPublicKey = obj.Spec.OAuthClientRSAPublicKey
		opts.OAuthClientRSAPublicKey2 = obj.Spec.OAuthClientRSAPublicKey2

		if len(obj.Spec.PreAuthorizedRolesList) > 0 {
			list := make([]string, len(obj.Spec.PreAuthorizedRolesList))
			copy(list, obj.Spec.PreAuthorizedRolesList)
			opts.PreAuthorizedRolesList = &list
		}

		if len(obj.Spec.BlockedRolesList) > 0 {
			list := make([]string, len(obj.Spec.BlockedRolesList))
			copy(list, obj.Spec.BlockedRolesList)
			opts.BlockedRolesList = &list
		}
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.OAuthIntegrationForCustomClients, obs *snowflake.OAuthIntegrationForCustomClientsObservation) *drift.Result {
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
		d.CompareStringValue("OAUTH_CLIENT_TYPE", obj.Spec.OAuthClientType, helpers.DescribeValue(obs.DescribeOutput, "OAUTH_CLIENT_TYPE"), true)
		d.CompareStringValue("OAUTH_REDIRECT_URI", obj.Spec.OAuthRedirectURI, helpers.DescribeValue(obs.DescribeOutput, "OAUTH_REDIRECT_URI"), false)
		d.CompareStringValueFold("OAUTH_USE_SECONDARY_ROLES", helpers.StringValueOrEmpty(obj.Spec.OAuthUseSecondaryRoles), helpers.DescribeValue(obs.DescribeOutput, "OAUTH_USE_SECONDARY_ROLES"), false)
		d.CompareStringValue("NETWORK_POLICY", helpers.StringValueOrEmpty(obj.Spec.NetworkPolicy), helpers.DescribeValue(obs.DescribeOutput, "NETWORK_POLICY"), false)

		if obj.Spec.OAuthAllowNonTLSRedirectURI != nil {
			obsVal := helpers.DescribeValue(obs.DescribeOutput, "OAUTH_ALLOW_NON_TLS_REDIRECT_URI")
			d.CompareStringValueFold("OAUTH_ALLOW_NON_TLS_REDIRECT_URI", helpers.BoolToString(*obj.Spec.OAuthAllowNonTLSRedirectURI), obsVal, false)
		}

		if obj.Spec.OAuthEnforcePKCE != nil {
			obsVal := helpers.DescribeValue(obs.DescribeOutput, "OAUTH_ENFORCE_PKCE")
			d.CompareStringValueFold("OAUTH_ENFORCE_PKCE", helpers.BoolToString(*obj.Spec.OAuthEnforcePKCE), obsVal, false)
		}

		if obj.Spec.OAuthIssueRefreshTokens != nil {
			obsVal := helpers.DescribeValue(obs.DescribeOutput, "OAUTH_ISSUE_REFRESH_TOKENS")
			d.CompareStringValueFold("OAUTH_ISSUE_REFRESH_TOKENS", helpers.BoolToString(*obj.Spec.OAuthIssueRefreshTokens), obsVal, false)
		}

		if obj.Spec.OAuthRefreshTokenValidity != nil {
			obsVal := helpers.DescribeValue(obs.DescribeOutput, "OAUTH_REFRESH_TOKEN_VALIDITY")
			d.CompareStringValue("OAUTH_REFRESH_TOKEN_VALIDITY", fmt.Sprintf("%d", *obj.Spec.OAuthRefreshTokenValidity), obsVal, false)
		}

		helpers.CompareListFromDescribeMap(d, "PRE_AUTHORIZED_ROLES_LIST", obj.Spec.PreAuthorizedRolesList, obs.DescribeOutput, false)
		helpers.CompareListFromDescribeMap(d, "BLOCKED_ROLES_LIST", obj.Spec.BlockedRolesList, obs.DescribeOutput, false)

		if obj.Spec.OAuthClientRSAPublicKey != nil {
			d.CompareStringValue("OAUTH_CLIENT_RSA_PUBLIC_KEY", *obj.Spec.OAuthClientRSAPublicKey, helpers.DescribeValue(obs.DescribeOutput, "OAUTH_CLIENT_RSA_PUBLIC_KEY"), false)
		}

		if obj.Spec.OAuthClientRSAPublicKey2 != nil {
			d.CompareStringValue("OAUTH_CLIENT_RSA_PUBLIC_KEY_2", *obj.Spec.OAuthClientRSAPublicKey2, helpers.DescribeValue(obs.DescribeOutput, "OAUTH_CLIENT_RSA_PUBLIC_KEY_2"), false)
		}
	}

	return d.Result()
}
