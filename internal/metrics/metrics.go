// Package metrics provides Prometheus metrics for Snowflake operations.
package metrics

import (
	"fmt"
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

// Connection pool statistics gauges (from sql.DBStats).
var (
	// DBStatsMaxOpenConns reports the configured maximum open connections per provider.
	DBStatsMaxOpenConns = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_max_open_connections",
			Help:      "Maximum number of open connections to Snowflake per provider.",
		},
		[]string{"provider"},
	)

	// DBStatsOpenConns reports the current number of open connections per provider.
	DBStatsOpenConns = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_open_connections",
			Help:      "Number of established connections (in-use + idle) per provider.",
		},
		[]string{"provider"},
	)

	// DBStatsInUse reports the number of connections currently in use per provider.
	DBStatsInUse = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_in_use_connections",
			Help:      "Number of connections currently in use per provider.",
		},
		[]string{"provider"},
	)

	// DBStatsIdle reports the number of idle connections per provider.
	DBStatsIdle = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_idle_connections",
			Help:      "Number of idle connections per provider.",
		},
		[]string{"provider"},
	)

	// DBStatsWaitCount reports cumulative wait count for connections per provider.
	// This is a cumulative gauge (resets on pool recreation), not a counter.
	DBStatsWaitCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_wait_count",
			Help:      "Cumulative number of connections waited for per provider (resets on pool recreation).",
		},
		[]string{"provider"},
	)

	// DBStatsWaitDuration reports cumulative wait duration for connections per provider.
	// This is a cumulative gauge (resets on pool recreation), not a counter.
	DBStatsWaitDuration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_wait_duration_seconds",
			Help:      "Cumulative time blocked waiting for a new connection per provider (resets on pool recreation).",
		},
		[]string{"provider"},
	)
)

// RecordDBStats publishes sql.DBStats for a given provider.
func RecordDBStats(provider string, maxOpen, open, inUse, idle int, waitCount int64, waitDuration time.Duration) {
	labels := prometheus.Labels{"provider": provider}
	DBStatsMaxOpenConns.With(labels).Set(float64(maxOpen))
	DBStatsOpenConns.With(labels).Set(float64(open))
	DBStatsInUse.With(labels).Set(float64(inUse))
	DBStatsIdle.With(labels).Set(float64(idle))
	DBStatsWaitCount.With(labels).Set(float64(waitCount))
	DBStatsWaitDuration.With(labels).Set(waitDuration.Seconds())
}

// SnowflakeRateLimitWaits counts how many times a reconciler waited for rate limit.
var SnowflakeRateLimitWaits = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "rate_limit_waits_total",
		Help:      "Total number of times a reconciler waited for the per-controller Snowflake rate limiter.",
	},
	[]string{"controller"},
)

// SnowflakeAccountRateLimitWaits counts how many times a reconciler waited
// for the per-account (aggregate) rate limiter. A non-zero value means the
// aggregate QPS across all controllers for the given provider (Snowflake
// account) is hitting the configured cap.
var SnowflakeAccountRateLimitWaits = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "account_rate_limit_waits_total",
		Help:      "Total number of times a reconciler waited for the per-account aggregate Snowflake rate limiter.",
	},
	[]string{"provider"},
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

// OrphanedResourcesTotal counts resources orphaned during deletion due to
// provider resolution failure. These Snowflake resources may still exist
// and require manual cleanup.
var OrphanedResourcesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "orphaned_resources_total",
		Help:      "Total number of Snowflake resources orphaned during deletion (provider resolution failed).",
	},
	[]string{"controller"},
)

// OwnershipConflictsTotal counts adoption attempts that were rejected because
// another CR in the same cluster already manages the same Snowflake resource.
var OwnershipConflictsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "ownership_conflicts_total",
		Help:      "Total number of adoption attempts rejected due to same-cluster ownership conflict.",
	},
	[]string{"controller"},
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

// LateInitTotal counts late-initialization events by controller and result.
var LateInitTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "late_init_total",
		Help:      "Total number of late-initialization events (spec enriched from observed Snowflake state).",
	},
	[]string{"controller", "result"},
)

// PreflightFailuresTotal counts pre-flight check failures by controller and reason.
var PreflightFailuresTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "preflight_failures_total",
		Help:      "Total number of pre-flight check failures that delay reconciliation.",
	},
	[]string{"controller", "reason"},
)

// SnowflakeErrorCodesTotal counts Snowflake errors by provider and error code.
var SnowflakeErrorCodesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "snowflake_error_codes_total",
		Help:      "Total number of Snowflake errors by error code (e.g. 2002=already exists, 3001=not authorized).",
	},
	[]string{"provider", "code"},
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

// SQLStatementExecutionsTotal counts SQLStatement executions by namespace, operation, and result.
// This is an audit metric for tracking arbitrary SQL execution via the escape-hatch CRD (H1).
var SQLStatementExecutionsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "sql_statement_executions_total",
		Help:      "Total number of SQLStatement SQL executions (audit trail for arbitrary SQL).",
	},
	[]string{"namespace", "operation"},
)

// SQLStatementDeniedTotal counts SQLStatement executions blocked by the
// statement denylist (H1 hardening). Labels: namespace, operation (execute/revert).
var SQLStatementDeniedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "sql_statement_denied_total",
		Help:      "Total number of SQLStatement executions blocked by the statement denylist.",
	},
	[]string{"namespace", "operation"},
)

// PolicyBodyRejectionsTotal counts policy body validation rejections by
// the denylist in ValidatePolicyBody (H2 hardening). Labels: controller.
var PolicyBodyRejectionsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "policy_body_rejections_total",
		Help:      "Total number of policy body validation rejections (denylist matches in masking/row-access policy bodies).",
	},
	[]string{"controller"},
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

// RecordOrphanedResource records a resource orphaned during deletion because
// the provider could not be resolved.
func RecordOrphanedResource(controller string) {
	OrphanedResourcesTotal.With(prometheus.Labels{"controller": controller}).Inc()
}

// RecordOwnershipConflict records an adoption attempt rejected because another
// CR already manages the same Snowflake resource.
func RecordOwnershipConflict(controller string) {
	OwnershipConflictsTotal.With(prometheus.Labels{"controller": controller}).Inc()
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

// RecordLateInit records a late-initialization event.
func RecordLateInit(controller string, modified bool) {
	result := "noop"
	if modified {
		result = "modified"
	}

	LateInitTotal.With(prometheus.Labels{"controller": controller, "result": result}).Inc()
}

// RecordPreflightFailure records a pre-flight check failure.
func RecordPreflightFailure(controller, reason string) {
	PreflightFailuresTotal.With(prometheus.Labels{"controller": controller, "reason": reason}).Inc()
}

// RecordSnowflakeErrorCode records a Snowflake error by code.
func RecordSnowflakeErrorCode(provider string, code int) {
	SnowflakeErrorCodesTotal.With(prometheus.Labels{"provider": provider, "code": fmt.Sprintf("%d", code)}).Inc()
}

// RecordSQLStatementExecution records an SQLStatement execution event.
// operation is one of: "execute", "revert".
func RecordSQLStatementExecution(namespace, operation string) {
	SQLStatementExecutionsTotal.With(prometheus.Labels{"namespace": namespace, "operation": operation}).Inc()
}

// RecordSQLStatementDenied records an SQLStatement execution blocked by the denylist.
// operation is one of: "execute", "revert".
func RecordSQLStatementDenied(namespace, operation string) {
	SQLStatementDeniedTotal.With(prometheus.Labels{"namespace": namespace, "operation": operation}).Inc()
}

// RecordPolicyBodyRejection records a policy body denylist rejection.
func RecordPolicyBodyRejection(controller string) {
	PolicyBodyRejectionsTotal.With(prometheus.Labels{"controller": controller}).Inc()
}

// NULBytesStrippedTotal counts NUL byte removals by the SQL escaping layer.
// A non-zero value may indicate binary injection attempts or encoding issues.
var NULBytesStrippedTotal = prometheus.NewCounter(
	prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "nul_bytes_stripped_total",
		Help:      "Total number of NUL bytes stripped from SQL strings. Non-zero may indicate injection attempts.",
	},
)

// RecordNULBytesStripped increments the counter by the number of NUL bytes removed.
func RecordNULBytesStripped(count int) {
	NULBytesStrippedTotal.Add(float64(count))
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

// ShardInfo is a gauge that exposes this manager's shard configuration as
// constant Prometheus labels. Value is always 1 when set.
var ShardInfo = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "shard_info",
		Help:      "Shard configuration for this controller manager instance. Value is always 1.",
	},
	[]string{"shard_id", "shard_count"},
)

// SetShardInfo publishes the shard configuration so Prometheus scrapes can
// identify which shard this replica owns. Call once during startup.
func SetShardInfo(shardID, shardCount int) {
	ShardInfo.With(prometheus.Labels{
		"shard_id":    fmt.Sprintf("%d", shardID),
		"shard_count": fmt.Sprintf("%d", shardCount),
	}).Set(1)
}

// RecordReconcile emits the reconcile result counter.
// result is one of: "success", "error", "terminal".
func RecordReconcile(controller string, result string) {
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
		SnowflakeAccountRateLimitWaits,
		AdoptionTotal,
		DriftDetectedTotal,
		OrphanedResourcesTotal,
		OwnershipConflictsTotal,
		CircuitBreakerTripsTotal,
		CircuitBreakerState,
		LateInitTotal,
		PreflightFailuresTotal,
		SnowflakeErrorCodesTotal,
		ProviderConfigHealthy,
		SQLStatementExecutionsTotal,
		SQLStatementDeniedTotal,
		PolicyBodyRejectionsTotal,
		NULBytesStrippedTotal,
		ShardInfo,
		DBStatsMaxOpenConns,
		DBStatsOpenConns,
		DBStatsInUse,
		DBStatsIdle,
		DBStatsWaitCount,
		DBStatsWaitDuration,
	)
}
