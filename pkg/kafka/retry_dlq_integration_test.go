package kafka

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetry_ShortRetries(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-short-retry-" + randomString(8)

	cfg := getTestConfig()
	cfg.Retry.ShortRetryAttempts = 3
	cfg.Retry.InitialBackoff = 100
	cfg.Retry.MaxBackoff = 500

	createTopic(t, ctx, topic)

	publishTestMessage(t, ctx, topic, []byte("retry-key"), []byte("retry-value"), nil)

	client, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-short-retry", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	handlerAttempts := 0
	received := make(chan Message, 1)

	go func() {
		handler := func(msg Message) error {
			handlerAttempts++
			if handlerAttempts < 3 {
				return errors.New("transient error")
			}
			select {
			case received <- msg:
			default:
			}
			return nil
		}
		consumer.Start(ctx, handler)
	}()

	select {
	case msg := <-received:
		assert.Equal(t, []byte("retry-key"), msg.Key)
		assert.Equal(t, 3, handlerAttempts)
	case <-time.After(30 * time.Second):
		t.Fatalf("timeout waiting for message after retries: attempts=%d", handlerAttempts)
	}
}

func TestRetry_LongRetries(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-long-retry"

	cfg := getTestConfig()
	cfg.Retry.ShortRetryAttempts = 1
	cfg.Retry.MaxLongRetryAttempts = 2
	cfg.Retry.InitialBackoff = 100
	cfg.Retry.MaxBackoff = 500
	cfg.Retry.DLQEnabled = false
	cfg.Retry.RetryTopicSuffix = ".retry"

	retryTopic := topic + cfg.Retry.RetryTopicSuffix

	createTopic(t, ctx, topic)
	createTopic(t, ctx, retryTopic)

	// Publish a message
	publishTestMessage(t, ctx, topic, []byte("long-retry-key"), []byte("long-retry-value"), nil)

	client, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-long-retry", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	handlerAttempts := 0
	received := make(chan Message, 1)
	lastRetryCount := 0

	go func() {
		handler := func(msg Message) error {
			handlerAttempts++
			if retryCount, ok := msg.Headers["x-retry-count"]; ok {
				lastRetryCount = parseRetryCountValue(retryCount)
			}
			if handlerAttempts < 5 {
				return errors.New("persistent error")
			}
			select {
			case received <- msg:
			default:
			}
			return nil
		}
		consumer.Start(ctx, handler)
	}()

	select {
	case msg := <-received:
		assert.Equal(t, []byte("long-retry-key"), msg.Key)
		// Message should have been retried
		assert.GreaterOrEqual(t, lastRetryCount, 1)
	case <-time.After(60 * time.Second):
		t.Fatalf("timeout waiting for message: attempts=%d, last-retry-count=%d", handlerAttempts, lastRetryCount)
	}
}

func TestDLQ_Routing(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-dlq-routing-" + randomString(8)
	dlqTopic := topic + ".dlq"

	cfg := getTestConfig()
	cfg.Retry.ShortRetryAttempts = 1
	cfg.Retry.MaxLongRetryAttempts = 2
	cfg.Retry.DLQEnabled = true
	cfg.Retry.DLQTopicPrefix = ""

	createTopic(t, ctx, topic)
	createTopic(t, ctx, dlqTopic)

	publishTestMessage(t, ctx, topic, []byte("dlq-key"), []byte("dlq-value"), nil)

	client, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-dlq", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	handlerAttempts := 0
	go func() {
		handler := func(msg Message) error {
			handlerAttempts++
			if handlerAttempts < 5 {
				return errors.New("always failing")
			}
			return nil
		}
		consumer.Start(ctx, handler)
	}()

	// Wait for message to be sent to DLQ
	time.Sleep(10 * time.Second)

	dlqClient, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer dlqClient.Close()

	dlqConsumer, err := dlqClient.Consumer("test-group-dlq-consumer", []string{dlqTopic})
	require.NoError(t, err)
	defer dlqConsumer.Close()

	dlqReceived := make(chan Message, 1)
	go func() {
		handler := func(msg Message) error {
			select {
			case dlqReceived <- msg:
			default:
			}
			return nil
		}
		dlqConsumer.Start(ctx, handler)
	}()

	select {
	case msg := <-dlqReceived:
		assert.Equal(t, []byte("dlq-key"), msg.Key)
		assert.Equal(t, []byte("dlq-value"), msg.Value)
		assert.Equal(t, topic, msg.Headers["original_topic"])
		assert.Contains(t, msg.Headers["x-error-message"], "always failing")
	case <-time.After(30 * time.Second):
		t.Fatalf("timeout waiting for DLQ message: attempts=%d", handlerAttempts)
	}
}

func TestDLQ_HeadersPreserved(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-dlq-headers"
	dlqTopic := topic + ".dlq"

	cfg := getTestConfig()
	cfg.Retry.DLQEnabled = true
	cfg.Retry.MaxLongRetryAttempts = 1
	cfg.Retry.ShortRetryAttempts = 1

	createTopic(t, ctx, topic)
	createTopic(t, ctx, dlqTopic)

	originalHeaders := map[string]string{
		"event-type":     "user.created",
		"correlation-id": "12345",
		"custom-header":  "custom-value",
	}

	publishTestMessage(t, ctx, topic, []byte("headers-key"), []byte("headers-value"), originalHeaders)

	client, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-dlq-headers", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	go func() {
		handler := func(msg Message) error {
			return errors.New("fail to trigger DLQ")
		}
		consumer.Start(ctx, handler)
	}()

	// Wait for DLQ routing
	time.Sleep(5 * time.Second)

	// Consume from DLQ
	dlqClient, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer dlqClient.Close()

	dlqConsumer, err := dlqClient.Consumer("test-group-dlq-headers-consumer", []string{dlqTopic})
	require.NoError(t, err)
	defer dlqConsumer.Close()

	dlqReceived := make(chan Message, 1)
	go func() {
		handler := func(msg Message) error {
			select {
			case dlqReceived <- msg:
			default:
			}
			return nil
		}
		dlqConsumer.Start(ctx, handler)
	}()

	select {
	case msg := <-dlqReceived:
		// Original headers should be preserved
		assert.Equal(t, "user.created", msg.Headers["event-type"])
		assert.Equal(t, "12345", msg.Headers["correlation-id"])
		assert.Equal(t, "custom-value", msg.Headers["custom-header"])
		// Original topic should be added
		assert.Equal(t, topic, msg.Headers["original_topic"])
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for DLQ message with headers")
	}
}

func TestDLQ_ErrorContext(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-dlq-error"
	dlqTopic := topic + ".dlq"

	cfg := getTestConfig()
	cfg.Retry.DLQEnabled = true
	cfg.Retry.MaxLongRetryAttempts = 1
	cfg.Retry.ShortRetryAttempts = 1

	createTopic(t, ctx, topic)
	createTopic(t, ctx, dlqTopic)

	expectedError := "database connection failed: timeout after 30s"
	publishTestMessage(t, ctx, topic, []byte("error-key"), []byte("error-value"), nil)

	client, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-dlq-error", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	go func() {
		handler := func(msg Message) error {
			return errors.New(expectedError)
		}
		consumer.Start(ctx, handler)
	}()

	time.Sleep(5 * time.Second)

	dlqClient, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer dlqClient.Close()

	dlqConsumer, err := dlqClient.Consumer("test-group-dlq-error-consumer", []string{dlqTopic})
	require.NoError(t, err)
	defer dlqConsumer.Close()

	dlqReceived := make(chan Message, 1)
	go func() {
		handler := func(msg Message) error {
			select {
			case dlqReceived <- msg:
			default:
			}
			return nil
		}
		dlqConsumer.Start(ctx, handler)
	}()

	select {
	case msg := <-dlqReceived:
		assert.Equal(t, expectedError, msg.Headers["x-error-message"])
		assert.NotEmpty(t, msg.Headers["x-last-failed-at"])
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for DLQ error context")
	}
}

func TestRetry_DLQ_TopicCreation(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-auto-topics"

	cfg := getTestConfig()
	cfg.Retry.DLQEnabled = true
	cfg.Retry.MaxLongRetryAttempts = 2
	cfg.Retry.ShortRetryAttempts = 1
	cfg.Retry.DLQTopicPrefix = ""
	cfg.Retry.RetryTopicSuffix = ".retry"

	dlqTopic := topic + ".dlq"
	retryTopic := topic + ".retry"

	createTopic(t, ctx, topic)

	client, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer client.Close()

	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, nil)
	require.NoError(t, err)
	defer admin.Close()

	// Initialize topics for consumer
	err = admin.InitializeTopicsForConsumer(ctx, []string{topic}, cfg.Retry)
	require.NoError(t, err)

	// Verify topics exist
	dlqExists, err := admin.TopicExists(ctx, dlqTopic)
	require.NoError(t, err)
	assert.True(t, dlqExists, "DLQ topic should be created")

	retryExists, err := admin.TopicExists(ctx, retryTopic)
	require.NoError(t, err)
	assert.True(t, retryExists, "Retry topic should be created")
}

func TestRetry_DisabledDLQ(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-no-dlq"

	cfg := getTestConfig()
	cfg.Retry.DLQEnabled = false
	cfg.Retry.MaxLongRetryAttempts = 1
	cfg.Retry.ShortRetryAttempts = 1

	createTopic(t, ctx, topic)

	publishTestMessage(t, ctx, topic, []byte("no-dlq-key"), []byte("no-dlq-value"), nil)

	client, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-no-dlq", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	handlerCalled := make(chan bool, 1)
	go func() {
		handler := func(msg Message) error {
			handlerCalled <- true
			return errors.New("error with no DLQ")
		}
		consumer.Start(ctx, handler)
	}()

	select {
	case <-handlerCalled:
		// Handler was called but no DLQ, so error is returned
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for handler")
	}
}

func TestRetry_CustomTopicSuffix(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-custom-suffix"
	customRetryTopic := topic + "_custom_retry"
	customDLQTopic := "dlq_" + topic

	cfg := getTestConfig()
	cfg.Retry.DLQEnabled = true
	cfg.Retry.DLQTopicPrefix = "dlq_"
	cfg.Retry.RetryTopicSuffix = "_custom_retry"
	cfg.Retry.MaxLongRetryAttempts = 1
	cfg.Retry.ShortRetryAttempts = 1

	createTopic(t, ctx, topic)
	createTopic(t, ctx, customRetryTopic)
	createTopic(t, ctx, customDLQTopic)

	publishTestMessage(t, ctx, topic, []byte("custom-suffix-key"), []byte("custom-suffix-value"), nil)

	client, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-custom-suffix", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	go func() {
		handler := func(msg Message) error {
			return errors.New("trigger DLQ with custom suffix")
		}
		consumer.Start(ctx, handler)
	}()

	time.Sleep(5 * time.Second)

	// Verify DLQ topic name
	dlqClient, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer dlqClient.Close()

	dlqConsumer, err := dlqClient.Consumer("test-group-custom-suffix-dlq", []string{customDLQTopic})
	require.NoError(t, err)
	defer dlqConsumer.Close()

	received := make(chan Message, 1)
	go func() {
		handler := func(msg Message) error {
			select {
			case received <- msg:
			default:
			}
			return nil
		}
		dlqConsumer.Start(ctx, handler)
	}()

	select {
	case msg := <-received:
		assert.Equal(t, customDLQTopic, msg.Topic)
		assert.Equal(t, []byte("custom-suffix-key"), msg.Key)
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for message in custom DLQ topic")
	}
}

// Helper function
func parseRetryCountValue(s string) int {
	if s == "" {
		return 0
	}
	for i, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		if i > 5 {
			return 0
		}
	}
	var result int
	for _, c := range s {
		result = result*10 + int(c-'0')
	}
	return result
}
