package kafka

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"gosdk/pkg/logger"

	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"
	kafkacompress "github.com/segmentio/kafka-go/compress"
)

const (
	validCompressionNone   = "none"
	validCompressionGzip   = "gzip"
	validCompressionSnappy = "snappy"
	validCompressionLz4    = "lz4"
	validCompressionZstd   = "zstd"

	validAcksAll    = "all"
	validAcksNone   = "none"
	validAcksLeader = "leader"

	defaultMaxMessageSize = 1 * 1024 * 1024  // 1MB default
	kafkaMaxMessageSize   = 10 * 1024 * 1024 // 10MB Kafka limit
)

var (
	compressionCodeMap = map[string]int{
		validCompressionNone:   int(kafkacompress.None),
		validCompressionGzip:   int(kafkacompress.Gzip),
		validCompressionSnappy: int(kafkacompress.Snappy),
		validCompressionLz4:    int(kafkacompress.Lz4),
		validCompressionZstd:   int(kafkacompress.Zstd),
	}

	acksMap = map[string]kafkago.RequiredAcks{
		validAcksAll:    kafkago.RequireAll,
		validAcksNone:   kafkago.RequireNone,
		validAcksLeader: kafkago.RequireOne,
	}
)

// messageCacheEntry represents a cached message ID with timestamp
type messageCacheEntry struct {
	id        string
	timestamp time.Time
}

// KafkaProducer provides high-performance message publishing with optional idempotency guarantees.
// It combines core Kafka publishing functionality with built-in deduplication support.
type KafkaProducer struct {
	// Core Kafka writer
	writer          *kafkago.Writer
	compressionType string
	logger          logger.Logger
	maxMessageSize  int
	metrics         *KafkaMetrics
	schemaRegistry  *SchemaRegistry

	// Idempotency support (optional, enabled via config)
	idempotencyEnabled bool
	idempotencyConfig  IdempotencyConfig
	mu                 sync.RWMutex
	cache              map[string]messageCacheEntry
	windowStart        time.Time
}

// NewKafkaProducer creates a new Kafka producer with optional idempotency support.
// Idempotency is controlled via the idemCfg parameter - use DefaultIdempotencyConfig() for defaults.
func NewKafkaProducer(cfg *ProducerConfig, brokers []string, dialer *kafkago.Dialer, idemCfg IdempotencyConfig, logger logger.Logger) (*KafkaProducer, error) {
	// Validate compression type
	compressionCode, ok := compressionCodeMap[cfg.CompressionType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidCompression, cfg.CompressionType)
	}

	// Validate acks value
	acks, ok := acksMap[cfg.RequiredAcks]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidAcks, cfg.RequiredAcks)
	}

	// Get the compression codec
	var compressionCodec kafkago.CompressionCodec
	if int(compressionCode) < len(kafkacompress.Codecs) {
		compressionCodec = kafkacompress.Codecs[compressionCode]
	}

	// Select balancer based on partition strategy
	balancer := getBalancer(cfg.PartitionStrategy)

	writer := kafkago.NewWriter(kafkago.WriterConfig{
		Brokers:          brokers,
		Balancer:         balancer,
		RequiredAcks:     int(acks),
		BatchSize:        cfg.BatchSize,
		Async:            cfg.Async,
		MaxAttempts:      cfg.MaxAttempts,
		Dialer:           dialer,
		ReadTimeout:      10 * time.Second,
		WriteTimeout:     10 * time.Second,
		CompressionCodec: compressionCodec,
		BatchTimeout:     time.Duration(cfg.LingerMs) * time.Millisecond,
	})

	// Set max message size with validation
	maxSize := cfg.MaxMessageSize
	if maxSize <= 0 {
		maxSize = defaultMaxMessageSize
	}
	if maxSize > kafkaMaxMessageSize {
		maxSize = kafkaMaxMessageSize
	}

	p := &KafkaProducer{
		writer:             writer,
		compressionType:    cfg.CompressionType,
		logger:             logger,
		maxMessageSize:     maxSize,
		metrics:            nil,
		idempotencyEnabled: idemCfg.Enabled,
		idempotencyConfig:  idemCfg,
		cache:              make(map[string]messageCacheEntry),
		windowStart:        time.Now(),
	}

	// Start cache cleanup goroutine if idempotency is enabled
	if idemCfg.Enabled {
		go p.cleanupCache()
	}

	return p, nil
}

// SetMetrics sets the metrics for the producer
func (p *KafkaProducer) SetMetrics(metrics *KafkaMetrics) {
	p.metrics = metrics
}

// SetSchemaRegistry sets the schema registry for the producer
func (p *KafkaProducer) SetSchemaRegistry(schemaRegistry *SchemaRegistry) {
	p.schemaRegistry = schemaRegistry
}

// Publish sends a message to Kafka. If idempotency is enabled, duplicate messages
// within the configured window will be automatically deduplicated.
func (p *KafkaProducer) Publish(ctx context.Context, msg Message) error {
	start := time.Now()

	if p.writer == nil {
		p.logger.Error(ctx, "Producer not initialized",
			logger.Field{Key: "topic", Value: msg.Topic})
		return ErrProducerNotInitialized
	}

	// Validate message size
	totalSize := len(msg.Key) + len(msg.Value)
	for k, v := range msg.Headers {
		totalSize += len(k) + len(v)
	}

	if totalSize > p.maxMessageSize {
		p.logger.Error(ctx, "Message exceeds maximum size",
			logger.Field{Key: "size", Value: totalSize},
			logger.Field{Key: "max_size", Value: p.maxMessageSize},
			logger.Field{Key: "topic", Value: msg.Topic})
		return fmt.Errorf("%w: %d bytes exceeds limit of %d", ErrMessageTooLarge, totalSize, p.maxMessageSize)
	}

	// Handle idempotency if enabled
	if p.idempotencyEnabled {
		// Generate message ID based on content
		msgID := p.generateMessageID(msg)

		// Check for duplicate
		if p.isDuplicate(msgID) {
			p.logger.Debug(ctx, "Duplicate message detected, skipping publish",
				logger.Field{Key: "message_id", Value: msgID},
				logger.Field{Key: "topic", Value: msg.Topic})
			return nil
		}

		// Add idempotency headers
		if msg.Headers == nil {
			msg.Headers = make(map[string]string)
		}
		msg.Headers["x-message-id"] = msgID
		msg.Headers["x-produced-at"] = time.Now().UTC().Format(time.RFC3339Nano)
	}

	// Validate schema encoding if x-schema-id header is present
	if schemaIDStr, ok := msg.Headers["x-schema-id"]; ok {
		if p.schemaRegistry == nil {
			p.logger.Error(ctx, "Schema validation failed: schema registry not set",
				logger.Field{Key: "topic", Value: msg.Topic},
				logger.Field{Key: "schema_id", Value: schemaIDStr})
			return ErrSchemaRegistry
		}

		// Validate the message is properly encoded with Confluent wire format
		if len(msg.Value) < 5 {
			p.logger.Error(ctx, "Schema validation failed: message too short for schema encoding",
				logger.Field{Key: "topic", Value: msg.Topic},
				logger.Field{Key: "schema_id", Value: schemaIDStr})
			return fmt.Errorf("%w: message too short for schema encoding", ErrSchemaEncode)
		}

		// Check magic byte (should be 0)
		if msg.Value[0] != 0 {
			p.logger.Error(ctx, "Schema validation failed: invalid magic byte",
				logger.Field{Key: "topic", Value: msg.Topic},
				logger.Field{Key: "schema_id", Value: schemaIDStr},
				logger.Field{Key: "magic_byte", Value: msg.Value[0]})
			return fmt.Errorf("%w: invalid magic byte %d, expected 0", ErrSchemaEncode, msg.Value[0])
		}
	}

	kafkaMsg := kafkago.Message{
		Topic: msg.Topic,
		Key:   msg.Key,
		Value: msg.Value,
	}

	if msg.Headers != nil {
		for k, v := range msg.Headers {
			kafkaMsg.Headers = append(kafkaMsg.Headers, kafkago.Header{
				Key:   k,
				Value: []byte(v),
			})
		}
	}

	// Record in-flight metrics if available
	if p.metrics != nil {
		p.metrics.MessagesInFlight.Add(ctx, 1)
	}

	err := p.writer.WriteMessages(ctx, kafkaMsg)

	// Decrement in-flight and record result
	if p.metrics != nil {
		p.metrics.MessagesInFlight.Add(ctx, -1)
		p.metrics.RecordPublish(ctx, msg.Topic, time.Since(start), err)
	}

	if err != nil {
		p.logger.Error(ctx, "Failed to publish message",
			logger.Field{Key: "error", Value: err},
			logger.Field{Key: "topic", Value: msg.Topic},
			logger.Field{Key: "compression", Value: p.compressionType})
		return err
	}

	// Cache successful publish if idempotency is enabled
	if p.idempotencyEnabled {
		p.cacheMessage(p.generateMessageID(msg))
	}

	p.logger.Debug(ctx, "Message published successfully",
		logger.Field{Key: "topic", Value: msg.Topic},
		logger.Field{Key: "compression", Value: p.compressionType})
	return nil
}

// PublishWithSchema publishes a message with schema encoding.
// It encodes the data using the specified schema from the registry and publishes it.
func (p *KafkaProducer) PublishWithSchema(ctx context.Context, topic string, key []byte, data interface{}, schemaID int) (Message, error) {
	if p.schemaRegistry == nil {
		p.logger.Error(ctx, "Schema registry not set",
			logger.Field{Key: "topic", Value: topic},
			logger.Field{Key: "schema_id", Value: schemaID})
		return Message{}, ErrSchemaRegistry
	}

	// Encode the data using the schema registry
	encoded, err := p.schemaRegistry.EncodeMessage(ctx, schemaID, data)
	if err != nil {
		p.logger.Error(ctx, "Failed to encode message with schema",
			logger.Field{Key: "topic", Value: topic},
			logger.Field{Key: "schema_id", Value: schemaID},
			logger.Field{Key: "error", Value: err})
		return Message{}, err
	}

	// Create the message with schema ID header
	msg := Message{
		Topic: topic,
		Key:   key,
		Value: encoded,
		Headers: map[string]string{
			"x-schema-id": fmt.Sprintf("%d", schemaID),
		},
	}

	// Publish using the existing Publish method
	if err := p.Publish(ctx, msg); err != nil {
		return msg, err
	}

	return msg, nil
}

// generateMessageID creates a unique identifier for the message
func (p *KafkaProducer) generateMessageID(msg Message) string {
	// Use content-based hashing for idempotency
	h := sha256.New()
	h.Write([]byte(msg.Topic))
	h.Write(msg.Key)
	h.Write(msg.Value)

	// Include headers in hash (sorted for consistency)
	for k, v := range msg.Headers {
		if k != "x-message-id" && k != "x-produced-at" {
			h.Write([]byte(k))
			h.Write([]byte(v))
		}
	}

	hash := hex.EncodeToString(h.Sum(nil))

	if p.idempotencyConfig.KeyPrefix != "" {
		return fmt.Sprintf("%s-%s", p.idempotencyConfig.KeyPrefix, hash[:16])
	}

	// Add UUID for uniqueness within the window
	return fmt.Sprintf("%s-%s", hash[:16], uuid.New().String()[:8])
}

// isDuplicate checks if a message ID has been seen recently
func (p *KafkaProducer) isDuplicate(msgID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	entry, exists := p.cache[msgID]
	if !exists {
		return false
	}

	// Check if within window
	if time.Since(entry.timestamp) > p.idempotencyConfig.WindowSize {
		return false
	}

	return true
}

// cacheMessage adds a message ID to the cache
func (p *KafkaProducer) cacheMessage(msgID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Evict oldest entries if cache is full
	if len(p.cache) >= p.idempotencyConfig.MaxCacheSize {
		p.evictOldest()
	}

	p.cache[msgID] = messageCacheEntry{
		id:        msgID,
		timestamp: time.Now(),
	}
}

// evictOldest removes the oldest entries from the cache
func (p *KafkaProducer) evictOldest() {
	// Simple eviction: clear half the cache
	targetSize := p.idempotencyConfig.MaxCacheSize / 2

	// Find entries outside the window first
	now := time.Now()
	for id, entry := range p.cache {
		if now.Sub(entry.timestamp) > p.idempotencyConfig.WindowSize {
			delete(p.cache, id)
		}
	}

	// If still over target, remove oldest entries
	if len(p.cache) > targetSize {
		// Convert to slice and sort by timestamp
		type kv struct {
			id        string
			timestamp time.Time
		}
		entries := make([]kv, 0, len(p.cache))
		for id, entry := range p.cache {
			entries = append(entries, kv{id: id, timestamp: entry.timestamp})
		}

		// Sort by timestamp (oldest first)
		for i := 0; i < len(entries)-1; i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[j].timestamp.Before(entries[i].timestamp) {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}

		// Remove oldest entries until we reach target
		for i := 0; i < len(entries) && len(p.cache) > targetSize; i++ {
			delete(p.cache, entries[i].id)
		}
	}
}

// cleanupCache periodically removes expired entries
func (p *KafkaProducer) cleanupCache() {
	ticker := time.NewTicker(p.idempotencyConfig.WindowSize / 2)
	defer ticker.Stop()

	for range ticker.C {
		if !p.idempotencyEnabled {
			return
		}

		p.mu.Lock()
		now := time.Now()
		for id, entry := range p.cache {
			if now.Sub(entry.timestamp) > p.idempotencyConfig.WindowSize {
				delete(p.cache, id)
			}
		}
		p.mu.Unlock()
	}
}

// Close closes the producer and cleans up resources
func (p *KafkaProducer) Close() error {
	// Disable idempotency to stop cleanup goroutine
	p.idempotencyEnabled = false
	if p.writer != nil {
		return p.writer.Close()
	}
	return nil
}

// Stats returns current idempotency statistics
func (p *KafkaProducer) Stats() IdempotencyStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return IdempotencyStats{
		Enabled:      p.idempotencyEnabled,
		CacheSize:    len(p.cache),
		MaxCacheSize: p.idempotencyConfig.MaxCacheSize,
		WindowSize:   p.idempotencyConfig.WindowSize,
		WindowStart:  p.windowStart,
	}
}

// SetIdempotencyEnabled enables or disables idempotency at runtime
func (p *KafkaProducer) SetIdempotencyEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// If enabling and wasn't enabled before, start cleanup goroutine
	if enabled && !p.idempotencyEnabled {
		p.idempotencyEnabled = enabled
		go p.cleanupCache()
	} else {
		p.idempotencyEnabled = enabled
	}
}

// getBalancer returns the appropriate balancer based on partition strategy
func getBalancer(strategy PartitionStrategy) kafkago.Balancer {
	switch strategy {
	case PartitionStrategyRoundRobin:
		return &kafkago.RoundRobin{}
	case PartitionStrategyLeastBytes:
		return &kafkago.LeastBytes{}
	case PartitionStrategyHash, "":
		// Default to hash partitioner for key-based ordering
		return &kafkago.Hash{}
	default:
		// Default to hash partitioner
		return &kafkago.Hash{}
	}
}
