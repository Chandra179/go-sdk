package kafka

import (
	"context"
	"fmt"
	"time"

	"gosdk/pkg/logger"

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

type KafkaProducer struct {
	writer          *kafkago.Writer
	compressionType string
	logger          logger.Logger
	maxMessageSize  int
}

func NewKafkaProducer(cfg *ProducerConfig, brokers []string, dialer *kafkago.Dialer, logger logger.Logger) (*KafkaProducer, error) {
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

	// Get the compression codec - compressionCodeMap already returns the int code
	var compressionCodec kafkago.CompressionCodec
	if int(compressionCode) < len(kafkacompress.Codecs) {
		compressionCodec = kafkacompress.Codecs[compressionCode]
	}

	writer := kafkago.NewWriter(kafkago.WriterConfig{
		Brokers:          brokers,
		Balancer:         &kafkago.Hash{}, // Use Hash for key-based ordering
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

	return &KafkaProducer{
		writer:          writer,
		compressionType: cfg.CompressionType,
		logger:          logger,
		maxMessageSize:  maxSize,
	}, nil
}

func (p *KafkaProducer) Publish(ctx context.Context, msg Message) error {
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

	err := p.writer.WriteMessages(ctx, kafkaMsg)
	if err != nil {
		p.logger.Error(ctx, "Failed to publish message",
			logger.Field{Key: "error", Value: err},
			logger.Field{Key: "topic", Value: msg.Topic},
			logger.Field{Key: "compression", Value: p.compressionType})
		return err
	}

	p.logger.Debug(ctx, "Message published successfully",
		logger.Field{Key: "topic", Value: msg.Topic},
		logger.Field{Key: "compression", Value: p.compressionType})
	return nil
}

func (p *KafkaProducer) Close() error {
	if p.writer != nil {
		return p.writer.Close()
	}
	return nil
}

var compressionCodeMap = map[string]int{
	validCompressionNone:   int(kafkacompress.None),
	validCompressionGzip:   int(kafkacompress.Gzip),
	validCompressionSnappy: int(kafkacompress.Snappy),
	validCompressionLz4:    int(kafkacompress.Lz4),
	validCompressionZstd:   int(kafkacompress.Zstd),
}

var acksMap = map[string]kafkago.RequiredAcks{
	validAcksAll:    kafkago.RequireAll,
	validAcksNone:   kafkago.RequireNone,
	validAcksLeader: kafkago.RequireOne,
}
