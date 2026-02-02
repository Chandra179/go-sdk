package kafka

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProducer_Publish_Success(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-producer-success"

	createTopic(t, ctx, topic)

	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer client.Close()

	producer, err := client.Producer()
	require.NoError(t, err)
	defer producer.Close()

	msg := Message{
		Topic: topic,
		Key:   []byte("test-key"),
		Value: []byte("test-value"),
	}

	err = producer.Publish(ctx, msg)
	assert.NoError(t, err)
}

func TestProducer_Publish_WithKey(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-producer-with-key"

	createTopic(t, ctx, topic)

	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer client.Close()

	producer, err := client.Producer()
	require.NoError(t, err)
	defer producer.Close()

	// Publish multiple messages with same key - should go to same partition
	for i := 0; i < 3; i++ {
		msg := Message{
			Topic: topic,
			Key:   []byte("same-key"),
			Value: []byte("value-" + string(rune('a'+i))),
		}
		err = producer.Publish(ctx, msg)
		assert.NoError(t, err)
	}
}

func TestProducer_Publish_WithHeaders(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-producer-with-headers-" + randomString(8)

	createTopic(t, ctx, topic)

	// Wait for topic to be fully created
	time.Sleep(2 * time.Second)

	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer client.Close()

	producer, err := client.Producer()
	require.NoError(t, err)
	defer producer.Close()

	headers := map[string]string{
		"event-type":     "user.created",
		"correlation-id": "12345",
		"trace-id":       "abc-123",
	}

	msg := Message{
		Topic:   topic,
		Key:     []byte("headers-test"),
		Value:   []byte("message with headers"),
		Headers: headers,
	}

	err = producer.Publish(ctx, msg)
	require.NoError(t, err)

	consumerClient, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer consumerClient.Close()

	consumer, err := consumerClient.Consumer("test-consumer-headers", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	received := make(chan Message, 1)
	consumerCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	go func() {
		handler := func(recvMsg Message) error {
			select {
			case received <- recvMsg:
			default:
			}
			return nil
		}
		consumer.Start(consumerCtx, handler)
	}()

	select {
	case recvMsg := <-received:
		assert.Equal(t, "user.created", recvMsg.Headers["event-type"])
		assert.Equal(t, "12345", recvMsg.Headers["correlation-id"])
		assert.Equal(t, "abc-123", recvMsg.Headers["trace-id"])
	case <-consumerCtx.Done():
		t.Fatal("timeout waiting for message with headers")
	}
}

func TestProducer_Publish_Compression(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	compressions := []string{"gzip", "snappy", "lz4", "zstd"}

	for _, compression := range compressions {
		t.Run(compression, func(t *testing.T) {
			topic := "test-compression-" + compression

			cfg := getTestConfig()
			cfg.Producer.CompressionType = compression
			client, err := NewClient(cfg, testLogger)
			require.NoError(t, err)
			defer client.Close()

			producer, err := client.Producer()
			require.NoError(t, err)
			defer producer.Close()

			createTopic(t, ctx, topic)

			msg := Message{
				Topic: topic,
				Key:   []byte("key"),
				Value: []byte("large payload for compression testing"),
			}

			err = producer.Publish(ctx, msg)
			assert.NoError(t, err)
		})
	}
}

func TestProducer_Publish_Acks(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-acks"

	createTopic(t, ctx, topic)

	ackLevels := []string{"all", "leader", "none"}

	for _, acks := range ackLevels {
		t.Run(acks, func(t *testing.T) {
			cfg := getTestConfig()
			cfg.Producer.RequiredAcks = acks
			client, err := NewClient(cfg, testLogger)
			require.NoError(t, err)
			defer client.Close()

			producer, err := client.Producer()
			require.NoError(t, err)
			defer producer.Close()

			msg := Message{
				Topic: topic,
				Key:   []byte("key-" + acks),
				Value: []byte("value-" + acks),
			}

			err = producer.Publish(ctx, msg)
			assert.NoError(t, err)
		})
	}
}

func TestProducer_Publish_Idempotency(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-idempotency"

	createTopic(t, ctx, topic)

	cfg := getTestConfig()
	cfg.Producer.RequiredAcks = "all" // Idempotency requires all acks
	cfg.Idempotency.Enabled = true
	cfg.Idempotency.WindowSize = 5 * time.Minute
	cfg.Idempotency.MaxCacheSize = 1000

	client, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer client.Close()

	producer, err := client.Producer()
	require.NoError(t, err)
	defer producer.Close()

	// Publish the same message twice
	msg := Message{
		Topic: topic,
		Key:   []byte("idempotent-key"),
		Value: []byte("same-value"),
	}

	err = producer.Publish(ctx, msg)
	require.NoError(t, err)

	err = producer.Publish(ctx, msg)
	assert.NoError(t, err) // Should succeed, not error
}

func TestProducer_Publish_IdempotencyDisabled(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-no-idempotency"

	createTopic(t, ctx, topic)

	cfg := getTestConfig()
	cfg.Producer.RequiredAcks = "all"
	cfg.Idempotency.Enabled = false

	client, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer client.Close()

	producer, err := client.Producer()
	require.NoError(t, err)
	defer producer.Close()

	msg := Message{
		Topic: topic,
		Key:   []byte("no-idempotent-key"),
		Value: []byte("value"),
	}

	err = producer.Publish(ctx, msg)
	require.NoError(t, err)

	err = producer.Publish(ctx, msg)
	require.NoError(t, err)
}

func TestProducer_AsyncPublish(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-async"

	createTopic(t, ctx, topic)

	cfg := getTestConfig()
	cfg.Producer.Async = true

	client, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer client.Close()

	producer, err := client.Producer()
	require.NoError(t, err)
	defer producer.Close()

	msg := Message{
		Topic: topic,
		Key:   []byte("async-key"),
		Value: []byte("async-value"),
	}

	err = producer.Publish(ctx, msg)
	assert.NoError(t, err)
}

func TestProducer_MessageTooLarge(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-large-message"

	createTopic(t, ctx, topic)

	cfg := getTestConfig()
	cfg.Producer.MaxMessageSize = 100 // 100 bytes max

	client, err := NewClient(cfg, testLogger)
	require.NoError(t, err)
	defer client.Close()

	producer, err := client.Producer()
	require.NoError(t, err)
	defer producer.Close()

	// Create a message larger than max size
	largeValue := make([]byte, 200)
	for i := range largeValue {
		largeValue[i] = 'x'
	}

	msg := Message{
		Topic: topic,
		Key:   []byte("large-key"),
		Value: largeValue,
	}

	err = producer.Publish(ctx, msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum size")
}

func TestProducer_PartitionStrategies(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	strategies := []PartitionStrategy{
		PartitionStrategyHash,
		PartitionStrategyRoundRobin,
		PartitionStrategyLeastBytes,
	}

	for _, strategy := range strategies {
		t.Run(string(strategy), func(t *testing.T) {
			topic := "test-strategy-" + string(strategy)

			cfg := getTestConfig()
			cfg.Producer.PartitionStrategy = strategy

			client, err := NewClient(cfg, testLogger)
			require.NoError(t, err)
			defer client.Close()

			producer, err := client.Producer()
			require.NoError(t, err)
			defer producer.Close()

			createTopic(t, ctx, topic)

			msg := Message{
				Topic: topic,
				Key:   []byte("key-" + string(strategy)),
				Value: []byte("value-" + string(strategy)),
			}

			err = producer.Publish(ctx, msg)
			assert.NoError(t, err)
		})
	}
}

func TestProducer_ConcurrentPublish(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-concurrent-produce"

	createTopic(t, ctx, topic)

	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer client.Close()

	producer, err := client.Producer()
	require.NoError(t, err)
	defer producer.Close()

	var wg sync.WaitGroup
	publishCount := 10

	for i := 0; i < publishCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			msg := Message{
				Topic: topic,
				Key:   []byte("key-" + string(rune('a'+idx))),
				Value: []byte("value-" + string(rune('a'+idx))),
			}

			_ = producer.Publish(ctx, msg)
		}(i)
	}

	wg.Wait()

	// Verify all messages were published by consuming them
	consumerClient, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)
	defer consumerClient.Close()

	consumer, err := consumerClient.Consumer("concurrent-consumer", []string{topic})
	require.NoError(t, err)
	defer consumer.Close()

	received := make(chan Message, publishCount)
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

	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()

	count := 0
	for count < publishCount {
		select {
		case <-received:
			count++
		case <-timer.C:
			t.Fatalf("timeout: got %d messages, want %d", count, publishCount)
		}
	}
}

func TestProducer_Close(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()
	topic := "test-producer-close"

	createTopic(t, ctx, topic)

	client, err := NewClient(getTestConfig(), testLogger)
	require.NoError(t, err)

	producer, err := client.Producer()
	require.NoError(t, err)

	err = producer.Close()
	assert.NoError(t, err)

	// Publish after close should fail
	msg := Message{
		Topic: topic,
		Key:   []byte("key"),
		Value: []byte("value"),
	}
	err = producer.Publish(ctx, msg)
	assert.Error(t, err)

	client.Close()
}
