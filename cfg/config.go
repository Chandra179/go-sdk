package cfg

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type RedisConfig struct {
	Host     string
	Port     string
	Password string
}

type Oauth2Config struct {
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectUrl  string
	GoogleLogoutUrl    string
	JWTSecret          string
	JWTExpiration      time.Duration
	StateTimeout       time.Duration
}

type ObservabilityConfig struct {
	OTLPEndpoint string
	ServiceName  string
	SamplerRatio float64
}

type KafkaConfig struct {
	Brokers []string
}

type PostgresConfig struct {
	Host          string
	Port          string
	User          string
	Password      string
	DBName        string
	SSLMode       string
	MigrationPath string
}

type HTTPServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type Config struct {
	AppEnv          string
	Redis           RedisConfig
	Postgres        PostgresConfig
	OAuth2          Oauth2Config
	Observability   ObservabilityConfig
	Kafka           KafkaConfig
	HTTPServer      HTTPServerConfig
	ShutdownTimeout time.Duration
}

func Load() (*Config, error) {
	var errs []error

	appEnv := mustEnv("APP_ENV", &errs)
	host := mustEnv("REDIS_HOST", &errs)
	port := mustEnv("REDIS_PORT", &errs)
	password := mustEnv("REDIS_PASSWORD", &errs)

	// ==========
	// OAuth2
	// ==========
	googleClientID := mustEnv("GOOGLE_CLIENT_ID", &errs)
	googleClientSecret := mustEnv("GOOGLE_CLIENT_SECRET", &errs)
	googleRedirectUrl := mustEnv("GOOGLE_REDIRECT_URL", &errs)
	jwtSecret := mustEnv("JWT_SECRET", &errs)
	if len(jwtSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be at least 32 characters"))
	}
	jwtExpirationStr := mustEnv("JWT_EXPIRATION", &errs)
	jwtExpiration, err := time.ParseDuration(jwtExpirationStr)
	if err != nil {
		errs = append(errs, errors.New("invalid duration for JWT_EXPIRATION: "+jwtExpirationStr))
	}
	stateTimeoutStr := mustEnv("STATE_TIMEOUT", &errs)
	stateTimeout, err := time.ParseDuration(stateTimeoutStr)
	if err != nil {
		errs = append(errs, errors.New("invalid duration for STATE_TIMEOUT: "+stateTimeoutStr))
	}

	// ==========
	// Postgres
	// ==========
	pgHost := mustEnv("POSTGRES_HOST", &errs)
	pgPort := mustEnv("POSTGRES_PORT", &errs)
	pgUser := mustEnv("POSTGRES_USER", &errs)
	pgPassword := mustEnv("POSTGRES_PASSWORD", &errs)
	pgDB := mustEnv("POSTGRES_DB", &errs)
	pgSSL := mustEnv("POSTGRES_SSLMODE", &errs)
	migrationPath := mustEnv("MIGRATION_PATH", &errs)

	// ==========
	// Observability
	// ==========
	otlpEndpoint := mustEnv("OTEL_EXPORTER_OTLP_ENDPOINT", &errs)
	serviceName := mustEnv("OTEL_SERVICE_NAME", &errs)
	samplerRatioStr := mustEnv("OTEL_SAMPLER_RATIO", &errs)
	samplerRatio, err := strconv.ParseFloat(samplerRatioStr, 64)
	if err != nil {
		errs = append(errs, errors.New("invalid float for OTEL_SAMPLER_RATIO: "+samplerRatioStr))
	}

	// ==========
	// Kafka
	// ==========
	kafkaBrokersStr := mustEnv("KAFKA_BROKERS", &errs)
	kafkaBrokers := []string{kafkaBrokersStr}

	shutdownTimeoutStr := mustEnv("SHUTDOWN_TIMEOUT", &errs)
	shutdownTimeout, err := time.ParseDuration(shutdownTimeoutStr)
	if err != nil {
		errs = append(errs, errors.New("invalid duration for SHUTDOWN_TIMEOUT: "+shutdownTimeoutStr))
	}

	// ==========
	// HTTP Server
	// ==========
	httpPort := mustEnv("HTTP_PORT", &errs)
	httpReadTimeoutStr := mustEnv("HTTP_READ_TIMEOUT", &errs)
	httpReadTimeout, err := time.ParseDuration(httpReadTimeoutStr)
	if err != nil {
		errs = append(errs, errors.New("invalid duration for HTTP_READ_TIMEOUT: "+httpReadTimeoutStr))
	}
	httpWriteTimeoutStr := mustEnv("HTTP_WRITE_TIMEOUT", &errs)
	httpWriteTimeout, err := time.ParseDuration(httpWriteTimeoutStr)
	if err != nil {
		errs = append(errs, errors.New("invalid duration for HTTP_WRITE_TIMEOUT: "+httpWriteTimeoutStr))
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return &Config{
		AppEnv: appEnv,
		Redis: RedisConfig{
			Host:     host,
			Port:     port,
			Password: password,
		},
		OAuth2: Oauth2Config{
			GoogleClientID:     googleClientID,
			GoogleClientSecret: googleClientSecret,
			GoogleRedirectUrl:  googleRedirectUrl,
			JWTSecret:          jwtSecret,
			JWTExpiration:      jwtExpiration,
			StateTimeout:       stateTimeout,
		},
		Observability: ObservabilityConfig{
			OTLPEndpoint: otlpEndpoint,
			ServiceName:  serviceName,
			SamplerRatio: samplerRatio,
		},
		Postgres: PostgresConfig{
			Host:          pgHost,
			Port:          pgPort,
			User:          pgUser,
			Password:      pgPassword,
			DBName:        pgDB,
			SSLMode:       pgSSL,
			MigrationPath: migrationPath,
		},
		Kafka: KafkaConfig{
			Brokers: kafkaBrokers,
		},
		HTTPServer: HTTPServerConfig{
			Port:         httpPort,
			ReadTimeout:  httpReadTimeout,
			WriteTimeout: httpWriteTimeout,
		},
		ShutdownTimeout: shutdownTimeout,
	}, nil
}

// mustEnv appends error into slice instead of returning.
func mustEnv(key string, errs *[]error) string {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		*errs = append(*errs, errors.New("missing env: "+key))
	}
	return value
}
