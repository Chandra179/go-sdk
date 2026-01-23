package cfg

import (
	"errors"
	"os"
	"strings"
	"time"
)

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

type KafkaConfig struct {
	Brokers  []string
	Producer ProducerConfig
	Consumer ConsumerConfig
	Security SecurityConfig
	Retry    RetryConfig
}

func (l *Loader) loadKafka() *KafkaConfig {
	brokers := l.requireEnv("KAFKA_BROKERS")
	if brokers == "" {
		l.errs = append(l.errs, errors.New("KAFKA_BROKERS is required"))
	}

	brokerList := strings.Split(brokers, ",")

	producer := l.loadProducerConfig()
	consumer := l.loadConsumerConfig()
	security := l.loadSecurityConfig()
	retry := l.loadRetryConfig()

	return &KafkaConfig{
		Brokers:  brokerList,
		Producer: *producer,
		Consumer: *consumer,
		Security: *security,
		Retry:    *retry,
	}
}

func (l *Loader) loadProducerConfig() *ProducerConfig {
	compression := l.getEnvWithDefault("KAFKA_PRODUCER_COMPRESSION", "")
	if compression != "" {
		if err := validateCompression(compression); err != nil {
			l.errs = append(l.errs, err)
		}
	}

	acks := l.getEnvWithDefault("KAFKA_PRODUCER_ACKS", validAcksAll)
	if err := validateAcks(acks); err != nil {
		l.errs = append(l.errs, err)
	}

	batchSize := l.getEnvIntWithDefault("KAFKA_PRODUCER_BATCH_SIZE", defaultProducerBatchSize)
	lingerMs := l.getEnvIntWithDefault("KAFKA_PRODUCER_LINGER_MS", defaultProducerLingerMs)
	maxAttempts := l.getEnvIntWithDefault("KAFKA_PRODUCER_MAX_ATTEMPTS", defaultProducerMaxAttempts)
	async := l.getEnvBoolWithDefault("KAFKA_PRODUCER_ASYNC", defaultProducerAsync)
	commitIntervalMs := l.getEnvIntWithDefault("KAFKA_PRODUCER_COMMIT_INTERVAL_MS", int(defaultProducerCommitInterval/time.Millisecond))

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

func (l *Loader) loadConsumerConfig() *ConsumerConfig {
	minBytes := l.getEnvInt64WithDefault("KAFKA_CONSUMER_MIN_BYTES", defaultConsumerMinBytes)
	maxBytes := l.getEnvInt64WithDefault("KAFKA_CONSUMER_MAX_BYTES", defaultConsumerMaxBytes)
	commitIntervalMs := l.getEnvIntWithDefault("KAFKA_CONSUMER_COMMIT_INTERVAL_MS", int(defaultConsumerCommitInterval/time.Millisecond))
	maxPollRecords := l.getEnvIntWithDefault("KAFKA_CONSUMER_MAX_POLL_RECORDS", defaultConsumerMaxPollRecords)
	heartbeatIntervalMs := l.getEnvIntWithDefault("KAFKA_CONSUMER_HEARTBEAT_INTERVAL_MS", int(defaultConsumerHeartbeatInterval/time.Millisecond))
	sessionTimeoutMs := l.getEnvIntWithDefault("KAFKA_CONSUMER_SESSION_TIMEOUT_MS", int(defaultConsumerSessionTimeout/time.Millisecond))
	watchPartitionChanges := l.getEnvBoolWithDefault("KAFKA_CONSUMER_WATCH_PARTITION_CHANGES", defaultConsumerWatchPartitionChanges)

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

func (l *Loader) loadSecurityConfig() *SecurityConfig {
	enabled := l.getEnvBoolWithDefault("KAFKA_SECURITY_ENABLED", false)

	if enabled {
		certFile := l.requireEnv("KAFKA_TLS_CERT_FILE")
		keyFile := l.requireEnv("KAFKA_TLS_KEY_FILE")
		caFile := l.requireEnv("KAFKA_TLS_CA_FILE")

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

func (l *Loader) loadRetryConfig() *RetryConfig {
	maxRetries := int64(l.getEnvIntWithDefault("KAFKA_RETRY_SHORT_ATTEMPTS", 3))
	initialBackoffMs := int64(l.getEnvIntWithDefault("KAFKA_RETRY_INITIAL_BACKOFF_MS", 100))
	maxBackoffMs := int64(l.getEnvIntWithDefault("KAFKA_RETRY_MAX_BACKOFF_MS", 1000))
	shortRetryAttempts := l.getEnvIntWithDefault("KAFKA_RETRY_SHORT_ATTEMPTS", 3)
	maxLongRetryAttempts := l.getEnvIntWithDefault("KAFKA_RETRY_MAX_LONG_ATTEMPTS", 3)
	dlqEnabled := l.getEnvBoolWithDefault("KAFKA_RETRY_DLQ_ENABLED", true)
	dlqTopicPrefix := l.getEnvWithDefault("KAFKA_RETRY_DLQ_TOPIC_PREFIX", ".dlq")
	retryTopicSuffix := l.getEnvWithDefault("KAFKA_RETRY_TOPIC_SUFFIX", ".retry")

	return &RetryConfig{
		MaxRetries:           maxRetries,
		InitialBackoff:       initialBackoffMs,
		MaxBackoff:           maxBackoffMs,
		DLQEnabled:           dlqEnabled,
		DLQTopicPrefix:       dlqTopicPrefix,
		ShortRetryAttempts:   shortRetryAttempts,
		MaxLongRetryAttempts: maxLongRetryAttempts,
		RetryTopicSuffix:     retryTopicSuffix,
	}
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

	return errors.New("invalid compression type: " + compression + ", must be one of: " + strings.Join(validTypes, ", "))
}

func validateAcks(acks string) error {
	validAcks := []string{validAcksAll, validAcksNone, validAcksLeader}

	for _, validAck := range validAcks {
		if acks == validAck {
			return nil
		}
	}

	return errors.New("invalid acks: " + acks + ", must be one of: " + strings.Join(validAcks, ", "))
}

func validateFileExists(filepath string) error {
	if _, err := os.Stat(filepath); err != nil {
		return errors.New("file does not exist: " + filepath)
	}
	return nil
}

const (
	validCompressionNone   = "none"
	validCompressionGzip   = "gzip"
	validCompressionSnappy = "snappy"
	validCompressionLz4    = "lz4"
	validCompressionZstd   = "zstd"

	validAcksAll    = "all"
	validAcksNone   = "none"
	validAcksLeader = "leader"
)
