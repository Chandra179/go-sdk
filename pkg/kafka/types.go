package kafka

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
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
	PingDetailed(ctx context.Context) *PingResult
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

// PingResult holds detailed result of a cluster ping operation
type PingResult struct {
	Healthy       bool           // True if any broker is reachable
	AllHealthy    bool           // True if all brokers are reachable
	Brokers       []BrokerStatus // Status of each broker
	Reachable     int            // Number of reachable brokers
	Total         int            // Total number of brokers
	Controller    *BrokerStatus  // Controller status if known
	MetadataError error          // Error fetching cluster metadata
}

// BrokerStatus represents the health status of a single broker
type BrokerStatus struct {
	Address      string        // Broker address (host:port)
	Healthy      bool          // Whether broker is reachable
	Error        error         // Connection error if any
	Latency      time.Duration // Connection latency
	IsController bool          // Whether this broker is the controller
}

// ClusterHealth holds comprehensive cluster health information
type ClusterHealth struct {
	Status       string        // "healthy", "degraded", "unhealthy"
	Timestamp    time.Time     // When check was performed
	Connectivity PingResult    // Broker connectivity status
	Metadata     *MetadataInfo // Cluster metadata if available
}

// MetadataInfo holds cluster metadata from the controller
type MetadataInfo struct {
	Topics     int      // Number of topics
	Partitions int      // Total partitions across all topics
	Brokers    []string // List of all brokers in cluster
	Controller string   // Controller broker address
}

// String returns a human-readable representation of PingResult
func (p PingResult) String() string {
	status := "unhealthy"
	if p.Healthy {
		if p.AllHealthy {
			status = "all-healthy"
		} else {
			status = "partially-healthy"
		}
	}
	return fmt.Sprintf("Kafka cluster: %s (%d/%d brokers reachable)", status, p.Reachable, p.Total)
}

// Healthy returns true if at least one broker is reachable
func (p PingResult) IsHealthy() bool {
	return p.Healthy
}

// Degraded returns true if some but not all brokers are reachable
func (p PingResult) IsDegraded() bool {
	return p.Healthy && !p.AllHealthy
}

// toRecord converts our Message type to a kgo.Record for franz-go
func (m *Message) toRecord() *kgo.Record {
	headers := make([]kgo.RecordHeader, 0, len(m.Headers))
	for k, v := range m.Headers {
		headers = append(headers, kgo.RecordHeader{
			Key:   k,
			Value: []byte(v),
		})
	}

	return &kgo.Record{
		Topic:   m.Topic,
		Key:     m.Key,
		Value:   m.Value,
		Headers: headers,
	}
}

// fromRecord converts a kgo.Record back to our Message type
func fromRecord(r *kgo.Record) Message {
	headers := make(map[string]string, len(r.Headers))
	for _, h := range r.Headers {
		headers[h.Key] = string(h.Value)
	}

	return Message{
		Topic:   r.Topic,
		Key:     r.Key,
		Value:   r.Value,
		Headers: headers,
	}
}
