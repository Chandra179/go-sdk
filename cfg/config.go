package cfg

import (
	"errors"
	"os"
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
}

type KafkaConfig struct {
	Brokers []string
}

type PostgresConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
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
	password := getEnvOrDefault("REDIS_PASSWORD", "")

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
	pgSSL := getEnvOrDefault("POSTGRES_SSLMODE", "disable") // default disable

	// ==========
	// Observability
	// ==========
	otlpEndpoint := mustEnv("OTEL_EXPORTER_OTLP_ENDPOINT", &errs)
	serviceName := mustEnv("OTEL_SERVICE_NAME", &errs)

	// ==========
	// Kafka
	// ==========
	kafkaBrokersStr := getEnvOrDefault("KAFKA_BROKERS", "localhost:9092")
	kafkaBrokers := []string{kafkaBrokersStr}

	shutdownTimeoutStr := getEnvOrDefault("SHUTDOWN_TIMEOUT", "30s")
	shutdownTimeout, err := time.ParseDuration(shutdownTimeoutStr)
	if err != nil {
		errs = append(errs, errors.New("invalid duration for SHUTDOWN_TIMEOUT: "+shutdownTimeoutStr))
	}

	// ==========
	// HTTP Server
	// ==========
	httpPort := getEnvOrDefault("HTTP_PORT", "8080")
	httpReadTimeoutStr := getEnvOrDefault("HTTP_READ_TIMEOUT", "30s")
	httpReadTimeout, err := time.ParseDuration(httpReadTimeoutStr)
	if err != nil {
		errs = append(errs, errors.New("invalid duration for HTTP_READ_TIMEOUT: "+httpReadTimeoutStr))
	}
	httpWriteTimeoutStr := getEnvOrDefault("HTTP_WRITE_TIMEOUT", "30s")
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
		},
		Postgres: PostgresConfig{
			Host:     pgHost,
			Port:     pgPort,
			User:     pgUser,
			Password: pgPassword,
			DBName:   pgDB,
			SSLMode:  pgSSL,
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

// getEnvOrDefault returns environment variable value or default if not set.
func getEnvOrDefault(key, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		return defaultValue
	}
	return value
}
