package cfg

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

type Config struct {
	AppEnv string
	Redis  RedisConfig
}

func Load() (*Config, error) {
	_ = godotenv.Load() // ignore if .env missing (local only)

	var errs []error

	appEnv := mustEnv("APP_ENV", &errs)
	host := mustEnv("REDIS_HOST", &errs)
	port := mustEnv("REDIS_PORT", &errs)
	password := mustEnv("REDIS_PASSWORD", &errs)

	// If any error collected, group them into one
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return &Config{
		AppEnv: appEnv,
		Redis: RedisConfig{
			Host:     host,
			Port:     port,
			Password: password,
			DB:       0,
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
