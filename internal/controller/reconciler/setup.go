package reconciler

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/hupe1980/snowplane/internal/circuitbreaker"
)

// SetupConfig bundles shared controller configuration for [GenericReconciler.Setup].
// Adding a new shared option requires only a struct field here and one line in Setup —
// no changes to main.go or individual controller packages.
type SetupConfig struct {
	// Manager is the controller-runtime manager used for registration.
	Manager ctrl.Manager

	// CircuitBreaker provides per-provider failure isolation.
	CircuitBreaker *circuitbreaker.Breaker

	// RequeueInterval is the periodic-resync interval for drift detection.
	RequeueInterval time.Duration

	// SnowflakeOpTimeout is the per-operation timeout for Snowflake CRUD calls.
	// Zero means DefaultSnowflakeOpTimeout (60s).
	SnowflakeOpTimeout time.Duration

	// Maturity is the maturity classification (alpha, beta, stable).
	Maturity string

	// AlphaEnabled controls whether alpha-maturity controllers are registered.
	AlphaEnabled bool

	// Disabled explicitly prevents this controller from registering.
	Disabled bool

	// MaxConcurrentReconciles is the maximum number of concurrent reconciles
	// per controller.
	MaxConcurrentReconciles int

	// ShardPredicate is an optional predicate that restricts this controller
	// to a deterministic subset of objects for horizontal sharding.
	// When nil, all objects are reconciled (single-instance mode).
	ShardPredicate predicate.Predicate
}

// Registerable is the interface satisfied by every GenericReconciler[T, S, D] instance.
// It allows type-erased controller registration without knowing the concrete
// CRD and service type parameters.
type Registerable interface {
	Setup(SetupConfig) error
}

// Setup applies shared configuration and registers the controller with the manager.
// It is the single entry-point for controller wiring, replacing the repeated
// With*().With*().SetupWithManager() chains.
func (r *GenericReconciler[T, S, D]) Setup(cfg SetupConfig) error {
	if cfg.ShardPredicate != nil {
		r.shardPredicate = cfg.ShardPredicate
	}

	return r.
		WithCircuitBreaker(cfg.CircuitBreaker).
		WithRequeueInterval(cfg.RequeueInterval).
		WithSnowflakeOpTimeout(cfg.SnowflakeOpTimeout).
		WithMaturity(cfg.Maturity).
		WithAlphaEnabled(cfg.AlphaEnabled).
		WithDisabled(cfg.Disabled).
		SetupWithManager(cfg.Manager, cfg.MaxConcurrentReconciles)
}
