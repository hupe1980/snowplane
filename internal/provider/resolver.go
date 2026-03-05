// Package provider contains shared utilities for resolving ProviderConfig
// resources and building Snowflake client configurations.
package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

// ProviderCacheKey returns the namespace-qualified cache key for a
// ProviderConfig. All subsystems (ClientFactory, RateLimiter, CircuitBreaker,
// metrics) must use this key to avoid cross-namespace collisions when two
// ProviderConfigs share the same name in different namespaces (C-3).
func ProviderCacheKey(namespace, name string) string {
	return namespace + "/" + name
}

// ResolvedProvider contains the resolved Snowflake client along with
// provider metadata useful for structured logging and metrics.
type ResolvedProvider struct {
	Client   clientfactory.SnowflakeClient
	CacheKey string // Namespace-qualified key (e.g. "system/default") for cache/metrics
	Name     string // ProviderConfig name (e.g. "default")
	Account  string // Snowflake account identifier (e.g. "orgname-accountname")

	// AllowedDatabases is the ProviderConfig's database restriction list.
	// Empty means all databases are allowed.
	AllowedDatabases []string

	// AllowedSchemas is the ProviderConfig's schema restriction list.
	// Empty means all schemas are allowed. Entries may be "SCHEMA" or "DATABASE.SCHEMA".
	AllowedSchemas []string
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

	// Enforce namespace restrictions: check static AllowedNamespaces list
	// and AllowedNamespaceSelector (OR semantics).
	if allowed, err := isNamespacePermitted(ctx, c, &pc.Spec, namespace); err != nil {
		msg := fmt.Sprintf("namespace evaluation error for ProviderConfig %q: %v", providerRef.Name, err)
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonNamespaceNotAllowed, msg)

		return nil, fmt.Errorf("namespace evaluation failed: %w", err)
	} else if !allowed {
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
	// Use namespace-qualified key to isolate per-namespace ProviderConfigs (C-3).
	cacheKey := ProviderCacheKey(pc.Namespace, pc.Name)

	if cb != nil {
		if err := cb.Allow(cacheKey); err != nil {
			msg := fmt.Sprintf("ProviderConfig %q circuit breaker open (consecutive failures exceeded threshold)", pc.Name)
			conditions.SetNotReady(obj, snowplanev1alpha1.ReasonDependencyNotReady, msg)

			return nil, fmt.Errorf("circuit breaker open for provider %q: %w", cacheKey, err)
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
		controllerWaited, accountWaited, err := rl.Wait(ctx, cacheKey, controllerName)
		if err != nil {
			return nil, fmt.Errorf("rate limit wait for provider %q controller %q: %w", cacheKey, controllerName, err)
		}

		if controllerWaited {
			metrics.SnowflakeRateLimitWaits.With(prometheus.Labels{"controller": controllerName}).Inc()
		}

		if accountWaited {
			metrics.SnowflakeAccountRateLimitWaits.With(prometheus.Labels{"provider": cacheKey}).Inc()
		}
	}

	sfClient, err := factory.GetOrCreate(cacheKey, hash, cfg)
	if err != nil {
		conditions.SetNotReady(obj, snowplanev1alpha1.ReasonReconcileError, fmt.Sprintf("failed to create Snowflake client: %v", err))
		return nil, err
	}

	return &ResolvedProvider{
		Client:           sfClient,
		CacheKey:         cacheKey,
		Name:             pc.Name,
		Account:          pc.Spec.Account,
		AllowedDatabases: pc.Spec.AllowedDatabases,
		AllowedSchemas:   pc.Spec.AllowedSchemas,
	}, nil
}

// isNamespacePermitted checks whether a namespace is permitted to reference a
// ProviderConfig, evaluating both the static AllowedNamespaces list and the
// AllowedNamespaceSelector label selector with OR semantics.
//
// Rules:
//   - Neither AllowedNamespaces nor AllowedNamespaceSelector set → allow all
//   - AllowedNamespaces matches (static list or "*") → allow
//   - AllowedNamespaceSelector matches namespace labels → allow
//   - Otherwise → deny
func isNamespacePermitted(
	ctx context.Context,
	c client.Client,
	spec *snowplanev1alpha1.ProviderConfigSpec,
	namespace string,
) (bool, error) {
	hasStaticList := len(spec.AllowedNamespaces) > 0
	hasSelector := spec.AllowedNamespaceSelector != nil

	// No restrictions configured — allow all (backward compatible).
	if !hasStaticList && !hasSelector {
		return true, nil
	}

	// Check static list (no API call needed).
	if hasStaticList {
		for _, ns := range spec.AllowedNamespaces {
			if ns == "*" || ns == namespace {
				return true, nil
			}
		}
	}

	// Check label selector (requires Namespace object lookup).
	if hasSelector {
		ns := &corev1.Namespace{}
		if err := c.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
			return false, fmt.Errorf("fetching namespace %q for selector evaluation: %w", namespace, err)
		}

		selector, err := metav1.LabelSelectorAsSelector(spec.AllowedNamespaceSelector)
		if err != nil {
			return false, fmt.Errorf("invalid AllowedNamespaceSelector: %w", err)
		}

		if selector.Matches(labels.Set(ns.Labels)) {
			return true, nil
		}
	}

	return false, nil
}

// IsDatabaseAllowed checks whether a resolved database name is permitted by
// the ProviderConfig's AllowedDatabases restriction.
func IsDatabaseAllowed(resolved *ResolvedProvider, database string) bool {
	if len(resolved.AllowedDatabases) == 0 {
		return true
	}

	for _, db := range resolved.AllowedDatabases {
		if db == "*" {
			return true
		}

		if strings.EqualFold(db, database) {
			return true
		}
	}

	return false
}

// IsSchemaAllowed checks whether a resolved schema name is permitted by
// the ProviderConfig's AllowedSchemas restriction. Entries may be:
//   - "SCHEMA" — matches the schema name in any database (case-insensitive)
//   - "DATABASE.SCHEMA" — matches the fully-qualified name (case-insensitive)
func IsSchemaAllowed(resolved *ResolvedProvider, database, schemaName string) bool {
	if len(resolved.AllowedSchemas) == 0 {
		return true
	}

	for _, entry := range resolved.AllowedSchemas {
		if entry == "*" {
			return true
		}

		if parts := strings.SplitN(entry, ".", 2); len(parts) == 2 {
			if strings.EqualFold(parts[0], database) && strings.EqualFold(parts[1], schemaName) {
				return true
			}
		} else {
			if strings.EqualFold(entry, schemaName) {
				return true
			}
		}
	}

	return false
}
