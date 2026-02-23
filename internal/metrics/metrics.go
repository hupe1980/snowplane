// Package metrics provides Prometheus metrics for Snowflake operations.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	namespace = "snowplane"
)

// ReconcileTotal counts total reconciliation attempts by controller and result.
var ReconcileTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "reconcile_total",
		Help:      "Total number of reconciliation attempts.",
	},
	[]string{"controller", "result"},
)

// ReconcileDuration observes the duration of each reconciliation loop.
var ReconcileDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "reconcile_duration_seconds",
		Help:      "Duration of reconciliation loops in seconds.",
		Buckets:   prometheus.DefBuckets,
	},
	[]string{"controller"},
)

// SnowflakeOperationTotal counts Snowflake API calls by controller and operation.
var SnowflakeOperationTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "snowflake_operation_total",
		Help:      "Total number of Snowflake API operations.",
	},
	[]string{"controller", "operation", "result"},
)

// SnowflakeOperationDuration observes the duration of Snowflake API calls.
var SnowflakeOperationDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "snowflake_operation_duration_seconds",
		Help:      "Duration of Snowflake API operations in seconds.",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
	},
	[]string{"controller", "operation"},
)

// ClientPoolSize tracks the number of active Snowflake clients.
var ClientPoolSize = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "client_pool_size",
		Help:      "Number of active Snowflake clients in the connection pool.",
	},
)

// SnowflakeRateLimitWaits counts how many times a reconciler waited for rate limit.
var SnowflakeRateLimitWaits = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "rate_limit_waits_total",
		Help:      "Total number of times a reconciler waited for the Snowflake rate limiter.",
	},
	[]string{"controller"},
)

// AdoptionTotal counts resource adoption outcomes by controller and result.
var AdoptionTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "adoption_total",
		Help:      "Total number of resource adoption attempts by outcome.",
	},
	[]string{"controller", "result"},
)

// DriftDetectedTotal counts drift detection events by controller.
var DriftDetectedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "drift_detected_total",
		Help:      "Total number of drift detection events.",
	},
	[]string{"controller"},
)

// ManagedResources tracks the number of managed resources by controller and state.
var ManagedResources = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "managed_resources",
		Help:      "Number of managed resources by controller and state (ready, not_ready, terminal).",
	},
	[]string{"controller", "state"},
)

// CircuitBreakerTripsTotal counts the number of times a circuit breaker has
// transitioned to the open state for a provider.
var CircuitBreakerTripsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "circuit_breaker_trips_total",
		Help:      "Total number of circuit breaker trips (transitions to open state) per provider.",
	},
	[]string{"provider"},
)

// CircuitBreakerState reports the current circuit breaker state per provider
// (0 = closed, 1 = open, 2 = half-open).
var CircuitBreakerState = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "circuit_breaker_state",
		Help:      "Current circuit breaker state per provider (0=closed, 1=open, 2=half-open).",
	},
	[]string{"provider"},
)

// ProviderConfigHealthy reports whether each ProviderConfig is healthy
// (1 = connected/healthy, 0 = unhealthy/disconnected).
var ProviderConfigHealthy = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "providerconfig_healthy",
		Help:      "Whether a ProviderConfig is healthy (1=connected, 0=unhealthy).",
	},
	[]string{"provider", "account"},
)

// RecordAdoption records a successful resource adoption.
func RecordAdoption(controller string) {
	AdoptionTotal.With(prometheus.Labels{"controller": controller, "result": "adopted"}).Inc()
}

// RecordAdoptionRejected records a rejected adoption (resource already exists, no adopt annotation).
func RecordAdoptionRejected(controller string) {
	AdoptionTotal.With(prometheus.Labels{"controller": controller, "result": "rejected"}).Inc()
}

// RecordDriftDetected records a drift detection event.
func RecordDriftDetected(controller string) {
	DriftDetectedTotal.With(prometheus.Labels{"controller": controller}).Inc()
}

// SetManagedResources updates the managed resources gauge for a controller.
func SetManagedResources(controller, state string, count float64) {
	ManagedResources.With(prometheus.Labels{"controller": controller, "state": state}).Set(count)
}

// RecordCircuitBreakerTrip increments the trip counter for a provider.
func RecordCircuitBreakerTrip(provider string) {
	CircuitBreakerTripsTotal.With(prometheus.Labels{"provider": provider}).Inc()
}

// SetCircuitBreakerState updates the state gauge for a provider.
// Values: 0 = closed, 1 = open, 2 = half-open.
func SetCircuitBreakerState(provider string, state float64) {
	CircuitBreakerState.With(prometheus.Labels{"provider": provider}).Set(state)
}

// SetProviderConfigHealthy updates the health gauge for a ProviderConfig.
// Values: 1 = healthy, 0 = unhealthy.
func SetProviderConfigHealthy(provider, account string, healthy bool) {
	val := float64(0)
	if healthy {
		val = 1
	}

	ProviderConfigHealthy.With(prometheus.Labels{"provider": provider, "account": account}).Set(val)
}

// DeleteProviderConfigHealthy removes the health gauge for a ProviderConfig
// that has been deleted, preventing stale metrics.
func DeleteProviderConfigHealthy(provider string) {
	ProviderConfigHealthy.DeletePartialMatch(prometheus.Labels{"provider": provider})
}

// ObserveSnowflakeOp instruments a Snowflake operation with duration and result metrics.
// The callback fn performs the actual operation; any error it returns is propagated.
func ObserveSnowflakeOp(controller, operation string, fn func() error) error {
	start := time.Now()
	err := fn()
	duration := time.Since(start).Seconds()

	result := "success"
	if err != nil {
		result = "error"
	}

	SnowflakeOperationTotal.With(prometheus.Labels{
		"controller": controller,
		"operation":  operation,
		"result":     result,
	}).Inc()
	SnowflakeOperationDuration.With(prometheus.Labels{
		"controller": controller,
		"operation":  operation,
	}).Observe(duration)

	return err
}

// RecordReconcile emits the reconcile result counter.
func RecordReconcile(controller string, err error) {
	result := "success"
	if err != nil {
		result = "error"
	}

	ReconcileTotal.With(prometheus.Labels{"controller": controller, "result": result}).Inc()
}

func init() {
	metrics.Registry.MustRegister(
		ReconcileTotal,
		ReconcileDuration,
		SnowflakeOperationTotal,
		SnowflakeOperationDuration,
		ClientPoolSize,
		SnowflakeRateLimitWaits,
		AdoptionTotal,
		DriftDetectedTotal,
		ManagedResources,
		CircuitBreakerTripsTotal,
		CircuitBreakerState,
		ProviderConfigHealthy,
	)
}
