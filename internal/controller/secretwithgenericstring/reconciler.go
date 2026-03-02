// Package secretwithgenericstring implements the reconciler for
// SecretWithGenericString resources.
package secretwithgenericstring

import (
	"context"

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
	finalizerName = "snowplane.hupe1980.github.io/secretwithgenericstring"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake secrets.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error)
	Create(ctx context.Context, opts snowflake.CreateSecretOptions) error
	Alter(ctx context.Context, opts snowflake.AlterSecretOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new SecretWithGenericString reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.SecretWithGenericString, Service, *snowflake.SecretObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.SecretWithGenericString, Service, *snowflake.SecretObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter, sf ServiceFactory) *reconciler.GenericReconciler[*snowplanev1alpha1.SecretWithGenericString, Service, *snowflake.SecretObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.SecretWithGenericString, Service, *snowflake.SecretObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
	if err != nil {
		return nil, nil, err
	}

	return snowflake.NewSecretClient(sfC), cleanup, nil
}

func applyObservation(obj *snowplanev1alpha1.SecretWithGenericString, obs *snowflake.SecretObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName, obs.ShowOutput.SchemaName, obs.ShowOutput.Name,
		).FullyQualifiedName()

		obj.Status.ShowOutput = &snowplanev1alpha1.SecretShowOutput{
			CreatedOn:    stringPtrOrNil(obs.ShowOutput.CreatedOn),
			Name:         stringPtrOrNil(obs.ShowOutput.Name),
			DatabaseName: stringPtrOrNil(obs.ShowOutput.DatabaseName),
			SchemaName:   stringPtrOrNil(obs.ShowOutput.SchemaName),
			Owner:        stringPtrOrNil(obs.ShowOutput.Owner),
			Comment:      stringPtrOrNil(obs.ShowOutput.Comment),
			SecretType:   stringPtrOrNil(obs.ShowOutput.SecretType),
			OAuthScopes:  stringPtrOrNil(obs.ShowOutput.OAuthScopes),
		}
	}

	if obs.DescribeOutput != nil {
		obj.Status.DescribeOutput = &snowplanev1alpha1.SecretDescribeOutput{
			SecretType:                  stringPtrOrNil(obs.DescribeOutput["secret_type"]),
			Username:                    stringPtrOrNil(obs.DescribeOutput["username"]),
			OAuthAccessTokenExpiryTime:  stringPtrOrNil(obs.DescribeOutput["oauth_access_token_expiry_time"]),
			OAuthRefreshTokenExpiryTime: stringPtrOrNil(obs.DescribeOutput["oauth_refresh_token_expiry_time"]),
			OAuthScopes:                 stringPtrOrNil(obs.DescribeOutput["oauth_scopes"]),
			IntegrationName:             stringPtrOrNil(obs.DescribeOutput["integration_name"]),
			Comment:                     stringPtrOrNil(obs.DescribeOutput["comment"]),
		}
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.SecretWithGenericString, id snowflake.SchemaObjectIdentifier) snowflake.CreateSecretOptions {
	return snowflake.CreateSecretOptions{
		Name:         id,
		SecretType:   snowflake.SecretTypeGenericString,
		SecretString: obj.Spec.SecretString,
		Comment:      obj.Spec.Comment,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.SecretWithGenericString, id snowflake.SchemaObjectIdentifier, obs *snowflake.SecretObservation) snowflake.AlterSecretOptions {
	opts := snowflake.AlterSecretOptions{
		Name:       id,
		SecretType: snowflake.SecretTypeGenericString,
	}
	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	secretString := obj.Spec.SecretString
	opts.SecretString = &secretString

	if obj.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.SecretWithGenericString, obs *snowflake.SecretObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	return d.Result()
}

func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
