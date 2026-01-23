package kafka

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
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
}

func NewKafkaProducer(cfg *ProducerConfig, brokers []string, dialer *kafkago.Dialer) *KafkaProducer {
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
	}
}

func (p *KafkaProducer) Publish(ctx context.Context, msg Message) error {
	if p.writer == nil {
		return ErrProducerNotInitialized
	}

	timer := prometheus.NewTimer(ProducerSendLatency.WithLabelValues(msg.Topic))
	defer timer.ObserveDuration()

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
		ProducerSendErrors.WithLabelValues(msg.Topic, "write_failed").Inc()
		return err
	}

	ProducerMessagesSent.WithLabelValues(msg.Topic, p.compressionType).Inc()
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
