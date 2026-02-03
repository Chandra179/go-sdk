package kafka

import (
	"context"
	"fmt"
	"gosdk/cfg"
	"gosdk/pkg/logger"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type KafkaClient struct {
	config         *Config
	brokers        []string
	client         *kgo.Client
	logger         logger.Logger
	producer       Producer
	consumers      map[string]*KafkaConsumer
	topicMap       map[string][]string
	schemaRegistry *SchemaRegistry
	mu             sync.RWMutex
}

func NewClient(cfg *Config, logger logger.Logger) (Client, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.Dialer((&net.Dialer{Timeout: 10 * time.Second}).DialContext),
	}

	if cfg.Security.Enabled {
		tlsConfig, err := CreateTLSConfig(&cfg.Security)
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config: %w", err)
		}
		opts = append(opts, kgo.DialTLSConfig(tlsConfig))
	}

	if cfg.Idempotency.Enabled {
		opts = append(opts, kgo.TransactionalID(cfg.Idempotency.KeyPrefix))
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create franz-go client: %w", err)
	}

	kafkaClient := &KafkaClient{
		config:    cfg,
		brokers:   cfg.Brokers,
		client:    client,
		logger:    logger,
		consumers: make(map[string]*KafkaConsumer),
		topicMap:  make(map[string][]string),
	}

	if cfg.SchemaRegistryConfig.Enabled {
		sr, err := NewSchemaRegistry(cfg.SchemaRegistryConfig, logger)
		if err != nil {
			client.Close()
			return nil, fmt.Errorf("failed to create schema registry: %w", err)
		}
		kafkaClient.schemaRegistry = sr
	}

	return kafkaClient, nil
}

func (c *KafkaClient) Producer() (Producer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.producer == nil {
		idemCfg := c.config.Idempotency
		if idemCfg.WindowSize == 0 {
			idemCfg = DefaultIdempotencyConfig()
		}

		producer, err := NewKafkaProducer(&c.config.Producer, c.client, idemCfg, c.logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create producer: %w", err)
		}
		if c.schemaRegistry != nil {
			producer.SetSchemaRegistry(c.schemaRegistry)
		}
		c.producer = producer
	}

	return c.producer, nil
}

func (c *KafkaClient) Consumer(groupID string, topics []string) (Consumer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	consumerKey := groupID + ":" + strings.Join(topics, ",")

	if _, exists := c.consumers[consumerKey]; !exists {
		consumer, err := NewKafkaConsumer(&c.config.Consumer, c.brokers, groupID, topics, c.logger)
		if err != nil {
			return nil, err
		}
		consumer.SetRetryConfig(c, c.config.Retry)
		if c.schemaRegistry != nil {
			consumer.SetSchemaRegistry(c.schemaRegistry)
		}
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

	if c.schemaRegistry != nil {
		c.schemaRegistry.ClearCache()
	}

	if c.client != nil {
		c.client.Close()
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

func (c *KafkaClient) Ping(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := c.pingAllBrokers(ctx)

	if !result.Healthy {
		return fmt.Errorf("%w: no brokers reachable (%d/%d attempted)",
			ErrKafkaConnection, result.Reachable, result.Total)
	}

	return nil
}

func (c *KafkaClient) PingDetailed(ctx context.Context) *PingResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.pingAllBrokers(ctx)
}

func (c *KafkaClient) pingAllBrokers(ctx context.Context) *PingResult {
	if len(c.brokers) == 0 {
		return &PingResult{
			Healthy: false,
			Total:   0,
		}
	}

	result := &PingResult{
		Brokers: make([]BrokerStatus, 0, len(c.brokers)),
		Total:   len(c.brokers),
	}

	type brokerResult struct {
		status BrokerStatus
		index  int
	}

	resultChan := make(chan brokerResult, len(c.brokers))

	for i, broker := range c.brokers {
		go func(idx int, addr string) {
			start := time.Now()
			status := BrokerStatus{
				Address: addr,
				Healthy: false,
			}

			connCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(connCtx, "tcp", addr)
			if err != nil {
				status.Error = err
			} else {
				status.Healthy = true
				status.Latency = time.Since(start)
				conn.Close()
			}

			resultChan <- brokerResult{status: status, index: idx}
		}(i, broker)
	}

	reachableCount := 0
	for i := 0; i < len(c.brokers); i++ {
		brokerResult := <-resultChan
		result.Brokers = append(result.Brokers, brokerResult.status)

		if brokerResult.status.Healthy {
			reachableCount++
		}
	}

	result.Reachable = reachableCount
	result.Healthy = reachableCount > 0
	result.AllHealthy = reachableCount == len(c.brokers)

	return result
}

func (c *KafkaClient) UnderlyingClient() *kgo.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

func (c *KafkaClient) SchemaRegistry() *SchemaRegistry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.schemaRegistry
}

type Config struct {
	Brokers              []string
	Producer             ProducerConfig
	Consumer             ConsumerConfig
	Security             SecurityConfig
	Retry                RetryConfig
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
