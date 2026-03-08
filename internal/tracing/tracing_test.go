package tracing

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestSetup_Disabled(t *testing.T) {
	t.Parallel()
	provider, err := Setup(context.Background(), Config{Enabled: false})
	require.NoError(t, err)
	assert.NotNil(t, provider)
	assert.NotNil(t, provider.Tracer())
	provider.Shutdown()
}

func TestSetup_DisabledSetsGlobalNoopProvider(t *testing.T) {
	provider, err := Setup(context.Background(), Config{Enabled: false})
	require.NoError(t, err)
	defer provider.Shutdown()
	tp := otel.GetTracerProvider()
	assert.IsType(t, noop.NewTracerProvider(), tp)
}

func TestProvider_Shutdown_NilTP(t *testing.T) {
	t.Parallel()
	p := &Provider{tracer: noop.NewTracerProvider().Tracer(TracerName)}
	p.Shutdown()
}

func TestStartSpan(t *testing.T) {
	t.Parallel()
	_, err := Setup(context.Background(), Config{Enabled: false})
	require.NoError(t, err)
	ctx, span := StartSpan(context.Background(), "test-span")
	assert.NotNil(t, ctx)
	assert.NotNil(t, span)
	span.End()
}

func TestReconcileAttrs(t *testing.T) {
	t.Parallel()
	attrs := ReconcileAttrs("Database", "default", "my-db")
	assert.Len(t, attrs, 3)
	assert.Equal(t, "snowplane.resource.type", string(attrs[0].Key))
	assert.Equal(t, "Database", attrs[0].Value.AsString())
	assert.Equal(t, "k8s.namespace", string(attrs[1].Key))
	assert.Equal(t, "default", attrs[1].Value.AsString())
	assert.Equal(t, "k8s.name", string(attrs[2].Key))
	assert.Equal(t, "my-db", attrs[2].Value.AsString())
}

func TestRecordError(t *testing.T) {
	t.Parallel()
	_, err := Setup(context.Background(), Config{Enabled: false})
	require.NoError(t, err)
	RecordError(context.Background(), fmt.Errorf("test error"))
}

func TestConfig_Defaults(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	assert.False(t, cfg.Enabled)
	assert.Empty(t, cfg.Endpoint)
	assert.Equal(t, 0.0, cfg.SamplingRatio)
	assert.False(t, cfg.Insecure)
}
