// Package externaloauthintegration implements the reconciler for ExternalOAuthIntegration resources.
package externaloauthintegration

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
	finalizerName = "snowplane.hupe1980.github.io/externaloauthintegration"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake External OAuth integrations.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ExternalOAuthIntegrationObservation, error)
	Create(ctx context.Context, opts snowflake.CreateExternalOAuthIntegrationOptions) error
	Alter(ctx context.Context, opts snowflake.AlterExternalOAuthIntegrationOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new ExternalOAuthIntegration reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.ExternalOAuthIntegration, Service, *snowflake.ExternalOAuthIntegrationObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewExternalOAuthIntegrationClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.ExternalOAuthIntegration, Service, *snowflake.ExternalOAuthIntegrationObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for ExternalOAuthIntegration resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.ExternalOAuthIntegration, Service, *snowflake.ExternalOAuthIntegrationObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.ExternalOAuthIntegration, Service, *snowflake.ExternalOAuthIntegrationObservation]{
		ResourceNameVal:  "externaloauthintegration",
		FinalizerNameVal: finalizerName,
		NewObjectFn: func() *snowplanev1alpha1.ExternalOAuthIntegration {
			return &snowplanev1alpha1.ExternalOAuthIntegration{}
		},
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.ExternalOAuthIntegration) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.ExternalOAuthIntegrationObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.ExternalOAuthIntegrationObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.ExternalOAuthIntegration, id snowflake.AccountObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterExternalOAuthIntegrationOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.ExternalOAuthIntegration, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.ExternalOAuthIntegrationObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.ExternalOAuthIntegration, obs *reconciler.Observation[*snowflake.ExternalOAuthIntegrationObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.ExternalOAuthIntegration, obs *reconciler.Observation[*snowflake.ExternalOAuthIntegrationObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

// validateImmutableFields checks that immutable fields have not changed.
func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.ExternalOAuthIntegration) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ShowOutput != nil {
		if obj.Status.ShowOutput.Name != "" && !strings.EqualFold(obj.Spec.Name, obj.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", obj.Status.ShowOutput.Name, obj.Spec.Name)
		}
	}

	return nil
}

func applyObservation(obj *snowplanev1alpha1.ExternalOAuthIntegration, obs *snowflake.ExternalOAuthIntegrationObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = obs.ShowOutput.Name
		obj.Status.ShowOutput = obs.ShowOutput
	}

	if obs.DescribeOutput != nil {
		obj.Status.DescribeOutput = obs.DescribeOutput
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.ExternalOAuthIntegration, id snowflake.AccountObjectIdentifier) snowflake.CreateExternalOAuthIntegrationOptions {
	return snowflake.CreateExternalOAuthIntegrationOptions{
		Name:                          id,
		Enabled:                       obj.Spec.Enabled,
		ExternalOAuthType:             obj.Spec.ExternalOAuthType,
		Issuer:                        obj.Spec.Issuer,
		TokenUserMappingClaim:         obj.Spec.TokenUserMappingClaim,
		SnowflakeUserMappingAttribute: obj.Spec.SnowflakeUserMappingAttribute,
		JWSKeysURL:                    obj.Spec.JWSKeysURL,
		AudienceList:                  obj.Spec.AudienceList,
		AllowedRoles:                  obj.Spec.AllowedRoles,
		BlockedRoles:                  obj.Spec.BlockedRoles,
		AnyRoleMode:                   obj.Spec.AnyRoleMode,
		ScopeDelimiter:                obj.Spec.ScopeDelimiter,
		NetworkPolicy:                 obj.Spec.NetworkPolicy,
		Comment:                       obj.Spec.Comment,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.ExternalOAuthIntegration, id snowflake.AccountObjectIdentifier, obs *snowflake.ExternalOAuthIntegrationObservation) snowflake.AlterExternalOAuthIntegrationOptions {
	opts := snowflake.AlterExternalOAuthIntegrationOptions{
		Name: id,
	}
	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	// Compare Enabled against observed value.
	if obj.Spec.Enabled != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Enabled != obs.ShowOutput.Enabled {
			opts.Enabled = obj.Spec.Enabled
		}
	}

	// Comment - only set if changed.
	if obj.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	// TokenUserMappingClaim is always sent (required, cannot be unset).
	// But diff-check it when observation is available to avoid redundant ALTER.
	if obs != nil && obs.DescribeOutput != nil {
		dm := obs.DescribeOutput

		if obj.Spec.TokenUserMappingClaim != helpers.DescribeValue(dm, "EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM") {
			claim := obj.Spec.TokenUserMappingClaim
			opts.TokenUserMappingClaim = &claim
		}

		// JWSKeysURL.
		if obj.Spec.JWSKeysURL != nil && *obj.Spec.JWSKeysURL != helpers.DescribeValue(dm, "EXTERNAL_OAUTH_JWS_KEYS_URL") {
			opts.JWSKeysURL = obj.Spec.JWSKeysURL
		}

		// List fields — diff-check against describe output.
		actualAudience := helpers.ParseCommaListFromMap(dm, "EXTERNAL_OAUTH_AUDIENCE_LIST")
		if !helpers.StringSlicesEqualFold(obj.Spec.AudienceList, actualAudience) {
			if len(obj.Spec.AudienceList) > 0 {
				list := make([]string, len(obj.Spec.AudienceList))
				copy(list, obj.Spec.AudienceList)
				opts.AudienceList = &list
			}
		}

		actualAllowed := helpers.ParseCommaListFromMap(dm, "EXTERNAL_OAUTH_ALLOWED_ROLES_LIST")
		if !helpers.StringSlicesEqualFold(obj.Spec.AllowedRoles, actualAllowed) {
			if len(obj.Spec.AllowedRoles) > 0 {
				list := make([]string, len(obj.Spec.AllowedRoles))
				copy(list, obj.Spec.AllowedRoles)
				opts.AllowedRoles = &list
			}
		}

		actualBlocked := helpers.ParseCommaListFromMap(dm, "EXTERNAL_OAUTH_BLOCKED_ROLES_LIST")
		if !helpers.StringSlicesEqualFold(obj.Spec.BlockedRoles, actualBlocked) {
			if len(obj.Spec.BlockedRoles) > 0 {
				list := make([]string, len(obj.Spec.BlockedRoles))
				copy(list, obj.Spec.BlockedRoles)
				opts.BlockedRoles = &list
			}
		}

		// Enum/scalar optional fields — diff-check.
		if obj.Spec.AnyRoleMode != nil && !strings.EqualFold(*obj.Spec.AnyRoleMode, helpers.DescribeValue(dm, "EXTERNAL_OAUTH_ANY_ROLE_MODE")) {
			opts.AnyRoleMode = obj.Spec.AnyRoleMode
		}

		if obj.Spec.ScopeDelimiter != nil && *obj.Spec.ScopeDelimiter != helpers.DescribeValue(dm, "EXTERNAL_OAUTH_SCOPE_DELIMITER") {
			opts.ScopeDelimiter = obj.Spec.ScopeDelimiter
		}

		if obj.Spec.NetworkPolicy != nil && *obj.Spec.NetworkPolicy != helpers.DescribeValue(dm, "NETWORK_POLICY") {
			opts.NetworkPolicy = obj.Spec.NetworkPolicy
		}
	} else {
		// No observation — send everything unconditionally.
		claim := obj.Spec.TokenUserMappingClaim
		opts.TokenUserMappingClaim = &claim

		if obj.Spec.JWSKeysURL != nil {
			opts.JWSKeysURL = obj.Spec.JWSKeysURL
		}

		if len(obj.Spec.AudienceList) > 0 {
			list := make([]string, len(obj.Spec.AudienceList))
			copy(list, obj.Spec.AudienceList)
			opts.AudienceList = &list
		}

		if len(obj.Spec.AllowedRoles) > 0 {
			list := make([]string, len(obj.Spec.AllowedRoles))
			copy(list, obj.Spec.AllowedRoles)
			opts.AllowedRoles = &list
		}

		if len(obj.Spec.BlockedRoles) > 0 {
			list := make([]string, len(obj.Spec.BlockedRoles))
			copy(list, obj.Spec.BlockedRoles)
			opts.BlockedRoles = &list
		}

		opts.AnyRoleMode = obj.Spec.AnyRoleMode
		opts.ScopeDelimiter = obj.Spec.ScopeDelimiter
		opts.NetworkPolicy = obj.Spec.NetworkPolicy
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.ExternalOAuthIntegration, obs *snowflake.ExternalOAuthIntegrationObservation) *drift.Result {
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
		d.CompareStringValue("EXTERNAL_OAUTH_JWS_KEYS_URL", helpers.StringValueOrEmpty(obj.Spec.JWSKeysURL), helpers.DescribeValue(obs.DescribeOutput, "EXTERNAL_OAUTH_JWS_KEYS_URL"), false)
		d.CompareStringValueFold("EXTERNAL_OAUTH_ANY_ROLE_MODE", helpers.StringValueOrEmpty(obj.Spec.AnyRoleMode), helpers.DescribeValue(obs.DescribeOutput, "EXTERNAL_OAUTH_ANY_ROLE_MODE"), false)

		helpers.CompareListFromDescribeMap(d, "EXTERNAL_OAUTH_AUDIENCE_LIST", obj.Spec.AudienceList, obs.DescribeOutput, false)
		helpers.CompareListFromDescribeMap(d, "EXTERNAL_OAUTH_ALLOWED_ROLES_LIST", obj.Spec.AllowedRoles, obs.DescribeOutput, false)
		helpers.CompareListFromDescribeMap(d, "EXTERNAL_OAUTH_BLOCKED_ROLES_LIST", obj.Spec.BlockedRoles, obs.DescribeOutput, false)

		d.CompareStringValue("NETWORK_POLICY", helpers.StringValueOrEmpty(obj.Spec.NetworkPolicy), helpers.DescribeValue(obs.DescribeOutput, "NETWORK_POLICY"), false)
		d.CompareStringValue("EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM", obj.Spec.TokenUserMappingClaim, helpers.DescribeValue(obs.DescribeOutput, "EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM"), false)
		d.CompareString("EXTERNAL_OAUTH_SCOPE_DELIMITER", obj.Spec.ScopeDelimiter, helpers.DescribeValue(obs.DescribeOutput, "EXTERNAL_OAUTH_SCOPE_DELIMITER"), false)
	}

	return d.Result()
}
