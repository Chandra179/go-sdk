package kafka

import (
	"context"
	"errors"
	"time"
)

var (
	ErrProducerNotInitialized   = errors.New("producer not initialized")
	ErrConsumerNotInitialized   = errors.New("consumer not initialized")
	ErrInvalidMessage           = errors.New("invalid message")
	ErrTopicNotFound            = errors.New("topic not found")
	ErrKafkaConnection          = errors.New("kafka connection error")
	ErrKafkaPublish             = errors.New("kafka publish error")
	ErrKafkaHealthCheck         = errors.New("kafka health check failed")
	ErrTLSConfiguration         = errors.New("failed to configure TLS")
	ErrInvalidCompression       = errors.New("invalid compression type, must be one of: none, gzip, snappy, lz4, zstd")
	ErrInvalidAcks              = errors.New("invalid acks value, must be one of: all, none, leader")
	ErrRequiredCompression      = errors.New("compression type is required")
	ErrRequiredBrokers          = errors.New("brokers list is required")
	ErrMessageTooLarge          = errors.New("message exceeds maximum size")
	ErrInvalidPartitionStrategy = errors.New("invalid partition strategy, must be one of: hash, roundrobin, leastbytes")

	// Schema Registry errors
	ErrSchemaRegistry = errors.New("schema registry error")
	ErrSchemaEncode   = errors.New("schema encoding error")
	ErrSchemaDecode   = errors.New("schema decoding error")
	ErrInvalidSchema  = errors.New("invalid schema")

	// Transaction errors
	ErrTransaction        = errors.New("transaction error")
	ErrTransactionAborted = errors.New("transaction aborted")
	ErrTransactionTimeout = errors.New("transaction timeout")
	ErrInvalidTransaction = errors.New("invalid transaction state")
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

// PartitionStrategy defines how messages are partitioned
type PartitionStrategy string

const (
	PartitionStrategyHash       PartitionStrategy = "hash"       // Consistent hashing by key
	PartitionStrategyRoundRobin PartitionStrategy = "roundrobin" // Round-robin distribution
	PartitionStrategyLeastBytes PartitionStrategy = "leastbytes" // Send to partition with least bytes
)

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

// IdempotencyConfig holds configuration for idempotent producer
type IdempotencyConfig struct {
	Enabled      bool          // Whether idempotency is enabled
	WindowSize   time.Duration // Time window for deduplication (default: 5 minutes)
	MaxCacheSize int           // Maximum number of message IDs to cache (default: 10000)
	KeyPrefix    string        // Optional prefix for message IDs
}

// DefaultIdempotencyConfig returns sensible defaults
func DefaultIdempotencyConfig() IdempotencyConfig {
	return IdempotencyConfig{
		Enabled:      true,
		WindowSize:   5 * time.Minute,
		MaxCacheSize: 10000,
		KeyPrefix:    "",
	}
}

// IdempotencyStats holds statistics about the idempotent producer
type IdempotencyStats struct {
	Enabled      bool
	CacheSize    int
	MaxCacheSize int
	WindowSize   time.Duration
	WindowStart  time.Time
}
