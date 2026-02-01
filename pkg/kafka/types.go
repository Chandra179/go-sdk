package kafka

import (
	"context"

	"errors"
)

var (
	ErrProducerNotInitialized = errors.New("producer not initialized")
	ErrConsumerNotInitialized = errors.New("consumer not initialized")
	ErrInvalidMessage         = errors.New("invalid message")
	ErrTopicNotFound          = errors.New("topic not found")
	ErrKafkaConnection        = errors.New("kafka connection error")
	ErrKafkaPublish           = errors.New("kafka publish error")
	ErrKafkaHealthCheck       = errors.New("kafka health check failed")
	ErrTLSConfiguration       = errors.New("failed to configure TLS")
	ErrInvalidCompression     = errors.New("invalid compression type, must be one of: none, gzip, snappy, lz4, zstd")
	ErrInvalidAcks            = errors.New("invalid acks value, must be one of: all, none, leader")
	ErrRequiredCompression    = errors.New("compression type is required")
	ErrRequiredBrokers        = errors.New("brokers list is required")
	ErrMessageTooLarge        = errors.New("message exceeds maximum size")
)

type Message struct {
	Topic   string
	Key     []byte
	Value   []byte
	Headers map[string]string
}

type ConsumerHandler func(msg Message) error

type Client interface {
	Producer() (Producer, error)
	Consumer(groupID string, topics []string) (Consumer, error)
	Ping(ctx context.Context) error
	Close() error
}

type Producer interface {
	Publish(ctx context.Context, msg Message) error
	Close() error
}

type Consumer interface {
	Start(ctx context.Context, handler ConsumerHandler) error
	Close() error
}

// StartOffset defines where to start consuming messages
type StartOffset int

const (
	StartOffsetEarliest StartOffset = iota // Read from beginning
	StartOffsetLatest                      // Read from end (real-time)
	StartOffsetNone                        // Fail if no offset exists
)

func (s StartOffset) String() string {
	switch s {
	case StartOffsetEarliest:
		return "earliest"
	case StartOffsetLatest:
		return "latest"
	case StartOffsetNone:
		return "none"
	default:
		return "unknown"
	}
}

type RetryConfig struct {
	MaxRetries           int64
	InitialBackoff       int64
	MaxBackoff           int64
	DLQEnabled           bool
	DLQTopicPrefix       string
	ShortRetryAttempts   int
	MaxLongRetryAttempts int
	RetryTopicSuffix     string
}
