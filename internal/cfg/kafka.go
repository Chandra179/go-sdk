package cfg

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// Kafka YAML config file path
const kafkaYAMLPath = "internal/cfg/kafka.yaml"

// Environment variable names for Kafka secrets
const (
	// Brokers (allows override of YAML config for Docker environments)
	envKAFKABrokers = "KAFKA_BROKERS"

	// Security/TLS
	envKAFKATLSCertFile = "KAFKA_TLS_CERT_FILE"
	envKAFKATLSKeyFile  = "KAFKA_TLS_KEY_FILE"
	envKAFKATLSCAFile   = "KAFKA_TLS_CA_FILE"

	// Schema Registry
	envKAFKASchemaRegistryUsername = "KAFKA_SCHEMA_REGISTRY_USERNAME"
	envKAFKASchemaRegistryPassword = "KAFKA_SCHEMA_REGISTRY_PASSWORD"

	// Transaction
	envKAFKATransactionID = "KAFKA_TRANSACTION_ID"
)

type RetryConfig struct {
	MaxRetries           int64
	InitialBackoff       int64
	MaxBackoff           int64
	DLQEnabled           bool
	DLQTopicPrefix       string
	ShortRetryAttempts   int
	MaxLongRetryAttempts int
	RetryTopicSuffix     string
}

type IdempotencyConfig struct {
	Enabled           bool
	WindowSizeSeconds int
	MaxCacheSize      int
}

type ProducerConfig struct {
	RequiredAcks      string
	BatchSize         int
	LingerMs          int
	CompressionType   string
	MaxAttempts       int
	Async             bool
	CommitInterval    time.Duration
	MaxMessageSize    int
	PartitionStrategy string
}

type ConsumerConfig struct {
	MinBytes              int64
	MaxBytes              int64
	CommitInterval        time.Duration
	MaxPollRecords        int
	HeartbeatInterval     time.Duration
	SessionTimeout        time.Duration
	WatchPartitionChanges bool
	StartOffset           string
}

type PoolConfig struct {
	MaxConnections     int
	IdleTimeoutSeconds int
}

type SecurityConfig struct {
	Enabled     bool
	TLSCertFile string
	TLSKeyFile  string
	TLSCAFile   string
}

type SchemaRegistryConfig struct {
	Enabled         bool
	URL             string
	Username        string
	Password        string
	Format          string
	CacheTTLSeconds int
}

type TransactionConfig struct {
	Enabled        bool
	TransactionID  string
	TimeoutSeconds int
	MaxRetries     int
	RetryBackoffMs int
}

type KafkaConfig struct {
	Brokers        []string
	Producer       ProducerConfig
	Consumer       ConsumerConfig
	Security       SecurityConfig
	Retry          RetryConfig
	Pool           PoolConfig
	Idempotency    IdempotencyConfig
	SchemaRegistry SchemaRegistryConfig
	Transaction    TransactionConfig
}

// YAML config structures - mirrors the yaml file structure
type KafkaYAMLConfig struct {
	Brokers        []string                 `yaml:"brokers"`
	Producer       ProducerYAMLConfig       `yaml:"producer"`
	Consumer       ConsumerYAMLConfig       `yaml:"consumer"`
	Retry          RetryYAMLConfig          `yaml:"retry"`
	Pool           PoolYAMLConfig           `yaml:"pool"`
	Idempotency    IdempotencyYAMLConfig    `yaml:"idempotency"`
	Security       SecurityYAMLConfig       `yaml:"security"`
	SchemaRegistry SchemaRegistryYAMLConfig `yaml:"schema_registry"`
	Transaction    TransactionYAMLConfig    `yaml:"transaction"`
}

type ProducerYAMLConfig struct {
	RequiredAcks      string `yaml:"required_acks"`
	BatchSize         int    `yaml:"batch_size"`
	LingerMs          int    `yaml:"linger_ms"`
	CompressionType   string `yaml:"compression"`
	MaxAttempts       int    `yaml:"max_attempts"`
	Async             bool   `yaml:"async"`
	CommitInterval    int    `yaml:"commit_interval_ms"`
	MaxMessageSize    int    `yaml:"max_message_size"`
	PartitionStrategy string `yaml:"partition_strategy"`
}

type ConsumerYAMLConfig struct {
	MinBytes              int64  `yaml:"min_bytes"`
	MaxBytes              int64  `yaml:"max_bytes"`
	CommitInterval        int    `yaml:"commit_interval_ms"`
	MaxPollRecords        int    `yaml:"max_poll_records"`
	HeartbeatInterval     int    `yaml:"heartbeat_interval_ms"`
	SessionTimeout        int    `yaml:"session_timeout_ms"`
	WatchPartitionChanges bool   `yaml:"watch_partition_changes"`
	StartOffset           string `yaml:"start_offset"`
}

type RetryYAMLConfig struct {
	MaxRetries           int64  `yaml:"max_retries"`
	InitialBackoff       int64  `yaml:"initial_backoff_ms"`
	MaxBackoff           int64  `yaml:"max_backoff_ms"`
	DLQEnabled           bool   `yaml:"dlq_enabled"`
	DLQTopicPrefix       string `yaml:"dlq_topic_prefix"`
	ShortRetryAttempts   int    `yaml:"short_retry_attempts"`
	MaxLongRetryAttempts int    `yaml:"max_long_retry_attempts"`
	RetryTopicSuffix     string `yaml:"retry_topic_suffix"`
}

type PoolYAMLConfig struct {
	MaxConnections     int `yaml:"max_connections"`
	IdleTimeoutSeconds int `yaml:"idle_timeout_seconds"`
}

type IdempotencyYAMLConfig struct {
	Enabled           bool `yaml:"enabled"`
	WindowSizeSeconds int  `yaml:"window_size_seconds"`
	MaxCacheSize      int  `yaml:"max_cache_size"`
}

type SecurityYAMLConfig struct {
	Enabled bool `yaml:"enabled"`
}

type SchemaRegistryYAMLConfig struct {
	Enabled         bool   `yaml:"enabled"`
	URL             string `yaml:"url"`
	Format          string `yaml:"format"`
	CacheTTLSeconds int    `yaml:"cache_ttl_seconds"`
}

type TransactionYAMLConfig struct {
	Enabled        bool `yaml:"enabled"`
	TimeoutSeconds int  `yaml:"timeout_seconds"`
	MaxRetries     int  `yaml:"max_retries"`
	RetryBackoffMs int  `yaml:"retry_backoff_ms"`
}

func (l *Loader) loadKafka() *KafkaConfig {
	// Load YAML config from internal/cfg/kafka.yaml
	yamlCfg, err := l.loadKafkaYAML()
	if err != nil {
		l.errs = append(l.errs, errors.New("failed to load kafka yaml config: "+err.Error()))
		return nil
	}

	// Brokers can be overridden via KAFKA_BROKERS env var (comma-separated)
	brokers := l.loadBrokers(yamlCfg.Brokers)

	// Validate required fields
	if len(brokers) == 0 {
		l.errs = append(l.errs, errors.New("kafka brokers is required"))
	}

	// Load security config - env for secrets (TLS files), YAML for enabled flag
	security := l.loadSecurityConfig(yamlCfg.Security)

	// Load schema registry config - env for secrets (username/password), YAML for other settings
	schemaRegistry := l.loadSchemaRegistryConfig(yamlCfg.SchemaRegistry)

	return &KafkaConfig{
		Brokers:        brokers,
		Producer:       l.toProducerConfig(yamlCfg.Producer),
		Consumer:       l.toConsumerConfig(yamlCfg.Consumer),
		Security:       *security,
		Retry:          l.toRetryConfig(yamlCfg.Retry),
		Pool:           l.toPoolConfig(yamlCfg.Pool),
		Idempotency:    l.toIdempotencyConfig(yamlCfg.Idempotency),
		Transaction:    l.toTransactionConfig(yamlCfg.Transaction),
		SchemaRegistry: *schemaRegistry,
	}
}

// loadBrokers loads broker list from env var KAFKA_BROKERS (comma-separated) or falls back to YAML config
func (l *Loader) loadBrokers(yamlBrokers []string) []string {
	if envBrokers := os.Getenv(envKAFKABrokers); envBrokers != "" {
		// Parse comma-separated brokers
		var brokers []string
		for _, broker := range splitAndTrim(envBrokers, ",") {
			if broker != "" {
				brokers = append(brokers, broker)
			}
		}
		if len(brokers) > 0 {
			return brokers
		}
	}
	return yamlBrokers
}

// splitAndTrim splits a string by separator and trims whitespace from each part
func splitAndTrim(s, sep string) []string {
	parts := make([]string, 0)
	for _, part := range strings.Split(s, sep) {
		parts = append(parts, strings.TrimSpace(part))
	}
	return parts
}

// loadKafkaYAML loads Kafka configuration from internal/cfg/kafka.yaml
func (l *Loader) loadKafkaYAML() (*KafkaYAMLConfig, error) {
	yamlData, err := os.ReadFile(kafkaYAMLPath)
	if err != nil {
		return nil, errors.New("failed to read " + kafkaYAMLPath + ": " + err.Error())
	}

	var cfg KafkaYAMLConfig
	if err := yaml.Unmarshal(yamlData, &cfg); err != nil {
		return nil, errors.New("failed to parse internal/cfg/kafka.yaml: " + err.Error())
	}

	return &cfg, nil
}

func (l *Loader) toProducerConfig(yamlCfg ProducerYAMLConfig) ProducerConfig {
	return ProducerConfig{
		RequiredAcks:      yamlCfg.RequiredAcks,
		BatchSize:         yamlCfg.BatchSize,
		LingerMs:          yamlCfg.LingerMs,
		CompressionType:   yamlCfg.CompressionType,
		MaxAttempts:       yamlCfg.MaxAttempts,
		Async:             yamlCfg.Async,
		CommitInterval:    time.Duration(yamlCfg.CommitInterval) * time.Millisecond,
		MaxMessageSize:    yamlCfg.MaxMessageSize,
		PartitionStrategy: yamlCfg.PartitionStrategy,
	}
}

func (l *Loader) toConsumerConfig(yamlCfg ConsumerYAMLConfig) ConsumerConfig {
	return ConsumerConfig{
		MinBytes:              yamlCfg.MinBytes,
		MaxBytes:              yamlCfg.MaxBytes,
		CommitInterval:        time.Duration(yamlCfg.CommitInterval) * time.Millisecond,
		MaxPollRecords:        yamlCfg.MaxPollRecords,
		HeartbeatInterval:     time.Duration(yamlCfg.HeartbeatInterval) * time.Millisecond,
		SessionTimeout:        time.Duration(yamlCfg.SessionTimeout) * time.Millisecond,
		WatchPartitionChanges: yamlCfg.WatchPartitionChanges,
		StartOffset:           yamlCfg.StartOffset,
	}
}

func (l *Loader) loadSecurityConfig(yamlCfg SecurityYAMLConfig) *SecurityConfig {
	// Secrets (TLS files) always come from env
	if yamlCfg.Enabled {
		certFile := l.requireEnv(envKAFKATLSCertFile)
		keyFile := l.requireEnv(envKAFKATLSKeyFile)
		caFile := l.requireEnv(envKAFKATLSCAFile)

		if err := validateFileExists(certFile); err != nil {
			l.errs = append(l.errs, errors.New("TLS cert file: "+err.Error()))
		}
		if err := validateFileExists(keyFile); err != nil {
			l.errs = append(l.errs, errors.New("TLS key file: "+err.Error()))
		}
		if err := validateFileExists(caFile); err != nil {
			l.errs = append(l.errs, errors.New("TLS CA file: "+err.Error()))
		}

		return &SecurityConfig{
			Enabled:     yamlCfg.Enabled,
			TLSCertFile: certFile,
			TLSKeyFile:  keyFile,
			TLSCAFile:   caFile,
		}
	}

	return &SecurityConfig{
		Enabled: false,
	}
}

func (l *Loader) toRetryConfig(yamlCfg RetryYAMLConfig) RetryConfig {
	return RetryConfig{
		MaxRetries:           yamlCfg.MaxRetries,
		InitialBackoff:       yamlCfg.InitialBackoff,
		MaxBackoff:           yamlCfg.MaxBackoff,
		DLQEnabled:           yamlCfg.DLQEnabled,
		DLQTopicPrefix:       yamlCfg.DLQTopicPrefix,
		ShortRetryAttempts:   yamlCfg.ShortRetryAttempts,
		MaxLongRetryAttempts: yamlCfg.MaxLongRetryAttempts,
		RetryTopicSuffix:     yamlCfg.RetryTopicSuffix,
	}
}

func (l *Loader) toPoolConfig(yamlCfg PoolYAMLConfig) PoolConfig {
	return PoolConfig{
		MaxConnections:     yamlCfg.MaxConnections,
		IdleTimeoutSeconds: yamlCfg.IdleTimeoutSeconds,
	}
}

func (l *Loader) toIdempotencyConfig(yamlCfg IdempotencyYAMLConfig) IdempotencyConfig {
	return IdempotencyConfig{
		Enabled:           yamlCfg.Enabled,
		WindowSizeSeconds: yamlCfg.WindowSizeSeconds,
		MaxCacheSize:      yamlCfg.MaxCacheSize,
	}
}

func (l *Loader) toTransactionConfig(yamlCfg TransactionYAMLConfig) TransactionConfig {
	// TransactionID always comes from env
	transactionID := l.getEnvWithDefault(envKAFKATransactionID, "")

	return TransactionConfig{
		Enabled:        yamlCfg.Enabled,
		TransactionID:  transactionID,
		TimeoutSeconds: yamlCfg.TimeoutSeconds,
		MaxRetries:     yamlCfg.MaxRetries,
		RetryBackoffMs: yamlCfg.RetryBackoffMs,
	}
}

func (l *Loader) loadSchemaRegistryConfig(yamlCfg SchemaRegistryYAMLConfig) *SchemaRegistryConfig {
	// Secrets (username/password) always come from env
	username := l.getEnvWithDefault(envKAFKASchemaRegistryUsername, "")
	password := l.getEnvWithDefault(envKAFKASchemaRegistryPassword, "")

	if yamlCfg.Enabled && yamlCfg.URL == "" {
		l.errs = append(l.errs, errors.New("kafka schema_registry.url is required when schema_registry is enabled"))
	}

	if yamlCfg.Format != "" {
		validFormats := []string{"avro", "json", "protobuf"}
		isValid := false
		for _, valid := range validFormats {
			if yamlCfg.Format == valid {
				isValid = true
				break
			}
		}
		if !isValid {
			l.errs = append(l.errs, errors.New("invalid schema_registry format: "+yamlCfg.Format+", must be one of: avro, json, protobuf"))
		}
	}

	return &SchemaRegistryConfig{
		Enabled:         yamlCfg.Enabled,
		URL:             yamlCfg.URL,
		Username:        username,
		Password:        password,
		Format:          yamlCfg.Format,
		CacheTTLSeconds: yamlCfg.CacheTTLSeconds,
	}
}

func validateFileExists(filepath string) error {
	if _, err := os.Stat(filepath); err != nil {
		return errors.New("file does not exist: " + filepath)
	}
	return nil
}
