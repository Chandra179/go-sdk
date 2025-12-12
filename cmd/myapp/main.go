package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"gosdk/cfg"
	"gosdk/internal/authservice"
	"gosdk/pkg/cache"
	"gosdk/pkg/db"
	"gosdk/pkg/logger"
	"gosdk/pkg/oauth2"
	"gosdk/pkg/session"

	_ "gosdk/api"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace" // <--- ALIASED HERE
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace" // <--- STANDARD API HERE
)

func main() {
	config, errCfg := cfg.Load()
	if errCfg != nil {
		log.Fatal(errCfg)
	}

	ctx := context.Background()
	shutdown, err := setupOTelSDK(ctx, &config.Observability)
	if err != nil {
		log.Fatalf("Error setting up OTel SDK: %v", err)
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			log.Fatalf("Error shutting down OTel SDK: %v", err)
		}
	}()

	appLogger := logger.NewLogger("dev")
	appLogger.Info(ctx, "Application starting up...")

	pg := config.Postgres
	pgDSN := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		pg.User, pg.Password, pg.Host, pg.Port, pg.DBName, pg.SSLMode,
	)

	dbClient, err := db.NewSQLClient("postgres", pgDSN)
	if err != nil {
		log.Fatal(err)
	}

	m, err := migrate.New("file://db/migrations", pgDSN)
	if err != nil {
		log.Fatal(err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatal(err)
	}

	redisAddr := config.Redis.Host + ":" + config.Redis.Port
	_ = cache.NewRedisCache(redisAddr)

	sessionStore := session.NewInMemoryStore()

	oauth2mgr, err := oauth2.NewManager(context.Background(), &config.OAuth2)
	if err != nil {
		log.Fatal(err)
	}

	authSvc := authservice.NewService(oauth2mgr, sessionStore, dbClient)
	authHandler := authservice.NewHandler(authSvc)
	oauth2mgr.CallbackHandler = func(ctx context.Context, provider string, userInfo *oauth2.UserInfo,
		tokenSet *oauth2.TokenSet) (*oauth2.CallbackInfo, error) {
		return authSvc.HandleCallback(ctx, provider, userInfo, tokenSet)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(otelgin.Middleware(config.Observability.ServiceName))
	r.Use(TraceLoggerMiddleware(appLogger))

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.GET("/docs", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		html := `<!DOCTYPE html>...`
		c.String(200, html)
	})

	auth := r.Group("/auth")
	{
		auth.POST("/login", authHandler.LoginHandler())
		auth.POST("/logout", authHandler.LogoutHandler())
		auth.GET("/callback/google", oauth2.GoogleCallbackHandler(oauth2mgr))
	}

	log.Printf("Server listening on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// setupOTelSDK bootstraps the OpenTelemetry pipeline using your config struct
func setupOTelSDK(ctx context.Context, obsCfg *cfg.ObservabilityConfig) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = fn(ctx)
		}
		shutdownFuncs = nil
		return err
	}

	handleErr := func(inErr error) {
		err = inErr
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(obsCfg.ServiceName),
		),
	)
	if err != nil {
		handleErr(err)
		return
	}

	// Helper options for all exporters
	// We point them to Alloy (obsCfg.OTLPEndpoint) and disable TLS (WithInsecure)
	// because strictly internal Docker networks usually don't have certs.
	// NOTE: Ensure obsCfg.OTLPEndpoint is "alloy:4317" (host:port), not "http://..."
	grpcOpts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(obsCfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(),
	}
	metricOpts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(obsCfg.OTLPEndpoint),
		otlpmetricgrpc.WithInsecure(),
	}
	logOpts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(obsCfg.OTLPEndpoint),
		otlploggrpc.WithInsecure(),
	}

	// Trace Exporter
	traceExporter, err := otlptracegrpc.New(ctx, grpcOpts...)
	if err != nil {
		handleErr(err)
		return
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)

	// Metric Exporter
	metricExporter, err := otlpmetricgrpc.New(ctx, metricOpts...)
	if err != nil {
		handleErr(err)
		return
	}
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter)),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)

	// Log Exporter
	logExporter, err := otlploggrpc.New(ctx, logOpts...)
	if err != nil {
		handleErr(err)
		return
	}
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(res),
	)
	global.SetLoggerProvider(loggerProvider)
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)

	return
}

type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func TraceLoggerMiddleware(l *logger.AppLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var reqBodyBytes []byte
		if c.Request.Body != nil {
			reqBodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(reqBodyBytes))
		}

		w := &responseBodyWriter{
			body:           bytes.NewBufferString(""),
			ResponseWriter: c.Writer,
		}
		c.Writer = w

		start := time.Now()
		c.Next()
		duration := time.Since(start)

		spanCtx := trace.SpanContextFromContext(c.Request.Context())
		traceID := ""
		if spanCtx.IsValid() {
			traceID = spanCtx.TraceID().String()
		}

		fields := []logger.Field{
			{Key: "trace_id", Value: traceID},
			{Key: "method", Value: c.Request.Method},
			{Key: "path", Value: c.Request.URL.Path},
			{Key: "status", Value: c.Writer.Status()},
			{Key: "latency", Value: duration.String()},
			{Key: "request_body", Value: string(reqBodyBytes)},
			{Key: "response_body", Value: w.body.String()},
		}

		msg := "HTTP Request"
		if c.Writer.Status() >= 500 {
			l.Error(c.Request.Context(), msg, fields...)
		} else {
			l.Info(c.Request.Context(), msg, fields...)
		}
	}
}
