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
	"github.com/twmb/franz-go/pkg/kgo"
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

	defaultMaxMessageSize = 1 * 1024 * 1024
	kafkaMaxMessageSize   = 10 * 1024 * 1024
)

var (
	compressionCodecs = map[string]func() kgo.CompressionCodec{
		validCompressionNone:   kgo.NoCompression,
		validCompressionGzip:   kgo.GzipCompression,
		validCompressionSnappy: kgo.SnappyCompression,
		validCompressionLz4:    kgo.Lz4Compression,
		validCompressionZstd:   kgo.ZstdCompression,
	}

	acksValues = map[string]kgo.Acks{
		validAcksAll:    kgo.AllISRAcks(),
		validAcksNone:   kgo.NoAck(),
		validAcksLeader: kgo.LeaderAck(),
	}
)

type messageCacheEntry struct {
	id        string
	timestamp time.Time
}

type KafkaProducer struct {
	client          *kgo.Client
	compressionType string
	logger          logger.Logger
	maxMessageSize  int
	metrics         *KafkaMetrics
	schemaRegistry  *SchemaRegistry

	idempotencyEnabled bool
	idempotencyConfig  IdempotencyConfig
	mu                 sync.RWMutex
	cache              map[string]messageCacheEntry
	windowStart        time.Time
}

func NewKafkaProducer(cfg *ProducerConfig, client *kgo.Client, idemCfg IdempotencyConfig, logger logger.Logger) (*KafkaProducer, error) {
	maxSize := cfg.MaxMessageSize
	if maxSize <= 0 {
		maxSize = defaultMaxMessageSize
	}
	if maxSize > kafkaMaxMessageSize {
		maxSize = kafkaMaxMessageSize
	}

	p := &KafkaProducer{
		client:             client,
		compressionType:    cfg.CompressionType,
		logger:             logger,
		maxMessageSize:     maxSize,
		metrics:            nil,
		idempotencyEnabled: idemCfg.Enabled,
		idempotencyConfig:  idemCfg,
		cache:              make(map[string]messageCacheEntry),
		windowStart:        time.Now(),
	}

	if idemCfg.Enabled {
		go p.cleanupCache()
	}

	return p, nil
}

func (p *KafkaProducer) SetMetrics(metrics *KafkaMetrics) {
	p.metrics = metrics
}

func (p *KafkaProducer) SetSchemaRegistry(schemaRegistry *SchemaRegistry) {
	p.schemaRegistry = schemaRegistry
}

func (p *KafkaProducer) Publish(ctx context.Context, msg Message) error {
	start := time.Now()

	if p.client == nil {
		p.logger.Error(ctx, "Producer not initialized",
			logger.Field{Key: "topic", Value: msg.Topic})
		return ErrProducerNotInitialized
	}

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

	if p.idempotencyEnabled {
		msgID := p.generateMessageID(msg)

		if p.isDuplicate(msgID) {
			p.logger.Debug(ctx, "Duplicate message detected, skipping publish",
				logger.Field{Key: "message_id", Value: msgID},
				logger.Field{Key: "topic", Value: msg.Topic})
			return nil
		}

		if msg.Headers == nil {
			msg.Headers = make(map[string]string)
		}
		msg.Headers["x-message-id"] = msgID
		msg.Headers["x-produced-at"] = time.Now().UTC().Format(time.RFC3339Nano)
	}

	if schemaIDStr, ok := msg.Headers["x-schema-id"]; ok {
		if p.schemaRegistry == nil {
			p.logger.Error(ctx, "Schema validation failed: schema registry not set",
				logger.Field{Key: "topic", Value: msg.Topic},
				logger.Field{Key: "schema_id", Value: schemaIDStr})
			return ErrSchemaRegistry
		}

		if len(msg.Value) < 5 {
			p.logger.Error(ctx, "Schema validation failed: message too short for schema encoding",
				logger.Field{Key: "topic", Value: msg.Topic},
				logger.Field{Key: "schema_id", Value: schemaIDStr})
			return fmt.Errorf("%w: message too short for schema encoding", ErrSchemaEncode)
		}

		if msg.Value[0] != 0 {
			p.logger.Error(ctx, "Schema validation failed: invalid magic byte",
				logger.Field{Key: "topic", Value: msg.Topic},
				logger.Field{Key: "schema_id", Value: schemaIDStr},
				logger.Field{Key: "magic_byte", Value: msg.Value[0]})
			return fmt.Errorf("%w: invalid magic byte %d, expected 0", ErrSchemaEncode, msg.Value[0])
		}
	}

	record := msg.toRecord()

	if p.metrics != nil {
		p.metrics.MessagesInFlight.Add(ctx, 1)
	}

	result := p.client.ProduceSync(ctx, record)
	if result.FirstErr() != nil {
		if p.metrics != nil {
			p.metrics.MessagesInFlight.Add(ctx, -1)
			p.metrics.RecordPublish(ctx, msg.Topic, time.Since(start), result.FirstErr())
		}
		p.logger.Error(ctx, "Failed to publish message",
			logger.Field{Key: "error", Value: result.FirstErr()},
			logger.Field{Key: "topic", Value: msg.Topic},
			logger.Field{Key: "compression", Value: p.compressionType})
		return result.FirstErr()
	}

	if p.metrics != nil {
		p.metrics.MessagesInFlight.Add(ctx, -1)
		p.metrics.RecordPublish(ctx, msg.Topic, time.Since(start), nil)
	}

	if p.idempotencyEnabled {
		p.cacheMessage(p.generateMessageID(msg))
	}

	p.logger.Debug(ctx, "Message published successfully",
		logger.Field{Key: "topic", Value: msg.Topic},
		logger.Field{Key: "compression", Value: p.compressionType})
	return nil
}

func (p *KafkaProducer) PublishWithSchema(ctx context.Context, topic string, key []byte, data interface{}, schemaID int) (Message, error) {
	if p.schemaRegistry == nil {
		p.logger.Error(ctx, "Schema registry not set",
			logger.Field{Key: "topic", Value: topic},
			logger.Field{Key: "schema_id", Value: schemaID})
		return Message{}, ErrSchemaRegistry
	}

	encoded, err := p.schemaRegistry.EncodeMessage(ctx, schemaID, data)
	if err != nil {
		p.logger.Error(ctx, "Failed to encode message with schema",
			logger.Field{Key: "topic", Value: topic},
			logger.Field{Key: "schema_id", Value: schemaID},
			logger.Field{Key: "error", Value: err})
		return Message{}, err
	}

	msg := Message{
		Topic: topic,
		Key:   key,
		Value: encoded,
		Headers: map[string]string{
			"x-schema-id": fmt.Sprintf("%d", schemaID),
		},
	}

	if err := p.Publish(ctx, msg); err != nil {
		return msg, err
	}

	return msg, nil
}

func (p *KafkaProducer) generateMessageID(msg Message) string {
	h := sha256.New()
	h.Write([]byte(msg.Topic))
	h.Write(msg.Key)
	h.Write(msg.Value)

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

	return fmt.Sprintf("%s-%s", hash[:16], uuid.New().String()[:8])
}

func (p *KafkaProducer) isDuplicate(msgID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	entry, exists := p.cache[msgID]
	if !exists {
		return false
	}

	if time.Since(entry.timestamp) > p.idempotencyConfig.WindowSize {
		return false
	}

	return true
}

func (p *KafkaProducer) cacheMessage(msgID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.cache) >= p.idempotencyConfig.MaxCacheSize {
		p.evictOldest()
	}

	p.cache[msgID] = messageCacheEntry{
		id:        msgID,
		timestamp: time.Now(),
	}
}

func (p *KafkaProducer) evictOldest() {
	targetSize := p.idempotencyConfig.MaxCacheSize / 2

	now := time.Now()
	for id, entry := range p.cache {
		if now.Sub(entry.timestamp) > p.idempotencyConfig.WindowSize {
			delete(p.cache, id)
		}
	}

	if len(p.cache) > targetSize {
		type kv struct {
			id        string
			timestamp time.Time
		}
		entries := make([]kv, 0, len(p.cache))
		for id, entry := range p.cache {
			entries = append(entries, kv{id: id, timestamp: entry.timestamp})
		}

		for i := 0; i < len(entries)-1; i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[j].timestamp.Before(entries[i].timestamp) {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}

		for i := 0; i < len(entries) && len(p.cache) > targetSize; i++ {
			delete(p.cache, entries[i].id)
		}
	}
}

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

func (p *KafkaProducer) Close() error {
	p.idempotencyEnabled = false
	return nil
}

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

func (p *KafkaProducer) SetIdempotencyEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if enabled && !p.idempotencyEnabled {
		p.idempotencyEnabled = enabled
		go p.cleanupCache()
	} else {
		p.idempotencyEnabled = enabled
	}
}
