// Package securityintegration implements the reconciler for SecurityIntegration resources.
package securityintegration

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
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/securityintegration"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake security integrations.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecurityIntegrationObservation, error)
	Create(ctx context.Context, opts snowflake.CreateSecurityIntegrationOptions) error
	Alter(ctx context.Context, opts snowflake.AlterSecurityIntegrationOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new SecurityIntegration reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.SecurityIntegration, Service, *snowflake.SecurityIntegrationObservation] {
	a := &adapter{newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.SecurityIntegration, Service, *snowflake.SecurityIntegrationObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.SecurityIntegration, Service, *snowflake.SecurityIntegrationObservation] {
	a := &adapter{newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.SecurityIntegration, Service, *snowflake.SecurityIntegrationObservation]{
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

	return snowflake.NewSecurityIntegrationClient(sfC), cleanup, nil
}

func applyObservation(si *snowplanev1alpha1.SecurityIntegration, obs *snowflake.SecurityIntegrationObservation) {
	if obs.ShowOutput != nil {
		si.Status.FullyQualifiedName = obs.ShowOutput.Name

		si.Status.ShowOutput = &snowplanev1alpha1.SecurityIntegrationShowOutput{
			CreatedOn: obs.ShowOutput.CreatedOn,
			Name:      obs.ShowOutput.Name,
			Type:      obs.ShowOutput.Type,
			Category:  obs.ShowOutput.Category,
			Enabled:   obs.ShowOutput.Enabled,
			Comment:   obs.ShowOutput.Comment,
		}
	}

	if obs.DescribeOutput != nil {
		si.Status.DescribeOutput = obs.DescribeOutput
	}
}

func buildCreateOptions(si *snowplanev1alpha1.SecurityIntegration, id snowflake.AccountObjectIdentifier) snowflake.CreateSecurityIntegrationOptions {
	opts := snowflake.CreateSecurityIntegrationOptions{
		Name:    id,
		Type:    string(si.Spec.Type),
		Enabled: si.Spec.Enabled,
		Comment: si.Spec.Comment,
	}

	switch si.Spec.Type {
	case snowplanev1alpha1.SecurityIntegrationTypeExternalOAuth:
		if c := si.Spec.ExternalOAuth; c != nil {
			opts.ExternalOAuthType = &c.Type
			opts.ExternalOAuthIssuer = &c.Issuer
			opts.ExternalOAuthTokenUserMappingClaim = &c.TokenUserMappingClaim
			opts.ExternalOAuthSnowflakeUserMappingAttr = &c.SnowflakeUserMappingAttribute
			opts.ExternalOAuthJWSKeysURL = c.JWSKeysURL
			opts.ExternalOAuthAudienceList = c.AudienceList
			opts.ExternalOAuthAllowedRoles = c.AllowedRoles
			opts.ExternalOAuthBlockedRoles = c.BlockedRoles
			opts.ExternalOAuthAnyRoleMode = c.AnyRoleMode
			opts.ExternalOAuthScopeDelimiter = c.ScopeDelimiter
			opts.ExternalOAuthNetworkPolicy = c.NetworkPolicy
		}
	case snowplanev1alpha1.SecurityIntegrationTypeSAML2:
		if c := si.Spec.SAML2; c != nil {
			opts.SAML2Issuer = &c.Issuer
			opts.SAML2SSOURL = &c.SSOURL
			opts.SAML2Provider = &c.Provider
			opts.SAML2X509Cert = &c.X509Cert
			opts.SAML2AllowedEmailPatterns = c.AllowedEmailPatterns
			opts.SAML2AllowedUserDomains = c.AllowedUserDomains
			opts.SAML2SPInitiatedLoginLabel = c.SPInitiatedLoginPageLabel
			opts.SAML2EnableSPInitiated = c.EnableSPInitiated
			opts.SAML2ForceAuthn = c.ForceAuthn
			opts.SAML2RequestedNameIDFormat = c.RequestedNameIDFormat
			opts.SAML2PostLogoutRedirectURL = c.PostLogoutRedirectURL
		}
	case snowplanev1alpha1.SecurityIntegrationTypeSCIM:
		if c := si.Spec.SCIM; c != nil {
			opts.SCIMClient = &c.SCIMClient
			opts.SCIMRunAsRole = &c.RunAsRole
			if c.NetworkPolicy != nil {
				opts.SCIMNetworkPolicy = c.NetworkPolicy
			}
			opts.SCIMSyncPassword = c.SyncPassword
		}
	case snowplanev1alpha1.SecurityIntegrationTypeAPIAuthentication:
		if c := si.Spec.APIAuthentication; c != nil {
			opts.OAuthClientID = &c.OAuthClientID
			opts.OAuthClientSecret = &c.OAuthClientSecret
			opts.OAuthTokenEndpoint = &c.OAuthTokenEndpoint
			opts.OAuthAllowedScopes = c.OAuthAllowedScopes
			opts.OAuthGrantType = c.OAuthGrantType
		}
	}

	return opts
}

func buildAlterOptions(si *snowplanev1alpha1.SecurityIntegration, id snowflake.AccountObjectIdentifier, obs *snowflake.SecurityIntegrationObservation) snowflake.AlterSecurityIntegrationOptions {
	opts := snowflake.AlterSecurityIntegrationOptions{
		Name: id,
		Type: string(si.Spec.Type),
	}
	opts.UnsetFields = tracked.ComputeUnset(&si.Spec, si.Status.TrackedParameters)

	// Compare Enabled and Comment against observed values.
	if si.Spec.Enabled != nil {
		if obs == nil || obs.ShowOutput == nil || *si.Spec.Enabled != obs.ShowOutput.Enabled {
			opts.Enabled = si.Spec.Enabled
		}
	}

	if si.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *si.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = si.Spec.Comment
		}
	}

	switch si.Spec.Type {
	case snowplanev1alpha1.SecurityIntegrationTypeExternalOAuth:
		if c := si.Spec.ExternalOAuth; c != nil {
			opts.ExternalOAuthTokenUserMappingClaim = &c.TokenUserMappingClaim
			opts.ExternalOAuthJWSKeysURL = c.JWSKeysURL
			if len(c.AudienceList) > 0 {
				list := make([]string, len(c.AudienceList))
				copy(list, c.AudienceList)
				opts.ExternalOAuthAudienceList = &list
			}
			if len(c.AllowedRoles) > 0 {
				list := make([]string, len(c.AllowedRoles))
				copy(list, c.AllowedRoles)
				opts.ExternalOAuthAllowedRoles = &list
			}
			if len(c.BlockedRoles) > 0 {
				list := make([]string, len(c.BlockedRoles))
				copy(list, c.BlockedRoles)
				opts.ExternalOAuthBlockedRoles = &list
			}
			opts.ExternalOAuthAnyRoleMode = c.AnyRoleMode
			opts.ExternalOAuthScopeDelimiter = c.ScopeDelimiter
			opts.ExternalOAuthNetworkPolicy = c.NetworkPolicy
		}
	case snowplanev1alpha1.SecurityIntegrationTypeSAML2:
		if c := si.Spec.SAML2; c != nil {
			opts.SAML2X509Cert = &c.X509Cert
			if len(c.AllowedEmailPatterns) > 0 {
				list := make([]string, len(c.AllowedEmailPatterns))
				copy(list, c.AllowedEmailPatterns)
				opts.SAML2AllowedEmailPatterns = &list
			}
			if len(c.AllowedUserDomains) > 0 {
				list := make([]string, len(c.AllowedUserDomains))
				copy(list, c.AllowedUserDomains)
				opts.SAML2AllowedUserDomains = &list
			}
			opts.SAML2SPInitiatedLoginLabel = c.SPInitiatedLoginPageLabel
			opts.SAML2EnableSPInitiated = c.EnableSPInitiated
			opts.SAML2ForceAuthn = c.ForceAuthn
			opts.SAML2RequestedNameIDFormat = c.RequestedNameIDFormat
			opts.SAML2PostLogoutRedirectURL = c.PostLogoutRedirectURL
		}
	case snowplanev1alpha1.SecurityIntegrationTypeSCIM:
		if c := si.Spec.SCIM; c != nil {
			if c.NetworkPolicy != nil {
				opts.SCIMNetworkPolicy = c.NetworkPolicy
			}
			opts.SCIMSyncPassword = c.SyncPassword
		}
	case snowplanev1alpha1.SecurityIntegrationTypeAPIAuthentication:
		if c := si.Spec.APIAuthentication; c != nil {
			if len(c.OAuthAllowedScopes) > 0 {
				list := make([]string, len(c.OAuthAllowedScopes))
				copy(list, c.OAuthAllowedScopes)
				opts.OAuthAllowedScopes = &list
			}
		}
	}

	return opts
}

func detectDrift(si *snowplanev1alpha1.SecurityIntegration, obs *snowflake.SecurityIntegrationObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", si.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("TYPE", string(si.Spec.Type), obs.ShowOutput.Type, true)

		// Mutable fields.
		d.CompareString("COMMENT", si.Spec.Comment, obs.ShowOutput.Comment, false)

		if si.Spec.Enabled != nil {
			obsEnabled := obs.ShowOutput.Enabled
			d.CompareBool("ENABLED", si.Spec.Enabled, &obsEnabled, false)
		}
	}

	// Compare sub-type fields from DESCRIBE output.
	if obs.DescribeOutput != nil {
		switch si.Spec.Type {
		case snowplanev1alpha1.SecurityIntegrationTypeExternalOAuth:
			if c := si.Spec.ExternalOAuth; c != nil {
				d.CompareStringValue("EXTERNAL_OAUTH_JWS_KEYS_URL", stringValueOrEmpty(c.JWSKeysURL), describeValue(obs, "EXTERNAL_OAUTH_JWS_KEYS_URL"), false)
				d.CompareStringValueFold("EXTERNAL_OAUTH_ANY_ROLE_MODE", stringValueOrEmpty(c.AnyRoleMode), describeValue(obs, "EXTERNAL_OAUTH_ANY_ROLE_MODE"), false)
				compareListFromDescribe(d, "EXTERNAL_OAUTH_AUDIENCE_LIST", c.AudienceList, obs)
				compareListFromDescribe(d, "EXTERNAL_OAUTH_ALLOWED_ROLES_LIST", c.AllowedRoles, obs)
				compareListFromDescribe(d, "EXTERNAL_OAUTH_BLOCKED_ROLES_LIST", c.BlockedRoles, obs)
				d.CompareStringValue("NETWORK_POLICY", stringValueOrEmpty(c.NetworkPolicy), describeValue(obs, "NETWORK_POLICY"), false)
			}
		case snowplanev1alpha1.SecurityIntegrationTypeSAML2:
			if c := si.Spec.SAML2; c != nil {
				d.CompareStringValue("SAML2_X509_CERT", c.X509Cert, describeValue(obs, "SAML2_X509_CERT"), false)
				compareListFromDescribe(d, "ALLOWED_EMAIL_PATTERNS", c.AllowedEmailPatterns, obs)
				compareListFromDescribe(d, "ALLOWED_USER_DOMAINS", c.AllowedUserDomains, obs)
			}
		case snowplanev1alpha1.SecurityIntegrationTypeSCIM:
			if c := si.Spec.SCIM; c != nil {
				d.CompareStringValue("NETWORK_POLICY", stringValueOrEmpty(c.NetworkPolicy), describeValue(obs, "NETWORK_POLICY"), false)
				if c.SyncPassword != nil {
					expected := "false"
					if *c.SyncPassword {
						expected = "true"
					}
					d.CompareStringValueFold("SYNC_PASSWORD", expected, describeValue(obs, "SYNC_PASSWORD"), false)
				}
			}
		case snowplanev1alpha1.SecurityIntegrationTypeAPIAuthentication:
			// API Authentication integrations have no sub-type fields to compare via DESCRIBE.
		}
	}

	return d.Result()
}

// describeValue extracts a DESCRIBE output value by key.
func describeValue(obs *snowflake.SecurityIntegrationObservation, key string) string {
	if obs.DescribeOutput == nil {
		return ""
	}

	return obs.DescribeOutput[key]
}

// stringValueOrEmpty returns the value of a string pointer, or "" if nil.
func stringValueOrEmpty(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// compareListFromDescribe compares a spec list against a comma-separated DESCRIBE output value.
func compareListFromDescribe(d *drift.Detector, key string, specList []string, obs *snowflake.SecurityIntegrationObservation) {
	descList := parseCommaList(obs, key)
	specJoined := strings.Join(specList, ",")
	descJoined := strings.Join(descList, ",")
	d.CompareStringValueFold(key, specJoined, descJoined, false)
}

// parseLocations extracts a location list from DESCRIBE output by key.
func parseCommaList(obs *snowflake.SecurityIntegrationObservation, key string) []string {
	if obs.DescribeOutput == nil {
		return nil
	}

	raw, ok := obs.DescribeOutput[key]
	if !ok || raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}

	return result
}
