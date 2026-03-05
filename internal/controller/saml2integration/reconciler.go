// Package saml2integration implements the reconciler for SAML2Integration resources.
package saml2integration

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
	finalizerName = "snowplane.hupe1980.github.io/saml2integration"
)

// SnowflakeClient is an alias for the client factory's SnowflakeClient type.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines the Snowflake operations for SAML2 security integrations.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SAML2IntegrationObservation, error)
	Create(ctx context.Context, opts snowflake.CreateSAML2IntegrationOptions) error
	Alter(ctx context.Context, opts snowflake.AlterSAML2IntegrationOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler creates a new reconciler using the default service factory.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.SAML2Integration, Service, *snowflake.SAML2IntegrationObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewSAML2IntegrationClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory creates a new reconciler with a custom service factory.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.SAML2Integration, Service, *snowflake.SAML2IntegrationObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for SAML2Integration resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.SAML2Integration, Service, *snowflake.SAML2IntegrationObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.SAML2Integration, Service, *snowflake.SAML2IntegrationObservation]{
		ResourceNameVal:  "saml2integration",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.SAML2Integration { return &snowplanev1alpha1.SAML2Integration{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.SAML2Integration) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.SAML2IntegrationObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.SAML2IntegrationObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.SAML2Integration, id snowflake.AccountObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterSAML2IntegrationOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.SAML2Integration, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.SAML2IntegrationObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.SAML2Integration, obs *reconciler.Observation[*snowflake.SAML2IntegrationObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.SAML2Integration, obs *reconciler.Observation[*snowflake.SAML2IntegrationObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

// validateImmutableFields checks that immutable fields have not been changed.
func validateImmutableFields(_ context.Context, si *snowplanev1alpha1.SAML2Integration) error {
	if reconciler.ShouldSkipImmutableValidation(si) {
		return nil
	}

	if si.Status.ShowOutput != nil && si.Status.ShowOutput.Name != "" {
		if !strings.EqualFold(si.Spec.Name, si.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", si.Status.ShowOutput.Name, si.Spec.Name)
		}
	}

	return nil
}

func applyObservation(si *snowplanev1alpha1.SAML2Integration, obs *snowflake.SAML2IntegrationObservation) {
	if obs.ShowOutput != nil {
		si.Status.FullyQualifiedName = obs.ShowOutput.Name
		si.Status.ShowOutput = obs.ShowOutput
	}

	if obs.DescribeOutput != nil {
		si.Status.DescribeOutput = obs.DescribeOutput
	}
}

func buildCreateOptions(si *snowplanev1alpha1.SAML2Integration, id snowflake.AccountObjectIdentifier) snowflake.CreateSAML2IntegrationOptions {
	return snowflake.CreateSAML2IntegrationOptions{
		Name:                  id,
		Enabled:               si.Spec.Enabled,
		Issuer:                si.Spec.Issuer,
		SSOURL:                si.Spec.SSOURL,
		Provider:              si.Spec.Provider,
		X509Cert:              si.Spec.X509Cert,
		AllowedEmailPatterns:  si.Spec.AllowedEmailPatterns,
		AllowedUserDomains:    si.Spec.AllowedUserDomains,
		SPInitiatedLoginLabel: si.Spec.SPInitiatedLoginPageLabel,
		EnableSPInitiated:     si.Spec.EnableSPInitiated,
		ForceAuthn:            si.Spec.ForceAuthn,
		RequestedNameIDFormat: si.Spec.RequestedNameIDFormat,
		PostLogoutRedirectURL: si.Spec.PostLogoutRedirectURL,
		Comment:               si.Spec.Comment,
	}
}

func buildAlterOptions(si *snowplanev1alpha1.SAML2Integration, id snowflake.AccountObjectIdentifier, obs *snowflake.SAML2IntegrationObservation) snowflake.AlterSAML2IntegrationOptions {
	opts := snowflake.AlterSAML2IntegrationOptions{
		Name: id,
	}
	opts.UnsetFields = tracked.ComputeUnset(&si.Spec, si.Status.TrackedParameters)

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

	// Always-SET fields.
	opts.X509Cert = &si.Spec.X509Cert

	if len(si.Spec.AllowedEmailPatterns) > 0 {
		list := make([]string, len(si.Spec.AllowedEmailPatterns))
		copy(list, si.Spec.AllowedEmailPatterns)
		opts.AllowedEmailPatterns = &list
	}

	if len(si.Spec.AllowedUserDomains) > 0 {
		list := make([]string, len(si.Spec.AllowedUserDomains))
		copy(list, si.Spec.AllowedUserDomains)
		opts.AllowedUserDomains = &list
	}

	opts.SPInitiatedLoginLabel = si.Spec.SPInitiatedLoginPageLabel
	opts.EnableSPInitiated = si.Spec.EnableSPInitiated
	opts.ForceAuthn = si.Spec.ForceAuthn
	opts.RequestedNameIDFormat = si.Spec.RequestedNameIDFormat
	opts.PostLogoutRedirectURL = si.Spec.PostLogoutRedirectURL

	return opts
}

func detectDrift(si *snowplanev1alpha1.SAML2Integration, obs *snowflake.SAML2IntegrationObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", si.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", si.Spec.Comment, obs.ShowOutput.Comment, false)

		if si.Spec.Enabled != nil {
			obsEnabled := obs.ShowOutput.Enabled
			d.CompareBool("ENABLED", si.Spec.Enabled, &obsEnabled, false)
		}
	}

	if obs.DescribeOutput != nil {
		d.CompareStringValue("SAML2_X509_CERT", si.Spec.X509Cert, describeValue(obs, "SAML2_X509_CERT"), false)
		compareListFromDescribe(d, "ALLOWED_EMAIL_PATTERNS", si.Spec.AllowedEmailPatterns, obs)
		compareListFromDescribe(d, "ALLOWED_USER_DOMAINS", si.Spec.AllowedUserDomains, obs)
	}

	return d.Result()
}

func describeValue(obs *snowflake.SAML2IntegrationObservation, key string) string {
	if obs.DescribeOutput == nil {
		return ""
	}

	return obs.DescribeOutput[key]
}

func compareListFromDescribe(d *drift.Detector, key string, specList []string, obs *snowflake.SAML2IntegrationObservation) {
	descList := parseCommaList(obs, key)
	specJoined := strings.Join(specList, ",")
	descJoined := strings.Join(descList, ",")
	d.CompareStringValueFold(key, specJoined, descJoined, false)
}

func parseCommaList(obs *snowflake.SAML2IntegrationObservation, key string) []string {
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
