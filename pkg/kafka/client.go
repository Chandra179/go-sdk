package kafka

import (
	"context"
)

type Producer interface {
	Publish(ctx context.Context, msg Message) error
	Close() error
}

type Consumer interface {
	Subscribe(ctx context.Context, topics []string, handler ConsumerHandler) error
	Close() error
}

type Client interface {
	Producer() (Producer, error)
	Consumer(groupID string) (Consumer, error)
	Close() error
}
