package oauthintegrationforpartnerapplications

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills nil spec fields from the observed Snowflake state.
func lateInitialize(obj *snowplanev1alpha1.OAuthIntegrationForPartnerApplications, obs *reconciler.Observation[*snowflake.OAuthIntegrationForPartnerApplicationsObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	var modified bool

	if reconciler.LateInitNonZero(&obj.Spec.Comment, detail.ShowOutput.Comment) {
		modified = true
	}

	if reconciler.LateInit(&obj.Spec.Enabled, detail.ShowOutput.Enabled) {
		modified = true
	}

	// Late-initialize optional fields from DESCRIBE output.
	if desc := detail.DescribeOutput; desc != nil {
		if reconciler.LateInitFromMap(&obj.Spec.OAuthRedirectURI, desc, "OAUTH_REDIRECT_URI") {
			modified = true
		}

		if reconciler.LateInitFromMap(&obj.Spec.OAuthUseSecondaryRoles, desc, "OAUTH_USE_SECONDARY_ROLES") {
			modified = true
		}

		if reconciler.LateInitBoolFromMap(&obj.Spec.OAuthIssueRefreshTokens, desc, "OAUTH_ISSUE_REFRESH_TOKENS") {
			modified = true
		}

		if reconciler.LateInitInt64FromMap(&obj.Spec.OAuthRefreshTokenValidity, desc, "OAUTH_REFRESH_TOKEN_VALIDITY") {
			modified = true
		}
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.OAuthIntegrationForPartnerApplications, *snowflake.OAuthIntegrationForPartnerApplicationsObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.OAuthIntegrationForPartnerApplications, Service, *snowflake.OAuthIntegrationForPartnerApplicationsObservation])(nil)
