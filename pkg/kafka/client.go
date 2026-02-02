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
	config         *Config
	brokers        []string
	dialer         *kafkago.Dialer
	logger         logger.Logger
	producer       Producer                  // lazily initialized singleton producer (interface)
	consumers      map[string]*KafkaConsumer // lazily initialized consumers by group ID
	topicMap       map[string][]string       // tracks topics per consumer group
	connPool       *ConnectionPool           // connection pool for better resource utilization
	connManager    *ConnectionManager        // manages multiple connection pools
	schemaRegistry *SchemaRegistry           // schema registry client for data governance
	mu             sync.RWMutex
}

// NewClient creates a new Kafka client without initializing producer or consumers.
// Producer and consumers are lazily initialized on first access to avoid
// unnecessary resource allocation and connection overhead.
func NewClient(cfg *Config, logger logger.Logger) (Client, error) {
	dialer, err := CreateDialer(&cfg.Security)
	if err != nil {
		return nil, fmt.Errorf("failed to create dialer: %w", err)
	}

	// Initialize connection pool with configuration
	poolCfg := cfg.Pool
	if poolCfg.MaxConnections == 0 {
		poolCfg = DefaultConnectionPoolConfig()
	}
	connPool := NewConnectionPool(dialer, poolCfg)
	connManager := NewConnectionManager(dialer, poolCfg)

	client := &KafkaClient{
		config:      cfg,
		brokers:     cfg.Brokers,
		dialer:      dialer,
		logger:      logger,
		consumers:   make(map[string]*KafkaConsumer),
		topicMap:    make(map[string][]string),
		connPool:    connPool,
		connManager: connManager,
	}

	// Initialize schema registry if enabled
	if cfg.SchemaRegistryConfig.Enabled {
		sr, err := NewSchemaRegistry(cfg.SchemaRegistryConfig, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create schema registry: %w", err)
		}
		client.schemaRegistry = sr
	}

	return client, nil
}

// Producer returns a lazily initialized singleton producer instance with optional idempotency.
// The producer is created only on first call to optimize startup performance.
// Idempotency is configured via the Idempotency field in Config.
func (c *KafkaClient) Producer() (Producer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.producer == nil {
		// Use configured idempotency settings or defaults
		idemCfg := c.config.Idempotency
		if idemCfg.WindowSize == 0 {
			idemCfg = DefaultIdempotencyConfig()
		}

		producer, err := NewKafkaProducer(&c.config.Producer, c.brokers, c.dialer, idemCfg, c.logger)
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

	// Close connection pools
	if c.connPool != nil {
		if closeErr := c.connPool.Close(); closeErr != nil {
			errs = append(errs, fmt.Errorf("failed to close connection pool: %w", closeErr))
		}
	}

	if c.connManager != nil {
		if closeErr := c.connManager.Close(); closeErr != nil {
			errs = append(errs, fmt.Errorf("failed to close connection manager: %w", closeErr))
		}
	}

	// Close schema registry if present
	if c.schemaRegistry != nil {
		c.schemaRegistry.ClearCache()
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
			_ = closeErr
		}
	}()

	return nil
}

// ConnectionPool returns the connection pool for direct use
func (c *KafkaClient) ConnectionPool() *ConnectionPool {
	return c.connPool
}

// ConnectionManager returns the connection manager for direct use
func (c *KafkaClient) ConnectionManager() *ConnectionManager {
	return c.connManager
}

// SchemaRegistry returns the schema registry client for data governance
func (c *KafkaClient) SchemaRegistry() *SchemaRegistry {
	return c.schemaRegistry
}

type Config struct {
	Brokers              []string
	Producer             ProducerConfig
	Consumer             ConsumerConfig
	Security             SecurityConfig
	Retry                RetryConfig
	Pool                 ConnectionPoolConfig
	Idempotency          IdempotencyConfig
	SchemaRegistryConfig SchemaRegistryConfig
	Transaction          TransactionConfig
}

type ProducerConfig struct {
	RequiredAcks      string
	BatchSize         int
	LingerMs          int
	CompressionType   string
	MaxAttempts       int
	Async             bool
	MaxMessageSize    int
	PartitionStrategy PartitionStrategy
}

type ConsumerConfig struct {
	MinBytes              int64
	MaxBytes              int64
	CommitInterval        time.Duration
	MaxPollRecords        int
	HeartbeatInterval     time.Duration
	SessionTimeout        time.Duration
	WatchPartitionChanges bool
	StartOffset           StartOffset
}

type SecurityConfig struct {
	Enabled     bool
	TLSCertFile string
	TLSKeyFile  string
	TLSCAFile   string
}

func NewConfig(cfg *cfg.KafkaConfig) *Config {
	return &Config{
		Brokers: cfg.Brokers,
		Idempotency: IdempotencyConfig{
			Enabled:      cfg.Idempotency.Enabled,
			WindowSize:   time.Duration(cfg.Idempotency.WindowSizeSeconds) * time.Second,
			MaxCacheSize: cfg.Idempotency.MaxCacheSize,
			KeyPrefix:    "",
		},
		Producer: ProducerConfig{
			RequiredAcks:      cfg.Producer.RequiredAcks,
			BatchSize:         cfg.Producer.BatchSize,
			LingerMs:          cfg.Producer.LingerMs,
			CompressionType:   cfg.Producer.CompressionType,
			MaxAttempts:       cfg.Producer.MaxAttempts,
			Async:             cfg.Producer.Async,
			MaxMessageSize:    cfg.Producer.MaxMessageSize,
			PartitionStrategy: PartitionStrategy(cfg.Producer.PartitionStrategy),
		},
		Consumer: ConsumerConfig{
			MinBytes:              cfg.Consumer.MinBytes,
			MaxBytes:              cfg.Consumer.MaxBytes,
			CommitInterval:        cfg.Consumer.CommitInterval,
			MaxPollRecords:        cfg.Consumer.MaxPollRecords,
			HeartbeatInterval:     cfg.Consumer.HeartbeatInterval,
			SessionTimeout:        cfg.Consumer.SessionTimeout,
			WatchPartitionChanges: cfg.Consumer.WatchPartitionChanges,
			StartOffset: func(s string) StartOffset {
				switch s {
				case "earliest":
					return StartOffsetEarliest
				case "latest":
					return StartOffsetLatest
				case "none":
					return StartOffsetNone
				default:
					return StartOffsetLatest
				}
			}(cfg.Consumer.StartOffset),
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
		Pool: ConnectionPoolConfig{
			MaxConnections: cfg.Pool.MaxConnections,
			IdleTimeout:    time.Duration(cfg.Pool.IdleTimeoutSeconds) * time.Second,
		},
		SchemaRegistryConfig: SchemaRegistryConfig{
			Enabled:  cfg.SchemaRegistry.Enabled,
			URL:      cfg.SchemaRegistry.URL,
			Username: cfg.SchemaRegistry.Username,
			Password: cfg.SchemaRegistry.Password,
			Format:   SchemaFormat(cfg.SchemaRegistry.Format),
			CacheTTL: time.Duration(cfg.SchemaRegistry.CacheTTLSeconds) * time.Second,
		},
		Transaction: TransactionConfig{
			Enabled:       cfg.Transaction.Enabled,
			TransactionID: cfg.Transaction.TransactionID,
			Timeout:       time.Duration(cfg.Transaction.TimeoutSeconds) * time.Second,
			MaxRetries:    cfg.Transaction.MaxRetries,
			RetryBackoff:  time.Duration(cfg.Transaction.RetryBackoffMs) * time.Millisecond,
		},
	}
}
