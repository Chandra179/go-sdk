package kafka

import (
	"context"
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

func NewKafkaProducer(cfg *ProducerConfig, brokers []string, dialer *kafkago.Dialer, logger logger.Logger) *KafkaProducer {
	acks := acksMap[cfg.RequiredAcks]
	compression := kafkago.Compression(compressionCodeMap[cfg.CompressionType])

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
		CompressionCodec: kafkacompress.Codecs[compression],
	})
	return &KafkaProducer{
		writer:          writer,
		compressionType: cfg.CompressionType,
		logger:          logger,
	}
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
