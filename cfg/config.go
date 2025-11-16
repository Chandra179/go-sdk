package cfg

import (
	"errors"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
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
	GithubClientID     string
	GithubClientSecret string
	GithubRedirectUrl  string
	JWTSecret          string
	JWTExpiration      time.Duration
	StateTimeout       time.Duration
}

type ObservabilityConfig struct {
	OTLPEndpoint string
	ServiceName  string
	Environment  string
}

type Config struct {
	AppEnv        string
	Redis         RedisConfig
	OAuth2        Oauth2Config
	Observability ObservabilityConfig
}

func Load() (*Config, error) {
	var errs []error

	err := godotenv.Load() // ignore if .env missing (local only)
	if err != nil {
		log.Print("skip error godot load env")
	}

	// App
	appEnv := mustEnv("APP_ENV", &errs)

	// Redis
	host := mustEnv("REDIS_HOST", &errs)
	port := mustEnv("REDIS_PORT", &errs)
	password := getEnvOrDefault("REDIS_PASSWORD", "")

	// OAuth2
	googleClientID := mustEnv("GOOGLE_CLIENT_ID", &errs)
	googleClientSecret := mustEnv("GOOGLE_CLIENT_SECRET", &errs)
	googleRedirectUrl := mustEnv("GOOGLE_REDIRECT_URL", &errs)
	githubClientID := mustEnv("GITHUB_CLIENT_ID", &errs)
	githubClientSecret := mustEnv("GITHUB_CLIENT_SECRET", &errs)
	githubRedirectUrl := mustEnv("GITHUB_REDIRECT_URL", &errs)
	jwtSecret := mustEnv("JWT_SECRET", &errs)

	// Parse durations, defaulting to some reasonable value if missing
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

	// Observability (optional with defaults)
	otlpEndpoint := getEnvOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "alloy.observability.svc.cluster.local:4317")
	serviceName := getEnvOrDefault("OTEL_SERVICE_NAME", "gosdk-app")
	environment := getEnvOrDefault("OTEL_RESOURCE_ATTRIBUTES", appEnv)

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
			GithubClientID:     githubClientID,
			GithubClientSecret: githubClientSecret,
			GithubRedirectUrl:  githubRedirectUrl,
			JWTSecret:          jwtSecret,
			JWTExpiration:      jwtExpiration,
			StateTimeout:       stateTimeout,
		},
		Observability: ObservabilityConfig{
			OTLPEndpoint: otlpEndpoint,
			ServiceName:  serviceName,
			Environment:  environment,
		},
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
