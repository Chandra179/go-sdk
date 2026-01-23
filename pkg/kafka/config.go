package kafka

import (
	"time"

	"gosdk/cfg"
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

// NewConfig creates a new Kafka Config from the central configuration
func NewConfig(cfg *cfg.KafkaConfig) *Config {
	return &Config{
		Brokers: cfg.Brokers,
		Producer: ProducerConfig{
			RequiredAcks:    cfg.Producer.RequiredAcks,
			BatchSize:       cfg.Producer.BatchSize,
			LingerMs:        cfg.Producer.LingerMs,
			CompressionType: cfg.Producer.CompressionType,
			MaxAttempts:     cfg.Producer.MaxAttempts,
			Async:           cfg.Producer.Async,
			CommitInterval:  cfg.Producer.CommitInterval,
		},
		Consumer: ConsumerConfig{
			MinBytes:              cfg.Consumer.MinBytes,
			MaxBytes:              cfg.Consumer.MaxBytes,
			CommitInterval:        cfg.Consumer.CommitInterval,
			MaxPollRecords:        cfg.Consumer.MaxPollRecords,
			HeartbeatInterval:     cfg.Consumer.HeartbeatInterval,
			SessionTimeout:        cfg.Consumer.SessionTimeout,
			WatchPartitionChanges: cfg.Consumer.WatchPartitionChanges,
		},
		Security: SecurityConfig{
			Enabled:     cfg.Security.Enabled,
			TLSCertFile: cfg.Security.TLSCertFile,
			TLSKeyFile:  cfg.Security.TLSKeyFile,
			TLSCAFile:   cfg.Security.TLSCAFile,
		},
		Retry: RetryConfig{
			MaxRetries:           cfg.Retry.MaxRetries,
			InitialBackoff:       cfg.Retry.InitialBackoff,
			MaxBackoff:           cfg.Retry.MaxBackoff,
			DLQEnabled:           cfg.Retry.DLQEnabled,
			DLQTopicPrefix:       cfg.Retry.DLQTopicPrefix,
			ShortRetryAttempts:   cfg.Retry.ShortRetryAttempts,
			MaxLongRetryAttempts: cfg.Retry.MaxLongRetryAttempts,
			RetryTopicSuffix:     cfg.Retry.RetryTopicSuffix,
		},
	}
}
