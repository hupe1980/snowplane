// Package provider contains shared utilities for resolving ProviderConfig
// resources and building Snowflake client configurations.
package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/circuitbreaker"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/metrics"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// ErrProviderConfigNotReady is returned when the referenced ProviderConfig
// exists but has not reached the Ready condition.
var ErrProviderConfigNotReady = errors.New("ProviderConfig not ready")

// ResolvedProvider contains the resolved Snowflake client along with
// provider metadata useful for structured logging and metrics.
type ResolvedProvider struct {
	Client  clientfactory.SnowflakeClient
	Name    string // ProviderConfig name (e.g. "default")
	Account string // Snowflake account identifier (e.g. "orgname-accountname")
}

// ResolveClient resolves the Snowflake client for a managed resource by
// looking up the ProviderConfig and its referenced credentials Secret.
// It sets appropriate conditions on the object when resolution fails.
// If a rate limiter is provided, it rate-limits the call per provider.
// If a circuit breaker is provided, it short-circuits calls to failing providers.
//
// Returns a ResolvedProvider containing the client plus provider metadata
// (name and account) for structured log enrichment (L-3).
func ResolveClient(
	ctx context.Context,
	c client.Client,
	factory *clientfactory.ClientFactory,
	obj conditions.ConditionedObject,
	providerRef snowplanev1alpha1.ProviderReference,
	namespace string,
	rl *ratelimit.Limiter,
	cb *circuitbreaker.Breaker,
	controllerName string,
) (*ResolvedProvider, error) {
	// Look up ProviderConfig. If providerRef.Namespace is set,
	// use it; otherwise fall back to the resource's namespace.
	pcNamespace := namespace
	if providerRef.Namespace != "" {
		pcNamespace = providerRef.Namespace
	}

	pc := &snowplanev1alpha1.ProviderConfig{}
	pcKey := types.NamespacedName{
		Namespace: pcNamespace,
		Name:      providerRef.Name,
	}

	if err := c.Get(ctx, pcKey, pc); err != nil {
		msg := fmt.Sprintf("ProviderConfig %q not found: %v", providerRef.Name, err)
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonDependencyWait, msg)

		return nil, fmt.Errorf("fetching ProviderConfig: %w", err)
	}

	// Enforce AllowedNamespaces: reject if resource namespace is not permitted.
	if !pc.Spec.IsNamespaceAllowed(namespace) {
		msg := fmt.Sprintf("namespace %q is not allowed to use ProviderConfig %q", namespace, providerRef.Name)
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonNamespaceNotAllowed, msg)

		return nil, fmt.Errorf("namespace not allowed: %s", msg)
	}

	if !conditions.IsTrue(pc, snowplanev1alpha1.TypeReady) {
		msg := fmt.Sprintf("ProviderConfig %q is not ready", providerRef.Name)
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonDependencyWait, msg)

		return nil, ErrProviderConfigNotReady
	}

	// Circuit breaker: skip providers with many consecutive failures.
	if cb != nil {
		if err := cb.Allow(pc.Name); err != nil {
			msg := fmt.Sprintf("ProviderConfig %q circuit breaker open (consecutive failures exceeded threshold)", pc.Name)
			conditions.SetNotReady(obj, snowplanev1alpha1.ReasonDependencyNotReady, msg)

			return nil, fmt.Errorf("circuit breaker open for provider %q: %w", pc.Name, err)
		}
	}

	// WorkloadIdentity: no Secret needed — the driver reads the token file directly.
	var cfg snowflake.Config

	if pc.Spec.AuthenticationType == snowplanev1alpha1.AuthenticationTypeWorkloadIdentity {
		var err error

		cfg, err = BuildSnowflakeConfig(pc, nil)
		if err != nil {
			conditions.SetNotReady(obj, snowplanev1alpha1.ReasonCredentialsError, err.Error())
			return nil, err
		}
	} else {
		if pc.Spec.Credentials.SecretRef == nil {
			conditions.SetNotReady(obj, snowplanev1alpha1.ReasonCredentialsError,
				"spec.credentials.secretRef is required")
			return nil, fmt.Errorf("spec.credentials.secretRef is required for %s authentication",
				pc.Spec.AuthenticationType)
		}

		secret := &corev1.Secret{}
		secretNS := pc.Spec.Credentials.SecretRef.Namespace

		if secretNS == "" {
			secretNS = pc.Namespace
		}

		secretRef := types.NamespacedName{
			Namespace: secretNS,
			Name:      pc.Spec.Credentials.SecretRef.Name,
		}

		if err := c.Get(ctx, secretRef, secret); err != nil {
			msg := fmt.Sprintf("credentials secret %q not found: %v", secretRef, err)
			conditions.SetNotReady(obj, snowplanev1alpha1.ReasonCredentialsError, msg)

			return nil, fmt.Errorf("fetching secret: %w", err)
		}

		var err error

		cfg, err = BuildSnowflakeConfig(pc, secret)
		if err != nil {
			conditions.SetNotReady(obj, snowplanev1alpha1.ReasonCredentialsError, err.Error())
			return nil, err
		}
	}

	hash := ComputeHash(cfg)

	// Apply rate limiting before acquiring or creating the Snowflake client.
	// Two levels:
	// 1. Per-controller: token bucket keyed by provider+controller, prevents a
	//    noisy controller from starving others.
	// 2. Per-account: aggregate token bucket keyed by provider, caps total QPS
	//    to a single Snowflake account across all controllers.
	if rl != nil {
		controllerWaited, accountWaited, err := rl.Wait(ctx, pc.Name, controllerName)
		if err != nil {
			return nil, fmt.Errorf("rate limit wait for provider %q controller %q: %w", pc.Name, controllerName, err)
		}

		if controllerWaited {
			metrics.SnowflakeRateLimitWaits.With(prometheus.Labels{"controller": controllerName}).Inc()
		}

		if accountWaited {
			metrics.SnowflakeAccountRateLimitWaits.With(prometheus.Labels{"provider": pc.Name}).Inc()
		}
	}

	sfClient, err := factory.GetOrCreate(pc.Name, hash, cfg)
	if err != nil {
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonReconcileError, fmt.Sprintf("failed to create Snowflake client: %v", err))
		return nil, err
	}

	return &ResolvedProvider{
		Client:  sfClient,
		Name:    pc.Name,
		Account: pc.Spec.Account,
	}, nil
}
