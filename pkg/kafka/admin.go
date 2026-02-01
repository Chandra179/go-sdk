package kafka

import (
	"context"
	"fmt"

	kafkago "github.com/segmentio/kafka-go"
)

// KafkaAdmin provides administrative operations for Kafka
type KafkaAdmin struct {
	brokers []string
	dialer  *kafkago.Dialer
}

// AdminConfig holds configuration for Kafka admin operations
type AdminConfig struct {
	DefaultPartitions     int
	DefaultReplication    int
	AutoCreateDLQTopics   bool
	AutoCreateRetryTopics bool
}

// DefaultAdminConfig returns sensible defaults for admin configuration
func DefaultAdminConfig() AdminConfig {
	return AdminConfig{
		DefaultPartitions:     3,
		DefaultReplication:    2,
		AutoCreateDLQTopics:   true,
		AutoCreateRetryTopics: true,
	}
}

// NewKafkaAdmin creates a new Kafka admin client
func NewKafkaAdmin(brokers []string, dialer *kafkago.Dialer) (*KafkaAdmin, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("%w: at least one broker required", ErrRequiredBrokers)
	}

	return &KafkaAdmin{
		brokers: brokers,
		dialer:  dialer,
	}, nil
}

// EnsureTopicExists checks if a topic exists and creates it if necessary
func (a *KafkaAdmin) EnsureTopicExists(ctx context.Context, topic string, partitions int, replicationFactor int) error {
	// Validate inputs
	if topic == "" {
		return fmt.Errorf("topic name cannot be empty")
	}
	if partitions <= 0 {
		partitions = 3
	}
	if replicationFactor <= 0 {
		replicationFactor = 1
	}

	// Check if topic exists by trying to create a connection to it
	exists, err := a.TopicExists(ctx, topic)
	if err != nil {
		return fmt.Errorf("failed to check topic existence: %w", err)
	}
	if exists {
		return nil // Topic already exists
	}

	// Create topic using kafka-go's topic creation via connection to controller
	conn, err := a.dialer.DialContext(ctx, "tcp", a.brokers[0])
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka: %w", err)
	}
	defer conn.Close()

	// Create topic
	topicConfigs := []kafkago.TopicConfig{
		{
			Topic:             topic,
			NumPartitions:     partitions,
			ReplicationFactor: replicationFactor,
			ConfigEntries: []kafkago.ConfigEntry{
				{
					ConfigName:  "retention.ms",
					ConfigValue: "604800000", // 7 days default retention
				},
			},
		},
	}

	err = conn.CreateTopics(topicConfigs...)
	if err != nil {
		// Check if the error is because topic already exists
		if err.Error() == "topic already exists" {
			return nil
		}
		return fmt.Errorf("failed to create topic %s: %w", topic, err)
	}

	return nil
}

// InitializeTopicsForConsumer ensures DLQ and retry topics exist for given topics
func (a *KafkaAdmin) InitializeTopicsForConsumer(ctx context.Context, topics []string, cfg RetryConfig) error {
	if !cfg.DLQEnabled {
		return nil
	}

	for _, topic := range topics {
		// Ensure DLQ topic exists
		dlqTopic := buildDLQTopic(topic, cfg.DLQTopicPrefix)
		if err := a.EnsureTopicExists(ctx, dlqTopic, 3, 2); err != nil {
			return fmt.Errorf("failed to ensure DLQ topic %s: %w", dlqTopic, err)
		}

		// Ensure retry topic exists
		retryTopic := buildRetryTopic(topic, cfg.RetryTopicSuffix)
		if err := a.EnsureTopicExists(ctx, retryTopic, 3, 2); err != nil {
			return fmt.Errorf("failed to ensure retry topic %s: %w", retryTopic, err)
		}
	}

	return nil
}

// Close closes the admin client
func (a *KafkaAdmin) Close() error {
	return nil
}

// TopicExists checks if a topic exists
func (a *KafkaAdmin) TopicExists(ctx context.Context, topic string) (bool, error) {
	conn, err := a.dialer.DialContext(ctx, "tcp", a.brokers[0])
	if err != nil {
		return false, fmt.Errorf("failed to connect to Kafka: %w", err)
	}
	defer conn.Close()

	// List all topics
	partitions, err := conn.ReadPartitions()
	if err != nil {
		return false, fmt.Errorf("failed to read partitions: %w", err)
	}

	for _, p := range partitions {
		if p.Topic == topic {
			return true, nil
		}
	}

	return false, nil
}

// DeleteTopic deletes a topic (use with caution!)
func (a *KafkaAdmin) DeleteTopic(ctx context.Context, topic string) error {
	conn, err := a.dialer.DialContext(ctx, "tcp", a.brokers[0])
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka: %w", err)
	}
	defer conn.Close()

	err = conn.DeleteTopics(topic)
	if err != nil {
		return fmt.Errorf("failed to delete topic %s: %w", topic, err)
	}

	return nil
}
