package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

type Producer struct {
	client *kgo.Client
}

func (p *Producer) SendMessage(ctx context.Context, topic string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	record := &kgo.Record{
		Topic: topic,
		Value: data,
		// Consider adding Key for partitioning
	}

	results := p.client.ProduceSync(ctx, record)
	return results.FirstErr()
}

// NewProducer creates a new Producer instance with the given Kafka client
func NewProducer(client *kgo.Client) *Producer {
	return &Producer{
		client: client,
	}
}
