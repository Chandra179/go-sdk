package kafka

import (
	"context"

	kafkago "github.com/segmentio/kafka-go"
)

type KafkaProducer struct {
	writer *kafkago.Writer
}

func NewKafkaProducer(brokers []string) *KafkaProducer {
	writer := &kafkago.Writer{
		Addr:     kafkago.TCP(brokers...),
		Balancer: &kafkago.LeastBytes{},
	}
	return &KafkaProducer{writer: writer}
}

func (p *KafkaProducer) Publish(ctx context.Context, msg Message) error {
	if p.writer == nil {
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

	return p.writer.WriteMessages(ctx, kafkaMsg)
}

func (p *KafkaProducer) Close() error {
	if p.writer != nil {
		return p.writer.Close()
	}
	return nil
}
