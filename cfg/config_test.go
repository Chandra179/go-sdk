package cfg

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Success(t *testing.T) {
	envVars := map[string]string{
		"APP_ENV":                     "development",
		"REDIS_HOST":                  "localhost",
		"REDIS_PORT":                  "6379",
		"GOOGLE_CLIENT_ID":            "test-client-id",
		"GOOGLE_CLIENT_SECRET":        "test-client-secret",
		"GOOGLE_REDIRECT_URL":         "http://localhost:8080/callback",
		"JWT_SECRET":                  "this-is-a-very-long-secret-key-that-is-at-least-32-characters-long",
		"JWT_EXPIRATION":              "24h",
		"STATE_TIMEOUT":               "5m",
		"POSTGRES_HOST":               "localhost",
		"POSTGRES_PORT":               "5432",
		"POSTGRES_USER":               "testuser",
		"POSTGRES_PASSWORD":           "testpass",
		"POSTGRES_DB":                 "testdb",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4317",
		"OTEL_SERVICE_NAME":           "gosdk",
	}

	for key, value := range envVars {
		t.Setenv(key, value)
	}

	config, err := Load()
	require.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "development", config.AppEnv)
	assert.Equal(t, "localhost", config.Redis.Host)
	assert.Equal(t, "6379", config.Redis.Port)
}

func TestLoad_MissingRequiredEnv(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	for _, key := range []string{
		"REDIS_HOST",
		"REDIS_PORT",
		"GOOGLE_CLIENT_ID",
		"GOOGLE_CLIENT_SECRET",
		"GOOGLE_REDIRECT_URL",
		"JWT_SECRET",
		"JWT_EXPIRATION",
		"STATE_TIMEOUT",
		"POSTGRES_HOST",
		"POSTGRES_PORT",
		"POSTGRES_USER",
		"POSTGRES_PASSWORD",
		"POSTGRES_DB",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_SERVICE_NAME",
	} {
		os.Unsetenv(key)
	}

	_, err := Load()
	assert.Error(t, err)
}

func TestLoad_ShortJWTSecret(t *testing.T) {
	envVars := map[string]string{
		"APP_ENV":                     "development",
		"REDIS_HOST":                  "localhost",
		"REDIS_PORT":                  "6379",
		"GOOGLE_CLIENT_ID":            "test-client-id",
		"GOOGLE_CLIENT_SECRET":        "test-client-secret",
		"GOOGLE_REDIRECT_URL":         "http://localhost:8080/callback",
		"JWT_SECRET":                  "short",
		"JWT_EXPIRATION":              "24h",
		"STATE_TIMEOUT":               "5m",
		"POSTGRES_HOST":               "localhost",
		"POSTGRES_PORT":               "5432",
		"POSTGRES_USER":               "testuser",
		"POSTGRES_PASSWORD":           "testpass",
		"POSTGRES_DB":                 "testdb",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4317",
		"OTEL_SERVICE_NAME":           "gosdk",
	}

	for key, value := range envVars {
		t.Setenv(key, value)
	}

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 32 characters")
}

func TestLoad_ValidJWTSecret(t *testing.T) {
	envVars := map[string]string{
		"APP_ENV":                     "development",
		"REDIS_HOST":                  "localhost",
		"REDIS_PORT":                  "6379",
		"GOOGLE_CLIENT_ID":            "test-client-id",
		"GOOGLE_CLIENT_SECRET":        "test-client-secret",
		"GOOGLE_REDIRECT_URL":         "http://localhost:8080/callback",
		"JWT_SECRET":                  "this-is-a-very-long-secret-key-that-is-at-least-32-characters-long",
		"JWT_EXPIRATION":              "24h",
		"STATE_TIMEOUT":               "5m",
		"POSTGRES_HOST":               "localhost",
		"POSTGRES_PORT":               "5432",
		"POSTGRES_USER":               "testuser",
		"POSTGRES_PASSWORD":           "testpass",
		"POSTGRES_DB":                 "testdb",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4317",
		"OTEL_SERVICE_NAME":           "gosdk",
	}

	for key, value := range envVars {
		t.Setenv(key, value)
	}

	config, err := Load()
	require.NoError(t, err)
	assert.NotNil(t, config)
}

func TestLoad_DefaultValues(t *testing.T) {
	envVars := map[string]string{
		"APP_ENV":                     "production",
		"REDIS_HOST":                  "localhost",
		"REDIS_PORT":                  "6379",
		"GOOGLE_CLIENT_ID":            "test-client-id",
		"GOOGLE_CLIENT_SECRET":        "test-client-secret",
		"GOOGLE_REDIRECT_URL":         "http://localhost:8080/callback",
		"JWT_SECRET":                  "this-is-a-very-long-secret-key-that-is-at-least-32-characters-long",
		"JWT_EXPIRATION":              "24h",
		"STATE_TIMEOUT":               "5m",
		"POSTGRES_HOST":               "localhost",
		"POSTGRES_PORT":               "5432",
		"POSTGRES_USER":               "testuser",
		"POSTGRES_PASSWORD":           "testpass",
		"POSTGRES_DB":                 "testdb",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4317",
		"OTEL_SERVICE_NAME":           "gosdk",
	}

	for key, value := range envVars {
		t.Setenv(key, value)
	}

	config, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "disable", config.Postgres.SSLMode)
	assert.Equal(t, []string{"localhost:9092"}, config.Kafka.Brokers)
	assert.Equal(t, 30*time.Second, config.ShutdownTimeout)
}
