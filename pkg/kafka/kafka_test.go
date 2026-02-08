package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

// setupKafkaContainer creates a Kafka container for testing
func setupKafkaContainer(t *testing.T) (*kafka.KafkaContainer, []string) {
	t.Helper()

	ctx := context.Background()

	// Create Kafka container using testcontainers
	kafkaContainer, err := kafka.Run(ctx,
		"confluentinc/confluent-local:7.5.0",
		testcontainers.WithEnv(map[string]string{
			"KAFKA_AUTO_CREATE_TOPICS_ENABLE": "true",
		}),
	)
	require.NoError(t, err, "failed to start Kafka container")

	// Get the broker address
	brokers, err := kafkaContainer.Brokers(ctx)
	require.NoError(t, err, "failed to get Kafka brokers")

	return kafkaContainer, brokers
}

// setupTestLogger creates a logger for testing
func setupTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// TestMessage represents a test message structure
type TestMessage struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// createAdminClient creates a simple admin client for topic operations
func createAdminClient(t *testing.T, brokers []string) *kadm.Client {
	t.Helper()

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
	)
	require.NoError(t, err)

	return kadm.NewClient(client)
}

// createTestClient creates a Kafka client with custom options for testing
func createTestClient(t *testing.T, logger *slog.Logger, brokers []string, topics []string, groupID string) *kgo.Client {
	t.Helper()

	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.ConsumeTopics(topics...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordRetries(10),
		kgo.RequestRetries(3),
		kgo.RetryBackoffFn(func(tries int) time.Duration {
			return time.Duration(tries) * 100 * time.Millisecond
		}),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxWait(500 * time.Millisecond),
		kgo.FetchMinBytes(1),
		kgo.FetchMaxBytes(50 * 1024 * 1024),
		kgo.RequestTimeoutOverhead(10 * time.Second),
		kgo.ConnIdleTimeout(60 * time.Second),
		kgo.WithLogger(&SlogShim{L: logger}),
	}

	client, err := kgo.NewClient(opts...)
	require.NoError(t, err, "failed to create Kafka client")

	return client
}

// createProducerClient creates a Kafka client for producing messages (no consumer group)
func createProducerClient(t *testing.T, logger *slog.Logger, brokers []string) *kgo.Client {
	t.Helper()

	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordRetries(10),
		kgo.RequestRetries(3),
		kgo.RetryBackoffFn(func(tries int) time.Duration {
			return time.Duration(tries) * 100 * time.Millisecond
		}),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),
		kgo.RequestTimeoutOverhead(10 * time.Second),
		kgo.ConnIdleTimeout(60 * time.Second),
		kgo.WithLogger(&SlogShim{L: logger}),
	}

	client, err := kgo.NewClient(opts...)
	require.NoError(t, err, "failed to create Kafka client")

	return client
}

// createTopic creates a Kafka topic using the admin client
func createTopic(ctx context.Context, adminClient *kadm.Client, topic string) error {
	_, err := adminClient.CreateTopics(ctx, 1, 1, nil, topic)
	return err
}

func TestNewProducer(t *testing.T) {
	t.Run("creates producer with client", func(t *testing.T) {
		ctx := context.Background()
		container, brokers := setupKafkaContainer(t)
		defer func() {
			if err := container.Terminate(ctx); err != nil {
				t.Logf("failed to terminate container: %v", err)
			}
		}()

		logger := setupTestLogger()
		client := createTestClient(t, logger, brokers, []string{"test-topic"}, "test-producer-group")
		defer client.Close()

		producer := NewProducer(client)
		assert.NotNil(t, producer)
		assert.Equal(t, client, producer.client)
	})
}

func TestProducer_SendMessage_Integration(t *testing.T) {
	t.Run("successfully sends and receives message", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		// Setup Kafka container
		container, brokers := setupKafkaContainer(t)
		defer func() {
			if err := container.Terminate(ctx); err != nil {
				t.Logf("failed to terminate container: %v", err)
			}
		}()

		topic := "test-integration-topic"
		logger := setupTestLogger()

		// Create topic using admin client
		adminClient := createAdminClient(t, brokers)
		err := createTopic(ctx, adminClient, topic)
		adminClient.Close()
		require.NoError(t, err, "failed to create topic")

		// Create producer client (no consumer group needed for producing)
		producerClient := createProducerClient(t, logger, brokers)
		defer producerClient.Close()

		producer := NewProducer(producerClient)

		// Create consumer first (before producing) with unique group ID
		consumerClient := createTestClient(t, logger, brokers, []string{topic}, "test-consumer-group-1")
		defer consumerClient.Close()

		// Poll for messages in background
		received := make(chan *kgo.Record, 1)
		done := make(chan bool, 1)

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					fetches := consumerClient.PollFetches(ctx)
					if fetches.IsClientClosed() {
						return
					}
					if errs := fetches.Errors(); len(errs) > 0 {
						for _, err := range errs {
							t.Logf("Fetch error: %v", err)
						}
						continue
					}
					fetches.EachRecord(func(r *kgo.Record) {
						select {
						case received <- r:
							select {
							case done <- true:
							default:
							}
						default:
						}
					})
				}
			}
		}()

		// Give consumer time to start
		time.Sleep(1 * time.Second)

		// Send a test message
		testMsg := TestMessage{
			ID:        "msg-001",
			Content:   "Hello, Kafka!",
			Timestamp: time.Now(),
		}

		err = producer.SendMessage(ctx, topic, testMsg)
		require.NoError(t, err, "failed to send message")

		// Wait for message with timeout
		select {
		case <-done:
			record := <-received
			require.NotNil(t, record)
			assert.Equal(t, topic, record.Topic)

			var receivedMsg TestMessage
			err = json.Unmarshal(record.Value, &receivedMsg)
			require.NoError(t, err)

			assert.Equal(t, testMsg.ID, receivedMsg.ID)
			assert.Equal(t, testMsg.Content, receivedMsg.Content)
		case <-time.After(15 * time.Second):
			t.Fatal("timeout waiting for message")
		}
	})

	t.Run("sends multiple messages", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		container, brokers := setupKafkaContainer(t)
		defer func() {
			if err := container.Terminate(ctx); err != nil {
				t.Logf("failed to terminate container: %v", err)
			}
		}()

		topic := "test-multi-messages"
		logger := setupTestLogger()

		// Create topic
		adminClient := createAdminClient(t, brokers)
		err := createTopic(ctx, adminClient, topic)
		adminClient.Close()
		require.NoError(t, err)

		// Create producer
		producerClient := createProducerClient(t, logger, brokers)
		defer producerClient.Close()

		producer := NewProducer(producerClient)

		// Create consumer first
		consumerClient := createTestClient(t, logger, brokers, []string{topic}, "test-consumer-group-2")
		defer consumerClient.Close()

		// Consume messages
		receivedCount := 0
		done := make(chan bool, 1)

		go func() {
			for receivedCount < 3 {
				select {
				case <-ctx.Done():
					return
				default:
					fetches := consumerClient.PollFetches(ctx)
					if fetches.IsClientClosed() {
						return
					}
					if errs := fetches.Errors(); len(errs) > 0 {
						continue
					}
					fetches.EachRecord(func(r *kgo.Record) {
						receivedCount++
						if receivedCount >= 3 {
							select {
							case done <- true:
							default:
							}
						}
					})
				}
			}
		}()

		// Give consumer time to start
		time.Sleep(1 * time.Second)

		// Send multiple messages
		messages := []TestMessage{
			{ID: "msg-001", Content: "First message", Timestamp: time.Now()},
			{ID: "msg-002", Content: "Second message", Timestamp: time.Now()},
			{ID: "msg-003", Content: "Third message", Timestamp: time.Now()},
		}

		for _, msg := range messages {
			err := producer.SendMessage(ctx, topic, msg)
			require.NoError(t, err)
		}

		select {
		case <-done:
			assert.Equal(t, 3, receivedCount)
		case <-time.After(20 * time.Second):
			t.Fatalf("timeout waiting for messages, received %d/3", receivedCount)
		}
	})

	t.Run("fails to send with invalid payload", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		container, brokers := setupKafkaContainer(t)
		defer func() {
			if err := container.Terminate(ctx); err != nil {
				t.Logf("failed to terminate container: %v", err)
			}
		}()

		topic := "test-invalid-payload"
		logger := setupTestLogger()

		// Create topic
		adminClient := createAdminClient(t, brokers)
		require.NoError(t, createTopic(ctx, adminClient, topic))
		adminClient.Close()

		producerClient := createProducerClient(t, logger, brokers)
		defer producerClient.Close()

		producer := NewProducer(producerClient)

		// Try to send a non-JSON serializable type
		type BadType struct {
			Channel chan int
		}

		err := producer.SendMessage(ctx, topic, BadType{Channel: make(chan int)})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "marshal error")
	})
}

func TestStartConsumer_Integration(t *testing.T) {
	t.Run("consumes messages and commits offsets", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		container, brokers := setupKafkaContainer(t)
		defer func() {
			if err := container.Terminate(ctx); err != nil {
				t.Logf("failed to terminate container: %v", err)
			}
		}()

		topic := "test-consumer"
		logger := setupTestLogger()

		// Create topic
		adminClient := createAdminClient(t, brokers)
		err := createTopic(ctx, adminClient, topic)
		adminClient.Close()
		require.NoError(t, err)

		// Send a message first
		producerClient := createProducerClient(t, logger, brokers)
		defer producerClient.Close()

		producer := NewProducer(producerClient)
		testMsg := TestMessage{ID: "001", Content: "Test content", Timestamp: time.Now()}
		err = producer.SendMessage(ctx, topic, testMsg)
		require.NoError(t, err)

		// Start consumer in background
		consumerClient := createTestClient(t, logger, brokers, []string{topic}, "test-consumer-group-3")

		receivedMessages := make([]*kgo.Record, 0)
		handler := func(r *kgo.Record) error {
			receivedMessages = append(receivedMessages, r)
			return nil
		}

		consumerCtx, consumerCancel := context.WithTimeout(ctx, 10*time.Second)
		defer consumerCancel()

		go func() {
			if err := StartConsumer(consumerCtx, consumerClient, handler); err != nil && err != context.Canceled {
				t.Logf("consumer error: %v", err)
			}
		}()

		// Wait for message to be consumed
		time.Sleep(5 * time.Second)
		consumerCancel()

		assert.GreaterOrEqual(t, len(receivedMessages), 1)
	})

	t.Run("handles handler errors gracefully", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		container, brokers := setupKafkaContainer(t)
		defer func() {
			if err := container.Terminate(ctx); err != nil {
				t.Logf("failed to terminate container: %v", err)
			}
		}()

		topic := "test-consumer-errors"
		logger := setupTestLogger()

		// Create topic
		adminClient := createAdminClient(t, brokers)
		err := createTopic(ctx, adminClient, topic)
		adminClient.Close()
		require.NoError(t, err)

		// Send a message
		producerClient := createProducerClient(t, logger, brokers)
		defer producerClient.Close()

		producer := NewProducer(producerClient)
		testMsg := TestMessage{ID: "001", Content: "Test", Timestamp: time.Now()}
		err = producer.SendMessage(ctx, topic, testMsg)
		require.NoError(t, err)

		// Consumer with failing handler
		consumerClient := createTestClient(t, logger, brokers, []string{topic}, "test-consumer-group-4")

		handler := func(r *kgo.Record) error {
			return fmt.Errorf("simulated handler error")
		}

		consumerCtx, consumerCancel := context.WithTimeout(ctx, 5*time.Second)
		defer consumerCancel()

		// Should not panic or crash
		err = StartConsumer(consumerCtx, consumerClient, handler)
		assert.Error(t, err) // Context timeout
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		container, brokers := setupKafkaContainer(t)
		defer func() {
			if err := container.Terminate(ctx); err != nil {
				t.Logf("failed to terminate container: %v", err)
			}
		}()

		topic := "test-consumer-cancel"
		logger := setupTestLogger()

		consumerClient := createTestClient(t, logger, brokers, []string{topic}, "test-consumer-group-5")

		handler := func(r *kgo.Record) error {
			return nil
		}

		// Cancel context immediately
		consumerCtx, consumerCancel := context.WithCancel(ctx)
		consumerCancel()

		err := StartConsumer(consumerCtx, consumerClient, handler)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestProducerConsumer_EndToEnd_Integration(t *testing.T) {
	t.Run("full producer-consumer workflow", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		container, brokers := setupKafkaContainer(t)
		defer func() {
			if err := container.Terminate(ctx); err != nil {
				t.Logf("failed to terminate container: %v", err)
			}
		}()

		topic := "test-e2e-workflow"
		logger := setupTestLogger()

		// Create topic first
		adminClient := createAdminClient(t, brokers)
		err := createTopic(ctx, adminClient, topic)
		adminClient.Close()
		require.NoError(t, err)

		// Track received messages
		receivedMsgs := make(map[string]TestMessage)
		msgChan := make(chan TestMessage, 10)

		// Start consumer first
		consumerClient := createTestClient(t, logger, brokers, []string{topic}, "test-consumer-group-e2e")

		go func() {
			consumerCtx, consumerCancel := context.WithTimeout(ctx, 30*time.Second)
			defer consumerCancel()

			handler := func(r *kgo.Record) error {
				var msg TestMessage
				if err := json.Unmarshal(r.Value, &msg); err != nil {
					return err
				}
				receivedMsgs[msg.ID] = msg
				msgChan <- msg
				return nil
			}

			if err := StartConsumer(consumerCtx, consumerClient, handler); err != nil && err != context.Canceled {
				t.Logf("consumer error: %v", err)
			}
		}()

		// Give consumer time to start
		time.Sleep(2 * time.Second)

		// Send messages via producer
		producerClient := createProducerClient(t, logger, brokers)
		defer producerClient.Close()

		producer := NewProducer(producerClient)

		sentMsgs := []TestMessage{
			{ID: "e2e-1", Content: "Message 1", Timestamp: time.Now()},
			{ID: "e2e-2", Content: "Message 2", Timestamp: time.Now()},
			{ID: "e2e-3", Content: "Message 3", Timestamp: time.Now()},
		}

		for _, msg := range sentMsgs {
			err := producer.SendMessage(ctx, topic, msg)
			require.NoError(t, err)
		}

		// Wait for all messages to be received
		for i := 0; i < len(sentMsgs); i++ {
			select {
			case <-msgChan:
				// Message received
			case <-time.After(15 * time.Second):
				t.Fatalf("timeout waiting for message %d", i+1)
			}
		}

		// Verify all messages were received
		assert.Equal(t, len(sentMsgs), len(receivedMsgs))
		for _, sent := range sentMsgs {
			received, exists := receivedMsgs[sent.ID]
			assert.True(t, exists, "message %s not received", sent.ID)
			assert.Equal(t, sent.Content, received.Content)
		}
	})
}
