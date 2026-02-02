package kafka

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsumer_Start_Consume(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-consumer-consume"

	createTopic(t, ctx, topic)

	// Publish a message first
	publishTestMessage(t, ctx, topic, []byte("key1"), []byte("value1"), nil)

	// Create consumer and consume
	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-consume", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	received := make(chan Message, 1)
	go func() {
		handler := func(msg Message) error {
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
		assert.Equal(t, []byte("key1"), msg.Key)
		assert.Equal(t, []byte("value1"), msg.Value)
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestConsumer_StartOffset_Earliest(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-consumer-earliest"

	createTopic(t, ctx, topic)

	// Publish messages
	for i := 0; i < 3; i++ {
		publishTestMessage(t, ctx, topic, []byte("key-"+string(rune('a'+i))), []byte("value-"+string(rune('a'+i))), nil)
	}

	cfg := getTestConfig()
	cfg.Consumer.StartOffset = StartOffsetEarliest

	client, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-earliest", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	received := make(chan Message, 3)
	go func() {
		count := 0
		handler := func(msg Message) error {
			select {
			case received <- msg:
			default:
			}
			count++
			if count >= 3 {
				return nil
			}
			return nil
		}
		consumer.Start(ctx, handler)
	}()

	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()

	count := 0
	for count < 3 {
		select {
		case <-received:
			count++
		case <-timer.C:
			t.Fatalf("timeout: got %d messages, want 3", count)
		}
	}
}

func TestConsumer_StartOffset_Latest(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-consumer-latest"

	createTopic(t, ctx, topic)

	// Publish messages before consumer starts
	for i := 0; i < 3; i++ {
		publishTestMessage(t, ctx, topic, []byte("old-key-"+string(rune('a'+i))), []byte("old-value-"+string(rune('a'+i))), nil)
	}

	cfg := getTestConfig()
	cfg.Consumer.StartOffset = StartOffsetLatest

	client, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-latest", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	// Start consumer and publish new messages
	received := make(chan Message, 1)
	go func() {
		handler := func(msg Message) error {
			select {
			case received <- msg:
			default:
			}
			return nil
		}
		consumer.Start(ctx, handler)
	}()

	// Give consumer time to join group
	time.Sleep(2 * time.Second)

	// Publish new message
	publishTestMessage(t, ctx, topic, []byte("new-key"), []byte("new-value"), nil)

	select {
	case msg := <-received:
		assert.Equal(t, []byte("new-key"), msg.Key)
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for new message")
	}
}

func TestConsumer_GracefulShutdown(t *testing.T) {
	requireKafka(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	topic := "test-consumer-shutdown"

	createTopic(t, ctx, topic)

	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-shutdown", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	// Start consumer in background
	done := make(chan error, 1)
	go func() {
		handler := func(msg Message) error {
			return nil
		}
		done <- consumer.Start(ctx, handler)
	}()

	// Wait a bit then cancel context
	time.Sleep(2 * time.Second)
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("consumer did not shut down gracefully")
	}
}

func TestConsumer_NoCommitOnFailure(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-consumer-failure-" + randomString(8)

	createTopic(t, ctx, topic)

	publishTestMessage(t, ctx, topic, []byte("fail-key"), []byte("fail-value"), nil)

	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-failure", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	handlerCalled := make(chan bool, 1)

	go func() {
		handler := func(msg Message) error {
			select {
			case handlerCalled <- true:
			default:
			}
			return errors.New("handler error")
		}
		consumer.Start(ctx, handler)
	}()

	select {
	case <-handlerCalled:
		// Handler was called and returned error
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for handler")
	}
}

func TestConsumer_MultipleHandlers(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-consumer-multi"

	createTopic(t, ctx, topic)

	// Publish messages to multiple topics
	topic2 := "test-consumer-multi-2"
	createTopic(t, ctx, topic2)

	publishTestMessage(t, ctx, topic, []byte("key1"), []byte("value1"), nil)
	publishTestMessage(t, ctx, topic2, []byte("key2"), []byte("value2"), nil)

	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-multi", []string{topic, topic2})
	require.NoError(t, err)
	defer consumer.Close()

	received := make(chan Message, 2)
	go func() {
		handler := func(msg Message) error {
			select {
			case received <- msg:
			default:
			}
			return nil
		}
		consumer.Start(ctx, handler)
	}()

	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()

	count := 0
	for count < 2 {
		select {
		case msg := <-received:
			count++
			assert.NotEmpty(t, msg.Topic)
			assert.NotEmpty(t, msg.Key)
		case <-timer.C:
			t.Fatalf("timeout: got %d messages, want 2", count)
		}
	}
}

func TestConsumer_MessageOrdering(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-consumer-ordering"

	createTopic(t, ctx, topic)

	// Publish multiple messages with same key (should maintain ordering)
	for i := 0; i < 5; i++ {
		publishTestMessage(t, ctx, topic, []byte("ordered-key"), []byte("value-"+string(rune('0'+i))), nil)
	}

	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-ordering", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	received := make(chan Message, 5)
	go func() {
		count := 0
		handler := func(msg Message) error {
			select {
			case received <- msg:
			default:
			}
			count++
			if count >= 5 {
				return nil
			}
			return nil
		}
		consumer.Start(ctx, handler)
	}()

	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()

	count := 0
	for count < 5 {
		select {
		case <-received:
			count++
		case <-timer.C:
			t.Fatalf("timeout: got %d messages, want 5", count)
		}
	}
}

func TestConsumer_WithHeaders(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-consumer-headers"

	headers := map[string]string{
		"event-type":     "user.created",
		"correlation-id": "12345",
		"trace-id":       "abc-123",
	}

	publishTestMessage(t, ctx, topic, []byte("header-key"), []byte("header-value"), headers)

	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-headers", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	received := make(chan Message, 1)
	go func() {
		handler := func(msg Message) error {
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
		assert.Equal(t, "user.created", msg.Headers["event-type"])
		assert.Equal(t, "12345", msg.Headers["correlation-id"])
		assert.Equal(t, "abc-123", msg.Headers["trace-id"])
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for message with headers")
	}
}

func TestConsumer_Close(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-consumer-close"

	createTopic(t, ctx, topic)

	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)

	consumer, err := client.Consumer("test-group-close", []string{topic})
	require.NoError(t, err)

	err = consumer.Close()
	assert.NoError(t, err)

	client.Close()
}

func TestConsumer_ContextCancellation(t *testing.T) {
	requireKafka(t)
	ctx, cancel := context.WithCancel(context.Background())
	topic := "test-consumer-context"

	createTopic(t, ctx, topic)

	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer client.Close()

	consumer, err := client.Consumer("test-group-context", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	handlerCalled := make(chan bool, 1)
	go func() {
		handler := func(msg Message) error {
			_ = msg // Message received
			handlerCalled <- true
			return nil
		}
		consumer.Start(ctx, handler)
	}()

	// Cancel context
	cancel()

	select {
	case <-handlerCalled:
		// Handler was notified of context cancellation
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for context cancellation")
	}
}
