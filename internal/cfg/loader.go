package cfg

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// VaultSecretsPath is the path where Vault Agent writes secret files
const VaultSecretsPath = "/vault/secrets"

type Loader struct {
	errs []error
}

func NewLoader() *Loader {
	// Load Vault secrets before returning the loader
	loadVaultSecrets()
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

// loadVaultSecrets loads environment variables from Vault Agent output files
func loadVaultSecrets() {
	// Check if Vault secrets directory exists
	if _, err := os.Stat(VaultSecretsPath); os.IsNotExist(err) {
		// Vault secrets not available, rely on existing env vars
		return
	}

	// Read all .env files in the Vault secrets directory
	entries, err := os.ReadDir(VaultSecretsPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".env") {
			continue
		}

		filePath := VaultSecretsPath + "/" + entry.Name()
		file, err := os.Open(filePath)
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			// Only set if not already set (env vars take precedence)
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}
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
