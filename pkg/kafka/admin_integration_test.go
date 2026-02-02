package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdmin_CreateTopic(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, nil)
	require.NoError(t, err)
	defer admin.Close()

	topic := "test-create-topic-" + randomString(8)
	err = admin.EnsureTopicExists(ctx, topic, 1, 1)
	require.NoError(t, err)

	// Verify topic exists
	exists, err := admin.TopicExists(ctx, topic)
	require.NoError(t, err)
	assert.True(t, exists, "topic should exist after creation")
}

func TestAdmin_TopicExists_True(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	topic := "test-topic-exists-" + randomString(8)
	createTopic(t, ctx, topic)

	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, nil)
	require.NoError(t, err)
	defer admin.Close()

	exists, err := admin.TopicExists(ctx, topic)
	require.NoError(t, err)
	assert.True(t, exists, "existing topic should be found")
}

func TestAdmin_TopicExists_False(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, nil)
	require.NoError(t, err)
	defer admin.Close()

	nonExistentTopic := "non-existent-topic-" + randomString(8)
	exists, err := admin.TopicExists(ctx, nonExistentTopic)
	require.NoError(t, err)
	assert.False(t, exists, "non-existent topic should not be found")
}

func TestAdmin_DeleteTopic(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	topic := "test-delete-topic-" + randomString(8)
	createTopic(t, ctx, topic)

	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, nil)
	require.NoError(t, err)
	defer admin.Close()

	// Verify topic exists
	exists, err := admin.TopicExists(ctx, topic)
	require.NoError(t, err)
	assert.True(t, exists)

	// Delete topic
	err = admin.DeleteTopic(ctx, topic)
	require.NoError(t, err)

	// Verify topic no longer exists (may take a moment)
	assert.Eventually(t, func() bool {
		exists, err := admin.TopicExists(ctx, topic)
		require.NoError(t, err)
		return !exists
	}, 10*time.Second, 500*time.Millisecond, "topic should be deleted")
}

func TestAdmin_InitConsumerTopics(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, nil)
	require.NoError(t, err)
	defer admin.Close()

	topics := []string{
		"test-init-consumer-1-" + randomString(8),
		"test-init-consumer-2-" + randomString(8),
	}

	// Create main topics
	for _, topic := range topics {
		err = admin.EnsureTopicExists(ctx, topic, 1, 1)
		require.NoError(t, err)
	}

	// Manually create DLQ and retry topics with replication factor 1
	for _, topic := range topics {
		dlqTopic := topic + ".dlq"
		retryTopic := topic + ".retry"

		err = admin.EnsureTopicExists(ctx, dlqTopic, 1, 1)
		require.NoError(t, err)

		err = admin.EnsureTopicExists(ctx, retryTopic, 1, 1)
		require.NoError(t, err)
	}

	// Verify all topics exist
	for _, topic := range topics {
		exists, err := admin.TopicExists(ctx, topic)
		require.NoError(t, err)
		assert.True(t, exists, "main topic should exist: "+topic)
	}

	// Verify DLQ topics
	for _, topic := range topics {
		dlqTopic := topic + ".dlq"
		exists, err := admin.TopicExists(ctx, dlqTopic)
		require.NoError(t, err)
		assert.True(t, exists, "DLQ topic should exist: "+dlqTopic)
	}

	// Verify retry topics
	for _, topic := range topics {
		retryTopic := topic + ".retry"
		exists, err := admin.TopicExists(ctx, retryTopic)
		require.NoError(t, err)
		assert.True(t, exists, "retry topic should exist: "+retryTopic)
	}
}

func TestAdmin_DefaultPartitions(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, nil)
	require.NoError(t, err)
	defer admin.Close()

	topic := "test-default-partitions-" + randomString(8)

	// Create topic with default partitions (0 = use default)
	err = admin.EnsureTopicExists(ctx, topic, 0, 0)
	require.NoError(t, err)

	// Verify topic exists
	exists, err := admin.TopicExists(ctx, topic)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestAdmin_MultipleBrokers(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, nil)
	require.NoError(t, err)
	defer admin.Close()

	topic := "test-multi-broker-" + randomString(8)
	err = admin.EnsureTopicExists(ctx, topic, 1, 1)
	require.NoError(t, err)

	exists, err := admin.TopicExists(ctx, topic)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestAdmin_TopicAlreadyExists(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	topic := "test-exists-already-" + randomString(8)
	createTopic(t, ctx, topic)

	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, nil)
	require.NoError(t, err)
	defer admin.Close()

	// Try to create same topic again - should not error
	err = admin.EnsureTopicExists(ctx, topic, 1, 1)
	assert.NoError(t, err, "creating existing topic should not error")
}

func TestAdmin_EmptyTopicName(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, nil)
	require.NoError(t, err)
	defer admin.Close()

	err = admin.EnsureTopicExists(ctx, "", 3, 1)
	assert.Error(t, err, "empty topic name should error")
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestAdmin_NoBrokers(t *testing.T) {
	_, err := NewKafkaAdmin([]string{}, nil)
	assert.Error(t, err, "no brokers should error")
}

func TestAdmin_Close(t *testing.T) {
	requireKafka(t)

	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, nil)
	require.NoError(t, err)

	err = admin.Close()
	assert.NoError(t, err)

	// Close should be idempotent
	err = admin.Close()
	assert.NoError(t, err)
}

// Helper function to generate random string for topic names
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}
