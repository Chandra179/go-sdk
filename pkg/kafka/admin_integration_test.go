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

	cfg := DefaultAdminConfig()
	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, &cfg)
	require.NoError(t, err)
	defer admin.Close()

	topic := "test-create-topic-" + randomString(8)
	err = admin.CreateTopic(ctx, topic, 1, 1, nil)
	require.NoError(t, err)

	exists, err := admin.TopicExists(ctx, topic)
	require.NoError(t, err)
	assert.True(t, exists, "topic should exist after creation")
}

func TestAdmin_TopicExists_True(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	topic := "test-topic-exists-" + randomString(8)
	createTopic(t, ctx, topic)

	cfg := DefaultAdminConfig()
	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, &cfg)
	require.NoError(t, err)
	defer admin.Close()

	exists, err := admin.TopicExists(ctx, topic)
	require.NoError(t, err)
	assert.True(t, exists, "existing topic should be found")
}

func TestAdmin_TopicExists_False(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	cfg := DefaultAdminConfig()
	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, &cfg)
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

	cfg := DefaultAdminConfig()
	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, &cfg)
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

func TestAdmin_ValidateTopics(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	cfg := DefaultAdminConfig()
	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, &cfg)
	require.NoError(t, err)
	defer admin.Close()

	topics := []string{
		"test-validate-topics-1-" + randomString(8),
		"test-validate-topics-2-" + randomString(8),
	}

	// Create topics
	for _, topic := range topics {
		err = admin.CreateTopic(ctx, topic, 1, 1, nil)
		require.NoError(t, err)
	}

	// Validate all topics exist
	err = admin.ValidateTopicsExist(ctx, topics)
	require.NoError(t, err, "all topics should exist")

	// Test validation with non-existent topic
	invalidTopics := append(topics, "non-existent-topic-"+randomString(8))
	err = admin.ValidateTopicsExist(ctx, invalidTopics)
	assert.Error(t, err, "validation should fail with non-existent topic")
	assert.Contains(t, err.Error(), "topics not found")
}

func TestAdmin_ValidateTopicsWithRetry(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	cfg := DefaultAdminConfig()
	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, &cfg)
	require.NoError(t, err)
	defer admin.Close()

	topic := "test-validate-retry-" + randomString(8)

	// Validation should fail before topic is created
	err = admin.ValidateTopicsExist(ctx, []string{topic})
	assert.Error(t, err, "validation should fail before topic creation")

	// Create topic
	err = admin.CreateTopic(ctx, topic, 1, 1, nil)
	require.NoError(t, err)

	// Validation with retry should succeed
	err = admin.ValidateTopicsExistWithRetry(ctx, []string{topic})
	require.NoError(t, err, "validation with retry should succeed after topic creation")
}

func TestAdmin_DefaultPartitions(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	cfg := DefaultAdminConfig()
	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, &cfg)
	require.NoError(t, err)
	defer admin.Close()

	topic := "test-default-partitions-" + randomString(8)

	// Create topic with default partitions (0 = use default)
	err = admin.CreateTopic(ctx, topic, 0, 0, nil)
	require.NoError(t, err)

	// Verify topic exists
	exists, err := admin.TopicExists(ctx, topic)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestAdmin_MultipleBrokers(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	cfg := DefaultAdminConfig()
	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, &cfg)
	require.NoError(t, err)
	defer admin.Close()

	topic := "test-multi-broker-" + randomString(8)
	err = admin.CreateTopic(ctx, topic, 1, 1, nil)
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

	cfg := DefaultAdminConfig()
	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, &cfg)
	require.NoError(t, err)
	defer admin.Close()

	// Try to create same topic again - should not error (idempotent)
	err = admin.CreateTopic(ctx, topic, 1, 1, nil)
	assert.NoError(t, err, "creating existing topic should not error")
}

func TestAdmin_EmptyTopicName(t *testing.T) {
	requireKafka(t)
	ctx := context.Background()

	cfg := DefaultAdminConfig()
	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, &cfg)
	require.NoError(t, err)
	defer admin.Close()

	err = admin.CreateTopic(ctx, "", 3, 1, nil)
	assert.Error(t, err, "empty topic name should error")
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestAdmin_NoBrokers(t *testing.T) {
	cfg := DefaultAdminConfig()
	_, err := NewKafkaAdmin([]string{}, &cfg)
	assert.Error(t, err, "no brokers should error")
}

func TestAdmin_Close(t *testing.T) {
	requireKafka(t)

	cfg := DefaultAdminConfig()
	admin, err := NewKafkaAdmin([]string{testBrokerAddress}, &cfg)
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
