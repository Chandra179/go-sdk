package bootstrap

import (
	"context"
	"fmt"
	"net/http"

	"gosdk/internal/cfg"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// MetricsHandler exposes the Prometheus HTTP handler for the OTEL metrics
type MetricsHandler interface {
	http.Handler
}

// InitOtel initializes OpenTelemetry for traces and metrics.
// Note: Logs are handled by Docker log scraping to Loki, not OTLP.
// Returns a shutdown function and the Prometheus metrics handler for serving metrics via HTTP.
func InitOtel(ctx context.Context, obsCfg *cfg.OtelConfig, samplerRatio float64) (func(context.Context) error, MetricsHandler, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceNameKey.String(obsCfg.ServiceName)),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create resource: %w", err)
	}

	tracerProvider, err := setupTracing(ctx, obsCfg, res, samplerRatio)
	if err != nil {
		return nil, nil, fmt.Errorf("setup tracing: %w", err)
	}

	meterProvider, metricsHandler, err := setupMetrics(ctx, obsCfg, res)
	if err != nil {
		return nil, nil, fmt.Errorf("setup metrics: %w", err)
	}

	shutdown := func(ctx context.Context) error {
		var errs []error
		if err := meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("meter provider: %w", err))
		}
		if err := tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("tracer provider: %w", err))
		}
		if len(errs) > 0 {
			return fmt.Errorf("shutdown errors: %v", errs)
		}
		return nil
	}

	return shutdown, metricsHandler, nil
}

func setupMetrics(ctx context.Context, cfg *cfg.OtelConfig, res *resource.Resource) (*metric.MeterProvider, http.Handler, error) {
	// Create a custom Prometheus registry for OTEL metrics
	reg := prometheus.NewRegistry()

	// Create Prometheus exporter with the custom registry
	promExporter, err := otelprom.New(otelprom.WithRegisterer(reg))
	if err != nil {
		return nil, nil, fmt.Errorf("create prometheus exporter: %w", err)
	}

	// Create OTLP exporter (keep for other metrics)
	otlpExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	provider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(otlpExporter)),
		metric.WithReader(promExporter), // Add Prometheus reader
		metric.WithResource(res),
	)
	otel.SetMeterProvider(provider)

	// Create HTTP handler from the custom registry
	metricsHandler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})

	return provider, metricsHandler, nil
}

func setupTracing(ctx context.Context, cfg *cfg.OtelConfig, res *resource.Resource, samplerRatio float64) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(samplerRatio))

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(provider)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return provider, nil
}
