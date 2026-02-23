// Package provider contains shared utilities for resolving ProviderConfig
// resources and building Snowflake client configurations.
package provider

import (
	"context"
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

// ResolveClient resolves the Snowflake client for a managed resource by
// looking up the ProviderConfig and its referenced credentials Secret.
// It sets appropriate conditions on the object when resolution fails.
// If a rate limiter is provided, it rate-limits the call per provider.
// If a circuit breaker is provided, it short-circuits calls to failing providers.
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
) (clientfactory.SnowflakeClient, error) {
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

		return nil, fmt.Errorf("ProviderConfig not ready")
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
	// The key is provider+controller so each controller gets its own token
	// bucket per provider, preventing a noisy controller from starving others.
	if rl != nil {
		waited, err := rl.Wait(ctx, pc.Name+"/"+controllerName)
		if err != nil {
			return nil, fmt.Errorf("rate limit wait for provider %q controller %q: %w", pc.Name, controllerName, err)
		}

		if waited {
			metrics.SnowflakeRateLimitWaits.With(prometheus.Labels{"controller": controllerName}).Inc()
		}
	}

	sfClient, err := factory.GetOrCreate(pc.Name, hash, cfg)
	if err != nil {
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonReconcileError, fmt.Sprintf("failed to create Snowflake client: %v", err))
		return nil, err
	}

	return sfClient, nil
}
