package bootstrap

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrometheusExporter_Creation(t *testing.T) {
	ctx := context.Background()

	// Create a test resource
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceNameKey.String("test-service")),
	)
	require.NoError(t, err)

	// Act - Test Prometheus exporter creation in isolation
	promExporter, err := prometheus.New()

	// Assert - Prometheus exporter should work without OTLP
	require.NoError(t, err)
	assert.NotNil(t, promExporter)

	// Test that we can create a meter provider with just Prometheus exporter
	provider := metric.NewMeterProvider(
		metric.WithReader(promExporter),
		metric.WithResource(res),
	)
	assert.NotNil(t, provider)

	// Verify the meter provider works with Prometheus
	meter := provider.Meter("test-meter")
	assert.NotNil(t, meter)

	// Test that we can create metrics through Prometheus
	counter, err := meter.Int64Counter("test_counter")
	require.NoError(t, err)
	assert.NotNil(t, counter)

	// Test metric recording
	counter.Add(ctx, 1)

	// Cleanup
	err = provider.Shutdown(ctx)
	assert.NoError(t, err)
}
