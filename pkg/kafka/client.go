package kafka

import (
	"context"
	"fmt"
	"gosdk/cfg"
	"gosdk/pkg/logger"
	"strings"
	"sync"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// KafkaClient provides lazy initialization for Kafka producers and consumers.
// The producer and consumers are only created when first accessed to optimize
// resource usage and startup performance.
type KafkaClient struct {
	config    *Config
	brokers   []string
	dialer    *kafkago.Dialer
	logger    logger.Logger
	producer  *KafkaProducer            // lazily initialized singleton producer
	consumers map[string]*KafkaConsumer // lazily initialized consumers by group ID
	topicMap  map[string][]string       // tracks topics per consumer group
	mu        sync.RWMutex
}

// NewClient creates a new Kafka client without initializing producer or consumers.
// Producer and consumers are lazily initialized on first access to avoid
// unnecessary resource allocation and connection overhead.
func NewClient(cfg *Config, logger logger.Logger) (Client, error) {
	dialer, err := CreateDialer(&cfg.Security)
	if err != nil {
		return nil, fmt.Errorf("failed to create dialer: %w", err)
	}

	return &KafkaClient{
		config:    cfg,
		brokers:   cfg.Brokers,
		dialer:    dialer,
		logger:    logger,
		consumers: make(map[string]*KafkaConsumer),
		topicMap:  make(map[string][]string),
	}, nil
}

// Producer returns a lazily initialized singleton producer instance.
// The producer is created only on first call to optimize startup performance.
func (c *KafkaClient) Producer() (Producer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.producer == nil {
		producer, err := NewKafkaProducer(&c.config.Producer, c.brokers, c.dialer, c.logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create producer: %w", err)
		}
		c.producer = producer
	}

	return c.producer, nil
}

// Consumer returns a lazily initialized consumer for the specified group ID and topics.
// Creates a new consumer instance only if one doesn't already exist for the group ID with the same topics.
func (c *KafkaClient) Consumer(groupID string, topics []string) (Consumer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create a unique key for the consumer based on group ID and topics
	consumerKey := groupID + ":" + strings.Join(topics, ",")

	if _, exists := c.consumers[consumerKey]; !exists {
		consumer := NewKafkaConsumer(&c.config.Consumer, c.brokers, groupID, topics, c.dialer, c.logger)
		consumer.SetRetryConfig(c, c.config.Retry)
		c.consumers[consumerKey] = consumer
		c.topicMap[consumerKey] = topics
	}

	return c.consumers[consumerKey], nil
}

func (c *KafkaClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var errs []error

	if c.producer != nil {
		if closeErr := c.producer.Close(); closeErr != nil {
			errs = append(errs, fmt.Errorf("failed to close producer: %w", closeErr))
		}
	}

	for groupID, consumer := range c.consumers {
		if closeErr := consumer.Close(); closeErr != nil {
			errs = append(errs, fmt.Errorf("failed to close consumer %s: %w", groupID, closeErr))
		}
		delete(c.consumers, groupID)
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

func (c *KafkaClient) Ping(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	conn, err := c.dialer.DialContext(ctx, "tcp", c.brokers[0])
	if err != nil {
		return fmt.Errorf("%w: %v", ErrKafkaConnection, err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			// Log the close error but don't fail the original operation
			_ = closeErr // TODO: Consider logging this
		}
	}()

	return nil
}

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
	MaxMessageSize  int // Maximum message size in bytes (0 = 1MB default)
}

type ConsumerConfig struct {
	MinBytes              int64
	MaxBytes              int64
	CommitInterval        time.Duration
	MaxPollRecords        int
	HeartbeatInterval     time.Duration
	SessionTimeout        time.Duration
	WatchPartitionChanges bool
	StartOffset           StartOffset // Where to start consuming (earliest, latest, none)
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
