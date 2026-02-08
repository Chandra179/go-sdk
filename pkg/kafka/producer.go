package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Message represents a message to be produced to Kafka.
type Message struct {
	Topic    string
	Payload  any
	Key      string
	Headers  map[string]string
	Callback func(*kgo.Record, error)
}

// Producer wraps a Kafka client for producing messages.
type Producer struct {
	client  *kgo.Client
	logger  *slog.Logger
	metrics *Metrics
}

// ProducerOption configures a Producer.
type ProducerOption func(*Producer)

// WithProducerLogger sets the logger for the producer.
func WithProducerLogger(logger *slog.Logger) ProducerOption {
	return func(p *Producer) {
		p.logger = logger
	}
}

// WithProducerMetrics sets the metrics collector for the producer.
func WithProducerMetrics(metrics *Metrics) ProducerOption {
	return func(p *Producer) {
		p.metrics = metrics
	}
}

// NewProducer creates a new Producer with the given client and options.
func NewProducer(client *kgo.Client, opts ...ProducerOption) *Producer {
	p := &Producer{
		client: client,
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Produce sends a message asynchronously.
func (p *Producer) Produce(ctx context.Context, msg *Message) error {
	data, err := p.marshalPayload(msg.Payload)
	if err != nil {
		return err
	}

	headers := make([]kgo.RecordHeader, 0, len(msg.Headers))
	for k, v := range msg.Headers {
		headers = append(headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}

	record := &kgo.Record{
		Topic:   msg.Topic,
		Key:     []byte(msg.Key),
		Value:   data,
		Headers: headers,
	}

	start := time.Now()
	callback := msg.Callback
	if callback == nil {
		callback = func(r *kgo.Record, err error) {
			duration := time.Since(start)
			if err != nil {
				p.logger.Error("produce error",
					"topic", r.Topic,
					"error", err,
				)
				if p.metrics != nil {
					p.metrics.RecordProduce(r.Topic, duration, false)
				}
			} else if p.metrics != nil {
				p.metrics.RecordProduce(r.Topic, duration, true)
			}
		}
	}

	p.client.Produce(ctx, record, callback)

	return nil
}

// ProduceSync sends a message and waits for acknowledgment.
func (p *Producer) ProduceSync(ctx context.Context, msg *Message) error {
	data, err := p.marshalPayload(msg.Payload)
	if err != nil {
		return err
	}

	headers := make([]kgo.RecordHeader, 0, len(msg.Headers))
	for k, v := range msg.Headers {
		headers = append(headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}

	record := &kgo.Record{
		Topic:   msg.Topic,
		Key:     []byte(msg.Key),
		Value:   data,
		Headers: headers,
	}

	start := time.Now()
	done := make(chan error, 1)

	p.client.Produce(ctx, record, func(r *kgo.Record, err error) {
		duration := time.Since(start)
		if p.metrics != nil {
			p.metrics.RecordProduce(r.Topic, duration, err == nil)
		}
		done <- err
		close(done)
	})

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SendMessage is a convenience method to send a message with just topic and payload.
func (p *Producer) SendMessage(ctx context.Context, topic string, payload any) error {
	return p.Produce(ctx, &Message{Topic: topic, Payload: payload})
}

// SendMessageSync is a convenience method to send a message synchronously.
func (p *Producer) SendMessageSync(ctx context.Context, topic string, payload any) error {
	return p.ProduceSync(ctx, &Message{Topic: topic, Payload: payload})
}

// Flush waits for all buffered records to be sent.
func (p *Producer) Flush(ctx context.Context) error {
	return p.client.Flush(ctx)
}

// Close flushes pending messages and indicates the producer should not be used.
func (p *Producer) Close(ctx context.Context) error {
	if err := p.client.Flush(ctx); err != nil {
		p.logger.Error("error flushing producer", "error", err)
		return err
	}
	return nil
}

// SendToDLQ sends a message to the dead letter queue asynchronously.
func (p *Producer) SendToDLQ(ctx context.Context, originalTopic string, value []byte, key []byte, dlqErr error) error {
	dlqTopic := originalTopic + ".dlq"

	headers := map[string]string{
		"origin_topic":  originalTopic,
		"dlq_timestamp": time.Now().Format(time.RFC3339),
		"dlq_error":     dlqErr.Error(),
	}

	var keyStr string
	if len(key) > 0 {
		keyStr = string(key)
	}

	return p.Produce(ctx, &Message{
		Topic:   dlqTopic,
		Payload: value,
		Key:     keyStr,
		Headers: headers,
	})
}

// SendToDLQSync sends a message to the dead letter queue and waits for acknowledgment.
func (p *Producer) SendToDLQSync(ctx context.Context, originalTopic string, value []byte, key []byte, dlqErr error) error {
	dlqTopic := originalTopic + ".dlq"

	headers := []kgo.RecordHeader{
		{Key: "origin_topic", Value: []byte(originalTopic)},
		{Key: "dlq_timestamp", Value: []byte(time.Now().Format(time.RFC3339))},
		{Key: "dlq_error", Value: []byte(dlqErr.Error())},
	}

	start := time.Now()
	done := make(chan error, 1)

	record := &kgo.Record{
		Topic:   dlqTopic,
		Key:     key,
		Value:   value,
		Headers: headers,
	}

	p.client.Produce(ctx, record, func(r *kgo.Record, err error) {
		duration := time.Since(start)
		if p.metrics != nil {
			p.metrics.RecordProduce(r.Topic, duration, err == nil)
		}
		done <- err
		close(done)
	})

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Producer) marshalPayload(payload any) ([]byte, error) {
	if b, ok := payload.([]byte); ok {
		return b, nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %w", err)
	}
	return data, nil
}
