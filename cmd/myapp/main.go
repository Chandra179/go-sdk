package main

import (
	"context"
	"fmt"
	"gosdk/cfg"
	"gosdk/pkg/logger"
	"gosdk/pkg/oauth2"
	"log"
	"time"

	_ "gosdk/api" // swagger docs

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// initTracer initializes OpenTelemetry tracer with OTLP exporter
func initTracer(ctx context.Context, config *cfg.ObservabilityConfig) (func(context.Context) error, error) {
	conn, err := grpc.NewClient(
		config.OTLPEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection to collector: %w", err)
	}

	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
			semconv.DeploymentEnvironment(config.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	log.Printf("OpenTelemetry tracer initialized - sending to: %s", config.OTLPEndpoint)

	return tp.Shutdown, nil
}

// initMeter initializes OpenTelemetry meter with OTLP exporter
func initMeter(ctx context.Context, config *cfg.ObservabilityConfig) (func(context.Context) error, error) {
	conn, err := grpc.NewClient(
		config.OTLPEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection to collector: %w", err)
	}

	metricExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
			semconv.DeploymentEnvironment(config.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter)),
		metric.WithResource(res),
	)

	otel.SetMeterProvider(mp)

	log.Printf("OpenTelemetry meter initialized - sending to: %s", config.OTLPEndpoint)

	return mp.Shutdown, nil
}

// TraceLoggerMiddleware extracts trace_id and span_id from the request context and attaches it to logger
func TraceLoggerMiddleware(log logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		span := trace.SpanFromContext(c.Request.Context())
		if span.SpanContext().IsValid() {
			traceID := span.SpanContext().TraceID().String()
			spanID := span.SpanContext().SpanID().String()

			// Store trace info in context for later use
			c.Set("trace_id", traceID)
			c.Set("span_id", spanID)

			log.Info("incoming request",
				logger.Field{Key: "trace_id", Value: traceID},
				logger.Field{Key: "span_id", Value: spanID},
				logger.Field{Key: "method", Value: c.Request.Method},
				logger.Field{Key: "path", Value: c.Request.URL.Path},
			)
		}

		c.Next()

		if span.SpanContext().IsValid() {
			traceID := span.SpanContext().TraceID().String()
			spanID := span.SpanContext().SpanID().String()

			log.Info("request completed",
				logger.Field{Key: "trace_id", Value: traceID},
				logger.Field{Key: "span_id", Value: spanID},
				logger.Field{Key: "status", Value: c.Writer.Status()},
				logger.Field{Key: "method", Value: c.Request.Method},
				logger.Field{Key: "path", Value: c.Request.URL.Path},
			)
		}
	}
}

func main() {
	ctx := context.Background()

	// ============
	// config
	// ============
	config, errCfg := cfg.Load()
	if errCfg != nil {
		log.Fatal(errCfg)
	}

	// ============
	// logger
	// ============
	zlogger := logger.NewZeroLog(config.AppEnv)
	log.Printf("Logger initialized for environment: %s", config.AppEnv)

	// ============
	// OpenTelemetry Tracing
	// ============
	shutdownTracer, err := initTracer(ctx, &config.Observability)
	if err != nil {
		log.Printf("WARNING: failed to initialize tracer: %v", err)
		log.Printf("Continuing without tracing...")
	} else {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := shutdownTracer(shutdownCtx); err != nil {
				log.Printf("failed to shutdown tracer: %v", err)
			}
		}()
	}

	// ============
	// OpenTelemetry Metrics
	// ============
	shutdownMeter, err := initMeter(ctx, &config.Observability)
	if err != nil {
		log.Printf("WARNING: failed to initialize meter: %v", err)
		log.Printf("Continuing without OTLP metrics...")
	} else {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := shutdownMeter(shutdownCtx); err != nil {
				log.Printf("failed to shutdown meter: %v", err)
			}
		}()
	}

	// ============
	// cache
	// ============
	redisAddr := config.Redis.Host + ":" + config.Redis.Port
	// redisCache := cache.NewRedisCache(redisAddr)
	log.Printf("Redis cache initialized at: %s", redisAddr)

	// ============
	// oauth2
	// ============
	oauth2mgr, err := oauth2.NewManager(&config.OAuth2)
	if err != nil {
		log.Fatal(err)
	}

	r := gin.Default()

	// ============
	// Middleware
	// ============
	// OpenTelemetry instrumentation for tracing
	r.Use(otelgin.Middleware(config.Observability.ServiceName))
	// Trace logger middleware
	r.Use(TraceLoggerMiddleware(zlogger))

	// ============
	// Health check endpoint
	// ============
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "healthy",
			"service": config.Observability.ServiceName,
			"env":     config.AppEnv,
		})
	})

	// ============
	// Prometheus metrics endpoint
	// ============
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// ============
	// swagger & redoc
	// ============
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Serve Redoc
	r.GET("/docs", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		html := `<!DOCTYPE html>
<html>
<head>
    <title>API Documentation</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
    <script id="api-reference" data-url="/swagger/doc.json"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`
		c.String(200, html)
	})

	r.GET("/rapidoc", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		html := `<!DOCTYPE html>
<html>
<head>
    <title>API Documentation</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <script type="module" src="https://unpkg.com/rapidoc/dist/rapidoc-min.js"></script>
</head>
<body>
    <rapi-doc 
        spec-url="/swagger/doc.json"
        theme="dark"
        render-style="read"
        show-header="false"
        allow-try="true"
        allow-server-selection="true"
    ></rapi-doc>
</body>
</html>`
		c.String(200, html)
	})

	// ============
	// HTTP handler
	// ============
	auth := r.Group("/auth")
	{
		auth.GET("/google", oauth2.GoogleAuthHandler(oauth2mgr))
		auth.GET("/callback/google", oauth2.GoogleCallbackHandler(oauth2mgr))
		auth.GET("/github", oauth2.GithubAuthHandler(oauth2mgr))
		auth.GET("/callback/github", oauth2.GithubCallbackHandler(oauth2mgr))
	}

	api := r.Group("/api")
	api.GET("/me", oauth2.MeHandler(oauth2mgr))

	log.Printf("Starting server on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
