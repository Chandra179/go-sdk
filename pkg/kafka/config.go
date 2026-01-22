package kafka

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Brokers  []string
	Producer ProducerConfig
	Consumer ConsumerConfig
	Security SecurityConfig
	Retry    RetryConfig
}

type ProducerConfig struct {
	RequiredAcks    string
	BatchSize       int
	LingerMs        int
	CompressionType string
	MaxAttempts     int
	Async           bool
	CommitInterval  time.Duration
}

type ConsumerConfig struct {
	MinBytes              int64
	MaxBytes              int64
	CommitInterval        time.Duration
	MaxPollRecords        int
	HeartbeatInterval     time.Duration
	SessionTimeout        time.Duration
	WatchPartitionChanges bool
}

type SecurityConfig struct {
	Enabled     bool
	TLSCertFile string
	TLSKeyFile  string
	TLSCAFile   string
}

const (
	defaultProducerBatchSize      = 1048576
	defaultProducerLingerMs       = 10
	defaultProducerMaxAttempts    = 10
	defaultProducerAsync          = false
	defaultProducerCommitInterval = 1 * time.Second

	defaultConsumerMinBytes              = 1024
	defaultConsumerMaxBytes              = 10485760
	defaultConsumerCommitInterval        = 1 * time.Second
	defaultConsumerMaxPollRecords        = 500
	defaultConsumerHeartbeatInterval     = 3 * time.Second
	defaultConsumerSessionTimeout        = 10 * time.Second
	defaultConsumerWatchPartitionChanges = true

	validCompressionNone   = "none"
	validCompressionGzip   = "gzip"
	validCompressionSnappy = "snappy"
	validCompressionLz4    = "lz4"
	validCompressionZstd   = "zstd"

	validAcksAll    = "all"
	validAcksNone   = "none"
	validAcksLeader = "leader"
)

func Load() (*Config, error) {
	var errs []error

	brokers := mustEnv("KAFKA_BROKERS", &errs)
	if brokers == "" {
		errs = append(errs, ErrRequiredBrokers)
	}

	brokerList := parseBrokers(brokers)

	producer := loadProducerConfig(&errs)
	consumer := loadConsumerConfig(&errs)
	security := loadSecurityConfig(&errs)
	retry := loadRetryConfig(&errs)

	if len(errs) > 0 {
		return nil, fmt.Errorf("failed to load Kafka config: %w", joinErrors(errs))
	}

	return &Config{
		Brokers:  brokerList,
		Producer: *producer,
		Consumer: *consumer,
		Security: *security,
		Retry:    *retry,
	}, nil
}

func loadProducerConfig(errs *[]error) *ProducerConfig {
	compression := mustEnv("KAFKA_PRODUCER_COMPRESSION", errs)
	if compression == "" {
		*errs = append(*errs, ErrRequiredCompression)
	} else {
		if err := validateCompression(compression); err != nil {
			*errs = append(*errs, err)
		}
	}

	acks := getEnvWithDefault("KAFKA_PRODUCER_ACKS", validAcksAll)
	if err := validateAcks(acks); err != nil {
		*errs = append(*errs, err)
	}

	batchSize := getEnvIntWithDefault("KAFKA_PRODUCER_BATCH_SIZE", defaultProducerBatchSize)
	lingerMs := getEnvIntWithDefault("KAFKA_PRODUCER_LINGER_MS", defaultProducerLingerMs)
	maxAttempts := getEnvIntWithDefault("KAFKA_PRODUCER_MAX_ATTEMPTS", defaultProducerMaxAttempts)
	async := getEnvBoolWithDefault("KAFKA_PRODUCER_ASYNC", defaultProducerAsync)
	commitIntervalMs := getEnvIntWithDefault("KAFKA_PRODUCER_COMMIT_INTERVAL_MS", int(defaultProducerCommitInterval/time.Millisecond))

	return &ProducerConfig{
		RequiredAcks:    acks,
		BatchSize:       batchSize,
		LingerMs:        lingerMs,
		CompressionType: compression,
		MaxAttempts:     maxAttempts,
		Async:           async,
		CommitInterval:  time.Duration(commitIntervalMs) * time.Millisecond,
	}
}

func loadConsumerConfig(errs *[]error) *ConsumerConfig {
	minBytes := getEnvInt64WithDefault("KAFKA_CONSUMER_MIN_BYTES", defaultConsumerMinBytes)
	maxBytes := getEnvInt64WithDefault("KAFKA_CONSUMER_MAX_BYTES", defaultConsumerMaxBytes)
	commitIntervalMs := getEnvIntWithDefault("KAFKA_CONSUMER_COMMIT_INTERVAL_MS", int(defaultConsumerCommitInterval/time.Millisecond))
	maxPollRecords := getEnvIntWithDefault("KAFKA_CONSUMER_MAX_POLL_RECORDS", defaultConsumerMaxPollRecords)
	heartbeatIntervalMs := getEnvIntWithDefault("KAFKA_CONSUMER_HEARTBEAT_INTERVAL_MS", int(defaultConsumerHeartbeatInterval/time.Millisecond))
	sessionTimeoutMs := getEnvIntWithDefault("KAFKA_CONSUMER_SESSION_TIMEOUT_MS", int(defaultConsumerSessionTimeout/time.Millisecond))
	watchPartitionChanges := getEnvBoolWithDefault("KAFKA_CONSUMER_WATCH_PARTITION_CHANGES", defaultConsumerWatchPartitionChanges)

	return &ConsumerConfig{
		MinBytes:              minBytes,
		MaxBytes:              maxBytes,
		CommitInterval:        time.Duration(commitIntervalMs) * time.Millisecond,
		MaxPollRecords:        maxPollRecords,
		HeartbeatInterval:     time.Duration(heartbeatIntervalMs) * time.Millisecond,
		SessionTimeout:        time.Duration(sessionTimeoutMs) * time.Millisecond,
		WatchPartitionChanges: watchPartitionChanges,
	}
}

func loadSecurityConfig(errs *[]error) *SecurityConfig {
	enabled := getEnvBoolWithDefault("KAFKA_SECURITY_ENABLED", false)

	if enabled {
		certFile := mustEnv("KAFKA_TLS_CERT_FILE", errs)
		keyFile := mustEnv("KAFKA_TLS_KEY_FILE", errs)
		caFile := mustEnv("KAFKA_TLS_CA_FILE", errs)

		if err := validateFileExists(certFile); err != nil {
			*errs = append(*errs, fmt.Errorf("TLS cert file: %w", err))
		}
		if err := validateFileExists(keyFile); err != nil {
			*errs = append(*errs, fmt.Errorf("TLS key file: %w", err))
		}
		if err := validateFileExists(caFile); err != nil {
			*errs = append(*errs, fmt.Errorf("TLS CA file: %w", err))
		}

		return &SecurityConfig{
			Enabled:     enabled,
			TLSCertFile: certFile,
			TLSKeyFile:  keyFile,
			TLSCAFile:   caFile,
		}
	}

	return &SecurityConfig{
		Enabled: false,
	}
}

func loadRetryConfig(errs *[]error) *RetryConfig {
	shortAttempts := getEnvIntWithDefault("KAFKA_RETRY_SHORT_ATTEMPTS", defaultShortRetryAttempts)
	initialBackoffMs := getEnvIntWithDefault("KAFKA_RETRY_INITIAL_BACKOFF_MS", 100)
	maxBackoffMs := getEnvIntWithDefault("KAFKA_RETRY_MAX_BACKOFF_MS", 1000)
	maxLongRetries := getEnvIntWithDefault("KAFKA_RETRY_MAX_LONG_ATTEMPTS", defaultMaxLongRetries)
	dlqEnabled := getEnvBoolWithDefault("KAFKA_RETRY_DLQ_ENABLED", true)
	dlqTopicPrefix := getEnvWithDefault("KAFKA_RETRY_DLQ_TOPIC_PREFIX", ".dlq")
	retryTopicSuffix := getEnvWithDefault("KAFKA_RETRY_TOPIC_SUFFIX", defaultRetryTopicSuffix)

	return &RetryConfig{
		MaxRetries:           int64(shortAttempts),
		InitialBackoff:       int64(initialBackoffMs),
		MaxBackoff:           int64(maxBackoffMs),
		MaxLongRetryAttempts: maxLongRetries,
		DLQEnabled:           dlqEnabled,
		DLQTopicPrefix:       dlqTopicPrefix,
		RetryTopicSuffix:     retryTopicSuffix,
		ShortRetryAttempts:   shortAttempts,
	}
}

func parseBrokers(brokers string) []string {
	return strings.Split(brokers, ",")
}

func validateCompression(compression string) error {
	validTypes := []string{
		validCompressionNone,
		validCompressionGzip,
		validCompressionSnappy,
		validCompressionLz4,
		validCompressionZstd,
	}

	for _, validType := range validTypes {
		if compression == validType {
			return nil
		}
	}

	return fmt.Errorf("%w: got '%s', must be one of: %s", ErrInvalidCompression, compression, strings.Join(validTypes, ", "))
}

func validateAcks(acks string) error {
	validAcks := []string{validAcksAll, validAcksNone, validAcksLeader}

	for _, validAck := range validAcks {
		if acks == validAck {
			return nil
		}
	}

	return fmt.Errorf("%w: got '%s', must be one of: %s", ErrInvalidAcks, acks, strings.Join(validAcks, ", "))
}

func validateFileExists(filepath string) error {
	if _, err := os.Stat(filepath); err != nil {
		return fmt.Errorf("file does not exist: %s", filepath)
	}
	return nil
}

func mustEnv(key string, errs *[]error) string {
	value := os.Getenv(key)
	if value == "" {
		*errs = append(*errs, fmt.Errorf("required environment variable not set: %s", key))
	}
	return value
}

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntWithDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
		log.Printf("invalid integer value for %s: %s, using default: %d", key, value, defaultValue)
	}
	return defaultValue
}

func getEnvInt64WithDefault(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
		log.Printf("invalid int64 value for %s: %s, using default: %d", key, value, defaultValue)
	}
	return defaultValue
}

func getEnvBoolWithDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
		log.Printf("invalid boolean value for %s: %s, using default: %t", key, value, defaultValue)
	}
	return defaultValue
}

func joinErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}

	errStrs := make([]string, len(errs))
	for i, err := range errs {
		errStrs[i] = err.Error()
	}
	return fmt.Errorf("multiple errors: %s", strings.Join(errStrs, "; "))
}
