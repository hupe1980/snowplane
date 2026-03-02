// Package authenticationpolicy implements the reconciler for AuthenticationPolicy resources.
package authenticationpolicy

import (
	"context"
	"fmt"
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
	finalizerName = "snowplane.hupe1980.github.io/authenticationpolicy"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake authentication policies.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AuthenticationPolicyObservation, error)
	Create(ctx context.Context, opts snowflake.CreateAuthenticationPolicyOptions) error
	Alter(ctx context.Context, opts snowflake.AlterAuthenticationPolicyOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new AuthenticationPolicy reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.AuthenticationPolicy, Service, *snowflake.AuthenticationPolicyObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.AuthenticationPolicy, Service, *snowflake.AuthenticationPolicyObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.AuthenticationPolicy, Service, *snowflake.AuthenticationPolicyObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.AuthenticationPolicy, Service, *snowflake.AuthenticationPolicyObservation]{
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

	return snowflake.NewAuthenticationPolicyClient(sfC), cleanup, nil
}

func applyObservation(ap *snowplanev1alpha1.AuthenticationPolicy, obs *snowflake.AuthenticationPolicyObservation) {
	if obs.ShowOutput != nil {
		ap.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()

		ap.Status.ShowOutput = &snowplanev1alpha1.AuthenticationPolicyShowOutput{
			CreatedOn:    obs.ShowOutput.CreatedOn,
			Name:         obs.ShowOutput.Name,
			DatabaseName: obs.ShowOutput.DatabaseName,
			SchemaName:   obs.ShowOutput.SchemaName,
			Owner:        obs.ShowOutput.Owner,
			Comment:      obs.ShowOutput.Comment,
		}
	}

	if obs.DescribeOutput != nil {
		ap.Status.DescribeOutput = obs.DescribeOutput
	}
}

func buildCreateOptions(ap *snowplanev1alpha1.AuthenticationPolicy, id snowflake.SchemaObjectIdentifier) snowflake.CreateAuthenticationPolicyOptions {
	opts := snowflake.CreateAuthenticationPolicyOptions{
		Name:                  id,
		AuthenticationMethods: ap.Spec.AuthenticationMethods,
		ClientTypes:           ap.Spec.ClientTypes,
		SecurityIntegrations:  ap.Spec.SecurityIntegrations,
		MfaEnrollment:         ap.Spec.MfaEnrollment,
		Comment:               ap.Spec.Comment,
	}

	if ap.Spec.MfaPolicy != nil {
		opts.MfaAllowedMethods = ap.Spec.MfaPolicy.AllowedMethods
		opts.MfaEnforceMfaOnExternalAuth = ap.Spec.MfaPolicy.EnforceMfaOnExternalAuthentication
	}

	if ap.Spec.PatPolicy != nil {
		opts.PatDefaultExpiryInDays = ap.Spec.PatPolicy.DefaultExpiryInDays
		opts.PatMaxExpiryInDays = ap.Spec.PatPolicy.MaxExpiryInDays
		opts.PatNetworkPolicyEvaluation = ap.Spec.PatPolicy.NetworkPolicyEvaluation
		opts.PatRequireRoleRestriction = ap.Spec.PatPolicy.RequireRoleRestrictionForServiceUsers
	}

	if ap.Spec.WorkloadIdentityPolicy != nil {
		opts.WorkloadIdentityAllowedProviders = ap.Spec.WorkloadIdentityPolicy.AllowedProviders
		opts.WorkloadIdentityAllowedAwsAccounts = ap.Spec.WorkloadIdentityPolicy.AllowedAwsAccounts
		opts.WorkloadIdentityAllowedAzureIssuers = ap.Spec.WorkloadIdentityPolicy.AllowedAzureIssuers
		opts.WorkloadIdentityAllowedOidcIssuers = ap.Spec.WorkloadIdentityPolicy.AllowedOidcIssuers
	}

	return opts
}

func buildAlterOptions(ap *snowplanev1alpha1.AuthenticationPolicy, id snowflake.SchemaObjectIdentifier, obs *snowflake.AuthenticationPolicyObservation) snowflake.AlterAuthenticationPolicyOptions {
	opts := snowflake.AlterAuthenticationPolicyOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&ap.Spec, ap.Status.TrackedParameters)

	desc := describeMap(obs)

	// List fields: send whenever spec is non-empty (Snowflake replaces the full list).
	if len(ap.Spec.AuthenticationMethods) > 0 {
		if !descListEqual("AUTHENTICATION_METHODS", ap.Spec.AuthenticationMethods, desc) {
			opts.AuthenticationMethods = ap.Spec.AuthenticationMethods
		}
	}

	if len(ap.Spec.ClientTypes) > 0 {
		if !descListEqual("CLIENT_TYPES", ap.Spec.ClientTypes, desc) {
			opts.ClientTypes = ap.Spec.ClientTypes
		}
	}

	if len(ap.Spec.SecurityIntegrations) > 0 {
		if !descListEqual("SECURITY_INTEGRATIONS", ap.Spec.SecurityIntegrations, desc) {
			opts.SecurityIntegrations = ap.Spec.SecurityIntegrations
		}
	}

	if ap.Spec.MfaEnrollment != nil {
		if descRaw, ok := desc["MFA_ENROLLMENT"]; !ok || !strings.EqualFold(*ap.Spec.MfaEnrollment, descRaw) {
			opts.MfaEnrollment = ap.Spec.MfaEnrollment
		}
	}

	if ap.Spec.MfaPolicy != nil {
		if len(ap.Spec.MfaPolicy.AllowedMethods) > 0 {
			if !descListEqual("MFA_AUTHENTICATION_METHODS", ap.Spec.MfaPolicy.AllowedMethods, desc) {
				opts.MfaAllowedMethods = ap.Spec.MfaPolicy.AllowedMethods
			}
		}

		if ap.Spec.MfaPolicy.EnforceMfaOnExternalAuthentication != nil {
			if descRaw, ok := desc["ENFORCE_MFA_ON_EXTERNAL_AUTHENTICATION"]; !ok || !strings.EqualFold(*ap.Spec.MfaPolicy.EnforceMfaOnExternalAuthentication, descRaw) {
				opts.MfaEnforceMfaOnExternalAuth = ap.Spec.MfaPolicy.EnforceMfaOnExternalAuthentication
			}
		}
	}

	if ap.Spec.PatPolicy != nil {
		opts.PatDefaultExpiryInDays = compareDescInt32(ap.Spec.PatPolicy.DefaultExpiryInDays, "PAT_DEFAULT_EXPIRY_IN_DAYS", desc)
		opts.PatMaxExpiryInDays = compareDescInt32(ap.Spec.PatPolicy.MaxExpiryInDays, "PAT_MAX_EXPIRY_IN_DAYS", desc)

		if ap.Spec.PatPolicy.NetworkPolicyEvaluation != nil {
			if descRaw, ok := desc["PAT_NETWORK_POLICY_EVALUATION"]; !ok || !strings.EqualFold(*ap.Spec.PatPolicy.NetworkPolicyEvaluation, descRaw) {
				opts.PatNetworkPolicyEvaluation = ap.Spec.PatPolicy.NetworkPolicyEvaluation
			}
		}

		if ap.Spec.PatPolicy.RequireRoleRestrictionForServiceUsers != nil {
			boolStr := "false"
			if *ap.Spec.PatPolicy.RequireRoleRestrictionForServiceUsers {
				boolStr = "true"
			}

			if descRaw, ok := desc["PAT_REQUIRE_ROLE_RESTRICTION_FOR_SERVICE_USERS"]; !ok || !strings.EqualFold(boolStr, descRaw) {
				opts.PatRequireRoleRestriction = ap.Spec.PatPolicy.RequireRoleRestrictionForServiceUsers
			}
		}
	}

	if ap.Spec.WorkloadIdentityPolicy != nil {
		if len(ap.Spec.WorkloadIdentityPolicy.AllowedProviders) > 0 {
			if !descListEqual("WORKLOAD_IDENTITY_ALLOWED_PROVIDERS", ap.Spec.WorkloadIdentityPolicy.AllowedProviders, desc) {
				opts.WorkloadIdentityAllowedProviders = ap.Spec.WorkloadIdentityPolicy.AllowedProviders
			}
		}

		if len(ap.Spec.WorkloadIdentityPolicy.AllowedAwsAccounts) > 0 {
			if !descListEqual("WORKLOAD_IDENTITY_ALLOWED_AWS_ACCOUNTS", ap.Spec.WorkloadIdentityPolicy.AllowedAwsAccounts, desc) {
				opts.WorkloadIdentityAllowedAwsAccounts = ap.Spec.WorkloadIdentityPolicy.AllowedAwsAccounts
			}
		}

		if len(ap.Spec.WorkloadIdentityPolicy.AllowedAzureIssuers) > 0 {
			if !descListEqual("WORKLOAD_IDENTITY_ALLOWED_AZURE_ISSUERS", ap.Spec.WorkloadIdentityPolicy.AllowedAzureIssuers, desc) {
				opts.WorkloadIdentityAllowedAzureIssuers = ap.Spec.WorkloadIdentityPolicy.AllowedAzureIssuers
			}
		}

		if len(ap.Spec.WorkloadIdentityPolicy.AllowedOidcIssuers) > 0 {
			if !descListEqual("WORKLOAD_IDENTITY_ALLOWED_OIDC_ISSUERS", ap.Spec.WorkloadIdentityPolicy.AllowedOidcIssuers, desc) {
				opts.WorkloadIdentityAllowedOidcIssuers = ap.Spec.WorkloadIdentityPolicy.AllowedOidcIssuers
			}
		}
	}

	if ap.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *ap.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = ap.Spec.Comment
		}
	}

	return opts
}

// describeMap safely extracts the DESCRIBE output map from an observation.
func describeMap(obs *snowflake.AuthenticationPolicyObservation) map[string]string {
	if obs == nil || obs.DescribeOutput == nil {
		return nil
	}

	return obs.DescribeOutput
}

// compareDescInt32 returns specVal only if it differs from the DESCRIBE output value.
func compareDescInt32(specVal *int32, key string, desc map[string]string) *int32 {
	if specVal == nil {
		return nil
	}

	if desc == nil {
		return specVal
	}

	descRaw, ok := desc[key]
	if !ok {
		return specVal
	}

	if fmt.Sprintf("%d", *specVal) == descRaw {
		return nil
	}

	return specVal
}

// descListEqual checks whether a DESCRIBE output value (a bracket-delimited list)
// matches the given spec list. DESCRIBE returns lists like "[PASSWORD, SAML]".
func descListEqual(key string, specVals []string, desc map[string]string) bool {
	if desc == nil {
		return false
	}

	descRaw, ok := desc[key]
	if !ok {
		return false
	}

	// Parse "[VAL1, VAL2]" format from DESCRIBE output.
	descRaw = strings.TrimPrefix(descRaw, "[")
	descRaw = strings.TrimSuffix(descRaw, "]")

	descParts := strings.Split(descRaw, ",")
	if len(descParts) != len(specVals) {
		return false
	}

	descSet := make(map[string]struct{}, len(descParts))
	for _, p := range descParts {
		descSet[strings.ToUpper(strings.TrimSpace(p))] = struct{}{}
	}

	for _, s := range specVals {
		if _, ok := descSet[strings.ToUpper(strings.TrimSpace(s))]; !ok {
			return false
		}
	}

	return true
}

func detectDrift(ap *snowplanev1alpha1.AuthenticationPolicy, obs *snowflake.AuthenticationPolicyObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", ap.Spec.Name, obs.ShowOutput.Name, true)

		// Mutable fields from SHOW output.
		d.CompareString("COMMENT", ap.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	if obs.DescribeOutput != nil {
		// Scalar enum fields from DESCRIBE output.
		compareDescStringFromDescribe(d, "MFA_ENROLLMENT", ap.Spec.MfaEnrollment, obs.DescribeOutput)

		// List fields from DESCRIBE output.
		compareDescListFromDescribe(d, "AUTHENTICATION_METHODS", ap.Spec.AuthenticationMethods, obs.DescribeOutput)
		compareDescListFromDescribe(d, "CLIENT_TYPES", ap.Spec.ClientTypes, obs.DescribeOutput)
		compareDescListFromDescribe(d, "SECURITY_INTEGRATIONS", ap.Spec.SecurityIntegrations, obs.DescribeOutput)

		// MFA sub-policy fields.
		if ap.Spec.MfaPolicy != nil {
			compareDescListFromDescribe(d, "MFA_AUTHENTICATION_METHODS", ap.Spec.MfaPolicy.AllowedMethods, obs.DescribeOutput)
			compareDescStringFromDescribe(d, "ENFORCE_MFA_ON_EXTERNAL_AUTHENTICATION", ap.Spec.MfaPolicy.EnforceMfaOnExternalAuthentication, obs.DescribeOutput)
		}

		// PAT sub-policy fields.
		if ap.Spec.PatPolicy != nil {
			compareDescInt32FromDescribe(d, "PAT_DEFAULT_EXPIRY_IN_DAYS", ap.Spec.PatPolicy.DefaultExpiryInDays, obs.DescribeOutput)
			compareDescInt32FromDescribe(d, "PAT_MAX_EXPIRY_IN_DAYS", ap.Spec.PatPolicy.MaxExpiryInDays, obs.DescribeOutput)
			compareDescStringFromDescribe(d, "PAT_NETWORK_POLICY_EVALUATION", ap.Spec.PatPolicy.NetworkPolicyEvaluation, obs.DescribeOutput)
			compareDescBoolFromDescribe(d, "PAT_REQUIRE_ROLE_RESTRICTION_FOR_SERVICE_USERS", ap.Spec.PatPolicy.RequireRoleRestrictionForServiceUsers, obs.DescribeOutput)
		}

		// Workload identity sub-policy fields.
		if ap.Spec.WorkloadIdentityPolicy != nil {
			compareDescListFromDescribe(d, "WORKLOAD_IDENTITY_ALLOWED_PROVIDERS", ap.Spec.WorkloadIdentityPolicy.AllowedProviders, obs.DescribeOutput)
			compareDescListFromDescribe(d, "WORKLOAD_IDENTITY_ALLOWED_AWS_ACCOUNTS", ap.Spec.WorkloadIdentityPolicy.AllowedAwsAccounts, obs.DescribeOutput)
			compareDescListFromDescribe(d, "WORKLOAD_IDENTITY_ALLOWED_AZURE_ISSUERS", ap.Spec.WorkloadIdentityPolicy.AllowedAzureIssuers, obs.DescribeOutput)
			compareDescListFromDescribe(d, "WORKLOAD_IDENTITY_ALLOWED_OIDC_ISSUERS", ap.Spec.WorkloadIdentityPolicy.AllowedOidcIssuers, obs.DescribeOutput)
		}
	}

	return d.Result()
}

// compareDescStringFromDescribe compares a spec string pointer against a DESCRIBE output value.
func compareDescStringFromDescribe(d *drift.Detector, key string, specVal *string, descOutput map[string]string) {
	if specVal == nil {
		return
	}

	descRaw, ok := descOutput[key]
	if !ok {
		return
	}

	d.CompareStringValueFold(key, *specVal, descRaw, false)
}

// compareDescInt32FromDescribe compares a spec int32 pointer against a DESCRIBE output value
// and records drift if they differ.
func compareDescInt32FromDescribe(d *drift.Detector, key string, specVal *int32, descOutput map[string]string) {
	if specVal == nil {
		return
	}

	descRaw, ok := descOutput[key]
	if !ok {
		return
	}

	specStr := fmt.Sprintf("%d", *specVal)
	d.CompareStringValueFold(key, specStr, descRaw, false)
}

// compareDescBoolFromDescribe compares a spec bool pointer against a DESCRIBE output value
// and records drift if they differ.
func compareDescBoolFromDescribe(d *drift.Detector, key string, specVal *bool, descOutput map[string]string) {
	if specVal == nil {
		return
	}

	descRaw, ok := descOutput[key]
	if !ok {
		return
	}

	specStr := "false"
	if *specVal {
		specStr = "true"
	}

	d.CompareStringValueFold(key, specStr, descRaw, false)
}

// compareDescListFromDescribe compares a spec string slice against a DESCRIBE output
// bracket-delimited list (e.g. "[PASSWORD, SAML]") and records drift if they differ.
func compareDescListFromDescribe(d *drift.Detector, key string, specVals []string, descOutput map[string]string) {
	if len(specVals) == 0 {
		return
	}

	descRaw, ok := descOutput[key]
	if !ok {
		return
	}

	// Parse "[VAL1, VAL2]" format from DESCRIBE output.
	inner := strings.TrimPrefix(descRaw, "[")
	inner = strings.TrimSuffix(inner, "]")

	var descParts []string
	for _, p := range strings.Split(inner, ",") {
		descParts = append(descParts, strings.TrimSpace(p))
	}

	d.CompareStringSliceFold(key, specVals, descParts, false)
}
