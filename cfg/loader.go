package cfg

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Loader struct {
	errs []error
}

func NewLoader() *Loader {
	return &Loader{errs: make([]error, 0)}
}

func (l *Loader) HasErrors() bool {
	return len(l.errs) > 0
}

func (l *Loader) Error() error {
	if len(l.errs) > 0 {
		return errors.Join(l.errs...)
	}
	return nil
}

func (l *Loader) requireEnv(key string) string {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		l.errs = append(l.errs, errors.New("missing env: "+key))
	}
	return value
}

func (l *Loader) requireDuration(key string) time.Duration {
	value := l.requireEnv(key)
	if value == "" {
		return 0
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		l.errs = append(l.errs, errors.New("invalid duration for "+key+": "+value))
	}
	return duration
}

func (l *Loader) requireInt(key string) int {
	value := l.requireEnv(key)
	if value == "" {
		return 0
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		l.errs = append(l.errs, errors.New("invalid int for "+key+": "+value))
	}
	return intValue
}

func (l *Loader) getEnvIntOrDefault(key string, defaultValue int) int {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		return defaultValue
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		l.errs = append(l.errs, errors.New("invalid int for "+key+": "+value))
		return defaultValue
	}
	return intValue
}

func (l *Loader) getEnvDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		return defaultValue
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		l.errs = append(l.errs, errors.New("invalid duration for "+key+": "+value))
		return defaultValue
	}
	return duration
}

func (l *Loader) requireInt64(key string) int64 {
	value := l.requireEnv(key)
	if value == "" {
		return 0
	}
	intValue, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		l.errs = append(l.errs, errors.New("invalid int64 for "+key+": "+value))
	}
	return intValue
}

func (l *Loader) requireFloat64(key string) float64 {
	value := l.requireEnv(key)
	if value == "" {
		return 0
	}
	floatValue, err := strconv.ParseFloat(value, 64)
	if err != nil {
		l.errs = append(l.errs, errors.New("invalid float for "+key+": "+value))
	}
	return floatValue
}

func (l *Loader) getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func (l *Loader) getEnvIntWithDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func (l *Loader) getEnvInt64WithDefault(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func (l *Loader) getEnvBoolWithDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func (l *Loader) getEnvDurationWithDefault(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
