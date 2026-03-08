// Package apiauthenticationintegrationwithjwtbearer implements the reconciler for
// APIAuthenticationIntegrationWithJWTBearer resources.
package apiauthenticationintegrationwithjwtbearer

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
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/apiauthenticationintegrationwithjwtbearer"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake API authentication integrations.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error)
	Create(ctx context.Context, opts snowflake.CreateAPIAuthenticationIntegrationOptions) error
	Alter(ctx context.Context, opts snowflake.AlterAPIAuthenticationIntegrationOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new APIAuthenticationIntegrationWithJWTBearer reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer, Service, *snowflake.APIAuthenticationIntegrationObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewAPIAuthenticationIntegrationClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer, Service, *snowflake.APIAuthenticationIntegrationObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, sf))
}

// newAdapter creates the BaseAdapter for APIAuthenticationIntegrationWithJWTBearer resources.
func newAdapter(c client.Client, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer, Service, *snowflake.APIAuthenticationIntegrationObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer, Service, *snowflake.APIAuthenticationIntegrationObservation]{
		ResourceNameVal:  "apiauthenticationintegrationwithjwtbearer",
		FinalizerNameVal: finalizerName,
		NewObjectFn: func() *snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer {
			return &snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer{}
		},
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.APIAuthenticationIntegrationObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer, id snowflake.AccountObjectIdentifier) error {
			opts, err := buildCreateOptions(ctx, c, obj, id)
			if err != nil {
				return err
			}
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterAPIAuthenticationIntegrationOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.APIAuthenticationIntegrationObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer, obs *reconciler.Observation[*snowflake.APIAuthenticationIntegrationObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer, obs *reconciler.Observation[*snowflake.APIAuthenticationIntegrationObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
	}
}

// validateImmutableFields checks that immutable fields have not changed.
func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Name != "" {
		if !strings.EqualFold(obj.Spec.Name, obj.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", obj.Status.ShowOutput.Name, obj.Spec.Name)
		}
	}

	return nil
}

func applyObservation(obj *snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer, obs *snowflake.APIAuthenticationIntegrationObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = obs.ShowOutput.Name
		obj.Status.ShowOutput = obs.ShowOutput
	}

	if obs.DescribeOutput != nil {
		obj.Status.DescribeOutput = &snowplanev1alpha1.APIAuthenticationIntegrationDescribeOutput{
			AuthType:                   stringPtrOrNil(obs.DescribeOutput["AUTH_TYPE"]),
			OAuthClientID:              stringPtrOrNil(obs.DescribeOutput["OAUTH_CLIENT_ID"]),
			OAuthClientAuthMethod:      stringPtrOrNil(obs.DescribeOutput["OAUTH_CLIENT_AUTH_METHOD"]),
			OAuthTokenEndpoint:         stringPtrOrNil(obs.DescribeOutput["OAUTH_TOKEN_ENDPOINT"]),
			OAuthAuthorizationEndpoint: stringPtrOrNil(obs.DescribeOutput["OAUTH_AUTHORIZATION_ENDPOINT"]),
			OAuthGrant:                 stringPtrOrNil(obs.DescribeOutput["OAUTH_GRANT"]),
			OAuthAccessTokenValidity:   stringPtrOrNil(obs.DescribeOutput["OAUTH_ACCESS_TOKEN_VALIDITY"]),
			OAuthRefreshTokenValidity:  stringPtrOrNil(obs.DescribeOutput["OAUTH_REFRESH_TOKEN_VALIDITY"]),
			OAuthAllowedScopes:         stringPtrOrNil(obs.DescribeOutput["OAUTH_ALLOWED_SCOPES"]),
			Enabled:                    stringPtrOrNil(obs.DescribeOutput["ENABLED"]),
			Comment:                    stringPtrOrNil(obs.DescribeOutput["COMMENT"]),
			ParentIntegration:          stringPtrOrNil(obs.DescribeOutput["PARENT_INTEGRATION"]),
		}
	}
}

func buildCreateOptions(ctx context.Context, c client.Client, obj *snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer, id snowflake.AccountObjectIdentifier) (snowflake.CreateAPIAuthenticationIntegrationOptions, error) {
	clientSecret, err := refresolver.ResolveSecretKeyRef(ctx, c, obj.Namespace, obj.Spec.OAuthClientSecretRef)
	if err != nil {
		return snowflake.CreateAPIAuthenticationIntegrationOptions{}, fmt.Errorf("resolving oauthClientSecretRef: %w", err)
	}

	opts := snowflake.CreateAPIAuthenticationIntegrationOptions{
		Name:              id,
		OAuthGrantType:    snowflake.OAuthGrantTypeJWTBearer,
		OAuthClientID:     obj.Spec.OAuthClientID,
		OAuthClientSecret: clientSecret,
		Comment:           obj.Spec.Comment,
	}

	opts.Enabled = obj.Spec.Enabled
	opts.OAuthTokenEndpoint = obj.Spec.OAuthTokenEndpoint
	opts.OAuthAuthorizationEndpoint = obj.Spec.OAuthAuthorizationEndpoint

	// OAuthAssertionIssuer is required for JWT bearer.
	issuer := obj.Spec.OAuthAssertionIssuer
	opts.OAuthAssertionIssuer = &issuer

	if obj.Spec.OAuthClientAuthMethod != nil {
		s := string(*obj.Spec.OAuthClientAuthMethod)
		opts.OAuthClientAuthMethod = &s
	}

	if obj.Spec.OAuthAccessTokenValidity != nil {
		v := int32(*obj.Spec.OAuthAccessTokenValidity) //nolint:gosec // G115: value is within int32 range
		opts.OAuthAccessTokenValidity = &v
	}

	if obj.Spec.OAuthRefreshTokenValidity != nil {
		v := int32(*obj.Spec.OAuthRefreshTokenValidity) //nolint:gosec // G115: value is within int32 range
		opts.OAuthRefreshTokenValidity = &v
	}

	// JWT Bearer has no OAuthAllowedScopes.

	return opts, nil
}

func buildAlterOptions(obj *snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer, id snowflake.AccountObjectIdentifier, obs *snowflake.APIAuthenticationIntegrationObservation) snowflake.AlterAPIAuthenticationIntegrationOptions {
	opts := snowflake.AlterAPIAuthenticationIntegrationOptions{
		Name:           id,
		OAuthGrantType: snowflake.OAuthGrantTypeJWTBearer,
	}

	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	if obj.Spec.Enabled != nil && (obs == nil || obs.ShowOutput == nil || *obj.Spec.Enabled != obs.ShowOutput.Enabled) {
		opts.Enabled = obj.Spec.Enabled
	}

	opts.OAuthTokenEndpoint = obj.Spec.OAuthTokenEndpoint
	opts.OAuthAuthorizationEndpoint = obj.Spec.OAuthAuthorizationEndpoint

	// OAuthAssertionIssuer is always sent because it is required.
	issuer := obj.Spec.OAuthAssertionIssuer
	opts.OAuthAssertionIssuer = &issuer

	if obj.Spec.OAuthClientAuthMethod != nil {
		s := string(*obj.Spec.OAuthClientAuthMethod)
		opts.OAuthClientAuthMethod = &s
	}

	if obj.Spec.OAuthAccessTokenValidity != nil {
		v := int32(*obj.Spec.OAuthAccessTokenValidity) //nolint:gosec // G115: value is within int32 range
		opts.OAuthAccessTokenValidity = &v
	}

	if obj.Spec.OAuthRefreshTokenValidity != nil {
		v := int32(*obj.Spec.OAuthRefreshTokenValidity) //nolint:gosec // G115: value is within int32 range
		opts.OAuthRefreshTokenValidity = &v
	}

	if obj.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer, obs *snowflake.APIAuthenticationIntegrationObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)

		obsEnabled := obs.ShowOutput.Enabled
		d.CompareBool("ENABLED", obj.Spec.Enabled, &obsEnabled, false)
	}

	if obs.DescribeOutput != nil {
		if obj.Spec.OAuthTokenEndpoint != nil {
			d.CompareStringValue("OAUTH_TOKEN_ENDPOINT", *obj.Spec.OAuthTokenEndpoint, obs.DescribeOutput["OAUTH_TOKEN_ENDPOINT"], false)
		}

		if obj.Spec.OAuthAuthorizationEndpoint != nil {
			d.CompareStringValue("OAUTH_AUTHORIZATION_ENDPOINT", *obj.Spec.OAuthAuthorizationEndpoint, obs.DescribeOutput["OAUTH_AUTHORIZATION_ENDPOINT"], false)
		}

		if obj.Spec.OAuthClientAuthMethod != nil {
			d.CompareStringValueFold("OAUTH_CLIENT_AUTH_METHOD", string(*obj.Spec.OAuthClientAuthMethod), obs.DescribeOutput["OAUTH_CLIENT_AUTH_METHOD"], false)
		}

		// Check immutable assertion issuer for drift.
		d.CompareStringValue("OAUTH_ASSERTION_ISSUER", obj.Spec.OAuthAssertionIssuer, obs.DescribeOutput["OAUTH_ASSERTION_ISSUER"], false)
	}

	return d.Result()
}

func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
