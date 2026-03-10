package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collectCounter(c prometheus.Counter) float64 {
	m := &dto.Metric{}
	_ = c.Write(m)

	return m.GetCounter().GetValue()
}

func collectHistogramCount(o prometheus.Observer) uint64 {
	h, ok := o.(prometheus.Histogram)
	if !ok {
		return 0
	}

	m := &dto.Metric{}
	_ = h.Write(m)

	return m.GetHistogram().GetSampleCount()
}

func collectGauge(g prometheus.Gauge) float64 {
	m := &dto.Metric{}
	_ = g.Write(m)

	return m.GetGauge().GetValue()
}

func TestReconcileTotal_Inc(t *testing.T) {
	t.Parallel()

	c := ReconcileTotal.With(prometheus.Labels{"controller": "test-rt", "result": "success"})
	c.Inc()

	assert.GreaterOrEqual(t, collectCounter(c), float64(1))
}

func TestReconcileDuration_Observe(t *testing.T) {
	t.Parallel()

	o := ReconcileDuration.With(prometheus.Labels{"controller": "test-rd"})
	o.Observe(1.5)

	assert.GreaterOrEqual(t, collectHistogramCount(o), uint64(1))
}

func TestSnowflakeOperationTotal_Inc(t *testing.T) {
	t.Parallel()

	c := SnowflakeOperationTotal.With(prometheus.Labels{
		"controller": "test-sot",
		"operation":  "create",
		"result":     "success",
	})
	c.Inc()

	assert.GreaterOrEqual(t, collectCounter(c), float64(1))
}

func TestSnowflakeOperationDuration_Observe(t *testing.T) {
	t.Parallel()

	o := SnowflakeOperationDuration.With(prometheus.Labels{
		"controller": "test-sod",
		"operation":  "observe",
	})
	o.Observe(0.5)

	assert.GreaterOrEqual(t, collectHistogramCount(o), uint64(1))
}

func TestClientPoolSize_SetAndGet(t *testing.T) {
	t.Parallel()

	ClientPoolSize.Set(5)
	assert.Equal(t, float64(5), collectGauge(ClientPoolSize))
}

func TestSnowflakeRateLimitWaits_Inc(t *testing.T) {
	t.Parallel()

	c := SnowflakeRateLimitWaits.With(prometheus.Labels{"controller": "test-rlw"})
	c.Inc()

	assert.GreaterOrEqual(t, collectCounter(c), float64(1))
}

func TestSnowflakeAccountRateLimitWaits_Inc(t *testing.T) {
	t.Parallel()

	c := SnowflakeAccountRateLimitWaits.With(prometheus.Labels{"provider": "test-arlw"})
	c.Inc()

	assert.GreaterOrEqual(t, collectCounter(c), float64(1))
}

func TestAdoptionTotal_RecordAdoption(t *testing.T) {
	t.Parallel()

	RecordAdoption("test-adopt")
	c := AdoptionTotal.With(prometheus.Labels{"controller": "test-adopt", "result": "adopted"})
	assert.GreaterOrEqual(t, collectCounter(c), float64(1))
}

func TestAdoptionTotal_RecordAdoptionRejected(t *testing.T) {
	t.Parallel()

	RecordAdoptionRejected("test-reject")
	c := AdoptionTotal.With(prometheus.Labels{"controller": "test-reject", "result": "rejected"})
	assert.GreaterOrEqual(t, collectCounter(c), float64(1))
}

func TestDriftDetectedTotal_RecordDriftDetected(t *testing.T) {
	t.Parallel()

	RecordDriftDetected("test-drift")
	c := DriftDetectedTotal.With(prometheus.Labels{"controller": "test-drift"})
	assert.GreaterOrEqual(t, collectCounter(c), float64(1))
}

func TestMetricLabels(t *testing.T) {
	t.Parallel()

	controllers := []string{"database", "schema", "providerconfig"}
	operations := []string{"observe", "create", "alter", "drop", "ping"}
	results := []string{"success", "error"}

	for _, c := range controllers {
		ReconcileDuration.With(prometheus.Labels{"controller": c})
		SnowflakeRateLimitWaits.With(prometheus.Labels{"controller": c})
		DriftDetectedTotal.With(prometheus.Labels{"controller": c})

		for _, r := range results {
			ReconcileTotal.With(prometheus.Labels{"controller": c, "result": r})

			for _, o := range operations {
				SnowflakeOperationTotal.With(prometheus.Labels{
					"controller": c,
					"operation":  o,
					"result":     r,
				})
			}
		}

		for _, o := range operations {
			SnowflakeOperationDuration.With(prometheus.Labels{
				"controller": c,
				"operation":  o,
			})
		}

		// Adoption metric labels.
		for _, r := range results {
			AdoptionTotal.With(prometheus.Labels{"controller": c, "result": r})
		}

	}
}

func TestMetricDescriptions(t *testing.T) {
	t.Parallel()

	ch := make(chan *prometheus.Desc, 10)
	ReconcileTotal.Describe(ch)
	require.Len(t, ch, 1)

	ch2 := make(chan *prometheus.Desc, 10)
	SnowflakeOperationDuration.Describe(ch2)
	require.Len(t, ch2, 1)
}

func TestCircuitBreakerTripsTotal(t *testing.T) {
	t.Parallel()

	RecordCircuitBreakerTrip("test-cb-provider")
	c := CircuitBreakerTripsTotal.With(prometheus.Labels{"provider": "test-cb-provider"})
	assert.GreaterOrEqual(t, collectCounter(c), float64(1))
}

func TestCircuitBreakerState_SetAndGet(t *testing.T) {
	t.Parallel()

	SetCircuitBreakerState("test-cb-state", 0)
	g := CircuitBreakerState.With(prometheus.Labels{"provider": "test-cb-state"})
	assert.Equal(t, float64(0), collectGauge(g))

	SetCircuitBreakerState("test-cb-state", 1)
	assert.Equal(t, float64(1), collectGauge(g))

	SetCircuitBreakerState("test-cb-state", 2)
	assert.Equal(t, float64(2), collectGauge(g))
}

func TestProviderConfigHealthy_SetAndGet(t *testing.T) {
	t.Parallel()

	SetProviderConfigHealthy("test-pc-healthy", "acct1", true)
	g := ProviderConfigHealthy.With(prometheus.Labels{"provider": "test-pc-healthy", "account": "acct1"})
	assert.Equal(t, float64(1), collectGauge(g))

	SetProviderConfigHealthy("test-pc-healthy", "acct1", false)
	assert.Equal(t, float64(0), collectGauge(g))
}

func TestProviderConfigHealthy_MultipleProviders(t *testing.T) {
	t.Parallel()

	SetProviderConfigHealthy("pc-multi-a", "acct-a", true)
	SetProviderConfigHealthy("pc-multi-b", "acct-b", false)

	gA := ProviderConfigHealthy.With(prometheus.Labels{"provider": "pc-multi-a", "account": "acct-a"})
	gB := ProviderConfigHealthy.With(prometheus.Labels{"provider": "pc-multi-b", "account": "acct-b"})
	assert.Equal(t, float64(1), collectGauge(gA))
	assert.Equal(t, float64(0), collectGauge(gB))
}

func TestDeleteProviderConfigHealthy(t *testing.T) {
	t.Parallel()

	SetProviderConfigHealthy("pc-delete-test", "acct-del", true)
	g := ProviderConfigHealthy.With(prometheus.Labels{"provider": "pc-delete-test", "account": "acct-del"})
	assert.Equal(t, float64(1), collectGauge(g))

	DeleteProviderConfigHealthy("pc-delete-test")
	// After deletion, getting the metric creates a new zero-value gauge
	g2 := ProviderConfigHealthy.With(prometheus.Labels{"provider": "pc-delete-test", "account": "acct-del"})
	assert.Equal(t, float64(0), collectGauge(g2))
}

func TestRecordLateInit_Modified(t *testing.T) {
	t.Parallel()

	RecordLateInit("test-late-init-mod", true)
	c := LateInitTotal.With(prometheus.Labels{"controller": "test-late-init-mod", "result": "modified"})
	assert.GreaterOrEqual(t, collectCounter(c), float64(1))
}

func TestRecordLateInit_Noop(t *testing.T) {
	t.Parallel()

	RecordLateInit("test-late-init-noop", false)
	c := LateInitTotal.With(prometheus.Labels{"controller": "test-late-init-noop", "result": "noop"})
	assert.GreaterOrEqual(t, collectCounter(c), float64(1))
}

func TestRecordPreflightFailure(t *testing.T) {
	t.Parallel()

	RecordPreflightFailure("test-preflight", "DependencyNotReady")
	c := PreflightFailuresTotal.With(prometheus.Labels{"controller": "test-preflight", "reason": "DependencyNotReady"})
	assert.GreaterOrEqual(t, collectCounter(c), float64(1))
}

func TestRecordSnowflakeErrorCode(t *testing.T) {
	t.Parallel()

	RecordSnowflakeErrorCode("test-sf-err", 2002)
	c := SnowflakeErrorCodesTotal.With(prometheus.Labels{"provider": "test-sf-err", "code": "2002"})
	assert.GreaterOrEqual(t, collectCounter(c), float64(1))
}

func TestRecordSQLStatementExecution_Execute(t *testing.T) {
	t.Parallel()

	RecordSQLStatementExecution("test-ns-exec", "execute")
	c := SQLStatementExecutionsTotal.With(prometheus.Labels{"namespace": "test-ns-exec", "operation": "execute"})
	assert.GreaterOrEqual(t, collectCounter(c), float64(1))
}

func TestRecordSQLStatementExecution_Revert(t *testing.T) {
	t.Parallel()

	RecordSQLStatementExecution("test-ns-revert", "revert")
	c := SQLStatementExecutionsTotal.With(prometheus.Labels{"namespace": "test-ns-revert", "operation": "revert"})
	assert.GreaterOrEqual(t, collectCounter(c), float64(1))
}

func TestSetShardInfo(t *testing.T) {
	t.Parallel()

	SetShardInfo(2, 5)
	g := ShardInfo.With(prometheus.Labels{"shard_id": "2", "shard_count": "5"})
	assert.Equal(t, float64(1), collectGauge(g))
}

func TestSetShardInfo_SingleInstance(t *testing.T) {
	t.Parallel()

	SetShardInfo(0, 1)
	g := ShardInfo.With(prometheus.Labels{"shard_id": "0", "shard_count": "1"})
	assert.Equal(t, float64(1), collectGauge(g))
}
