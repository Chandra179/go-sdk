package kafka

import (
	"context"
	"os"
	"testing"
	"time"

	"gosdk/pkg/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
)

var (
	testKafkaContainer *kafka.KafkaContainer
	testBrokerAddress  string
	testLogger         logger.Logger
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testKafkaContainer, testBrokerAddress, err = setupKafkaContainer(ctx)
	if err != nil {
		// If we can't start Kafka, skip tests
		os.Exit(m.Run())
	}

	testLogger = logger.NewLogger("test")

	code := m.Run()

	// Teardown
	if testKafkaContainer != nil {
		terminateContainer(ctx, testKafkaContainer)
	}

	os.Exit(code)
}

func setupKafkaContainer(ctx context.Context) (*kafka.KafkaContainer, string, error) {
	kafkaContainer, err := kafka.Run(ctx,
		"confluentinc/confluent-local:7.6.0",
		kafka.WithClusterID("test-cluster-123"),
	)
	if err != nil {
		return nil, "", err
	}

	brokers, err := kafkaContainer.Brokers(ctx)
	if err != nil {
		terminateContainer(ctx, kafkaContainer)
		return nil, "", err
	}

	brokerAddress := brokers[0]
	return kafkaContainer, brokerAddress, nil
}

func terminateContainer(ctx context.Context, container *kafka.KafkaContainer) {
	if container == nil {
		return
	}
	if err := container.Terminate(ctx); err != nil {
		// Log but don't fail
		_ = err
	}
}

func requireKafka(t *testing.T) {
	t.Helper()
	if testKafkaContainer == nil {
		t.Skip("Kafka container not available")
	}
}

func getTestConfig() *Config {
	return &Config{
		Brokers: []string{testBrokerAddress},
		Producer: ProducerConfig{
			RequiredAcks:      "all",
			BatchSize:         1,
			LingerMs:          10,
			CompressionType:   "none",
			MaxAttempts:       3,
			Async:             false,
			MaxMessageSize:    1024 * 1024,
			PartitionStrategy: PartitionStrategyHash,
		},
		Consumer: ConsumerConfig{
			MinBytes:              1024,
			MaxBytes:              10 * 1024 * 1024,
			CommitInterval:        1 * time.Second,
			MaxPollRecords:        100,
			HeartbeatInterval:     3 * time.Second,
			WatchPartitionChanges: true,
			StartOffset:           StartOffsetEarliest,
		},
		Security: SecurityConfig{
			Enabled: false,
		},
		Retry: RetryConfig{
			MaxRetries:           3,
			InitialBackoff:       100,
			MaxBackoff:           1000,
			DLQEnabled:           true,
			DLQTopicPrefix:       "",
			ShortRetryAttempts:   3,
			MaxLongRetryAttempts: 3,
			RetryTopicSuffix:     ".retry",
		},
		Idempotency: IdempotencyConfig{
			Enabled:      true,
			WindowSize:   5 * time.Minute,
			MaxCacheSize: 1000,
		},
	}
}

func createTopic(t *testing.T, ctx context.Context, topic string) {
	t.Helper()

	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer client.Close()

	cfg := DefaultAdminConfig()
	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, &cfg)
	require.NoError(t, err)
	defer admin.Close()

	err = admin.CreateTopic(ctx, topic, 1, 1, nil)
	require.NoError(t, err)
}

func publishTestMessage(t *testing.T, ctx context.Context, topic string, key, value []byte, headers map[string]string) Message {
	t.Helper()

	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer client.Close()

	producer, err := client.Producer()
	require.NoError(t, err)
	defer producer.Close()

	msg := Message{
		Topic:   topic,
		Key:     key,
		Value:   value,
		Headers: headers,
	}

	err = producer.Publish(ctx, msg)
	require.NoError(t, err)

	return msg
}

func consumeMessages(t *testing.T, ctx context.Context, client Client, groupID string, topics []string, expectedCount int, timeout time.Duration) []Message {
	t.Helper()

	consumer, err := client.Consumer(groupID, topics)
	require.NoError(t, err)

	messages := make([]Message, 0, expectedCount)
	done := make(chan error, 1)

	go func() {
		handler := func(msg Message) error {
			messages = append(messages, msg)
			if len(messages) >= expectedCount {
				return nil
			}
			return nil
		}
		done <- consumer.Start(ctx, handler)
	}()

	select {
	case <-ctx.Done():
		t.Fatal("context cancelled while waiting for messages")
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for messages: got %d, want %d", len(messages), expectedCount)
	case err := <-done:
		require.NoError(t, err)
	}

	return messages
}

func assertMessageReceived(t *testing.T, messages []Message, expectedKey, expectedValue []byte) {
	t.Helper()
	found := false
	for _, msg := range messages {
		if string(msg.Key) == string(expectedKey) {
			assert.Equal(t, expectedValue, msg.Value)
			found = true
			break
		}
	}
	assert.True(t, found, "expected message with key %s not found", string(expectedKey))
}
