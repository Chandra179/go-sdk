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
)

type KafkaProducer struct {
	writer          *kafkago.Writer
	compressionType string
	logger          logger.Logger
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
		Balancer:         &kafkago.LeastBytes{},
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
	return &KafkaProducer{
		writer:          writer,
		compressionType: cfg.CompressionType,
		logger:          logger,
	}, nil
}

func (p *KafkaProducer) Publish(ctx context.Context, msg Message) error {
	if p.writer == nil {
		p.logger.Error(ctx, "Producer not initialized",
			logger.Field{Key: "topic", Value: msg.Topic})
		return ErrProducerNotInitialized
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
