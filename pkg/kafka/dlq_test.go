package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockProducer struct {
	mock.Mock
}

func (m *MockProducer) Publish(ctx context.Context, msg Message) error {
	args := m.Called(ctx, msg)
	return args.Error(0)
}

func (m *MockProducer) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockKafkaClient struct {
	mock.Mock
}

func (m *MockKafkaClient) Producer() (Producer, error) {
	args := m.Called()
	producer, _ := args.Get(0).(Producer)
	return producer, args.Error(1)
}

func (m *MockKafkaClient) Consumer(groupID string) (Consumer, error) {
	args := m.Called(groupID)
	return args.Get(0).(Consumer), args.Error(1)
}

func (m *MockKafkaClient) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockKafkaClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestSendToDLQ(t *testing.T) {
	t.Run("success with event headers", func(t *testing.T) {
		mockKafkaClient := new(MockKafkaClient)
		mockProducer := new(MockProducer)

		mockKafkaClient.On("Producer").Return(mockProducer, nil)
		mockProducer.On("Publish", mock.Anything, mock.MatchedBy(func(msg Message) bool {
			assert.Equal(t, "user.created-dlq", msg.Topic)
			assert.Equal(t, assert.AnError.Error(), msg.Headers[headerErrorMessage])
			assert.Equal(t, "user.created", msg.Headers[headerOriginalTopic])
			assert.NotEmpty(t, msg.Headers[headerLastFailedAt])
			assert.NotEmpty(t, msg.Headers[headerFirstFailedAt])
			return true
		})).Return(nil)

		originalMsg := Message{
			Topic: "user.created",
			Key:   []byte("user-123"),
			Value: []byte(`{"user_id":"123"}`),
			Headers: map[string]string{
				"event_type": "UserCreatedEvent",
				"version":    "1.0",
				"event_id":   "evt-123",
				"offset":     "42",
			},
		}

		err := SendToDLQ(context.Background(), mockKafkaClient, "user.created-dlq", originalMsg, assert.AnError)

		assert.NoError(t, err)
		mockKafkaClient.AssertExpectations(t)
		mockProducer.AssertExpectations(t)
	})

	t.Run("success without event headers", func(t *testing.T) {
		mockKafkaClient := new(MockKafkaClient)
		mockProducer := new(MockProducer)

		mockKafkaClient.On("Producer").Return(mockProducer, nil)
		mockProducer.On("Publish", mock.Anything, mock.Anything).Return(nil)

		originalMsg := Message{
			Topic:   "test-topic",
			Key:     []byte("key-123"),
			Value:   []byte(`{"data":"value"}`),
			Headers: nil,
		}

		err := SendToDLQ(context.Background(), mockKafkaClient, "test-topic-dlq", originalMsg, assert.AnError)

		assert.NoError(t, err)
		mockKafkaClient.AssertExpectations(t)
		mockProducer.AssertExpectations(t)
	})

	t.Run("preserves x-first-failed-at from original message", func(t *testing.T) {
		mockKafkaClient := new(MockKafkaClient)
		mockProducer := new(MockProducer)

		firstFailedAt := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)

		mockKafkaClient.On("Producer").Return(mockProducer, nil)
		mockProducer.On("Publish", mock.Anything, mock.MatchedBy(func(msg Message) bool {
			assert.Equal(t, firstFailedAt, msg.Headers[headerFirstFailedAt])
			return true
		})).Return(nil)

		originalMsg := Message{
			Topic: "test-topic",
			Key:   []byte("key-123"),
			Value: []byte(`{"data":"value"}`),
			Headers: map[string]string{
				headerFirstFailedAt: firstFailedAt,
				headerRetryCount:    "2",
			},
		}

		err := SendToDLQ(context.Background(), mockKafkaClient, "test-topic-dlq", originalMsg, assert.AnError)

		assert.NoError(t, err)
		mockKafkaClient.AssertExpectations(t)
		mockProducer.AssertExpectations(t)
	})
}

func TestSendToRetryTopic(t *testing.T) {
	t.Run("success with all headers", func(t *testing.T) {
		mockKafkaClient := new(MockKafkaClient)
		mockProducer := new(MockProducer)

		firstFailedAt := time.Now()

		mockKafkaClient.On("Producer").Return(mockProducer, nil)
		mockProducer.On("Publish", mock.Anything, mock.MatchedBy(func(msg Message) bool {
			assert.Equal(t, "user.created.retry", msg.Topic)
			assert.Equal(t, assert.AnError.Error(), msg.Headers[headerErrorMessage])
			assert.Equal(t, "user.created", msg.Headers[headerOriginalTopic])
			assert.Equal(t, "1", msg.Headers[headerRetryCount])
			assert.NotEmpty(t, msg.Headers[headerLastFailedAt])
			assert.Equal(t, firstFailedAt.UTC().Format(time.RFC3339Nano), msg.Headers[headerFirstFailedAt])
			assert.Equal(t, "UserCreatedEvent", msg.Headers["event_type"])
			return true
		})).Return(nil)

		originalMsg := Message{
			Topic: "user.created",
			Key:   []byte("user-123"),
			Value: []byte(`{"user_id":"123"}`),
			Headers: map[string]string{
				"event_type": "UserCreatedEvent",
				"version":    "1.0",
				"event_id":   "evt-123",
				"offset":     "42",
			},
		}

		err := SendToRetryTopic(context.Background(), mockKafkaClient, "user.created.retry", originalMsg, assert.AnError, 1, firstFailedAt)

		assert.NoError(t, err)
		mockKafkaClient.AssertExpectations(t)
		mockProducer.AssertExpectations(t)
	})

	t.Run("success without original headers", func(t *testing.T) {
		mockKafkaClient := new(MockKafkaClient)
		mockProducer := new(MockProducer)

		mockKafkaClient.On("Producer").Return(mockProducer, nil)
		mockProducer.On("Publish", mock.Anything, mock.Anything).Return(nil)

		originalMsg := Message{
			Topic:   "test-topic",
			Key:     []byte("key-123"),
			Value:   []byte(`{"data":"value"}`),
			Headers: nil,
		}

		err := SendToRetryTopic(context.Background(), mockKafkaClient, "test-topic.retry", originalMsg, assert.AnError, 1, time.Now())

		assert.NoError(t, err)
		mockKafkaClient.AssertExpectations(t)
		mockProducer.AssertExpectations(t)
	})
}

func TestExtractRetryCount(t *testing.T) {
	t.Run("extracts retry count from headers", func(t *testing.T) {
		headers := map[string]string{
			headerRetryCount: "3",
			"other":          "value",
		}
		count := extractRetryCount(headers)
		assert.Equal(t, 3, count)
	})

	t.Run("returns 0 when header is missing", func(t *testing.T) {
		headers := map[string]string{
			"other": "value",
		}
		count := extractRetryCount(headers)
		assert.Equal(t, 0, count)
	})

	t.Run("returns 0 when headers is nil", func(t *testing.T) {
		count := extractRetryCount(nil)
		assert.Equal(t, 0, count)
	})

	t.Run("returns 0 when header is not a number", func(t *testing.T) {
		headers := map[string]string{
			headerRetryCount: "invalid",
		}
		count := extractRetryCount(headers)
		assert.Equal(t, 0, count)
	})
}

func TestShouldSendToDLQ(t *testing.T) {
	t.Run("returns true when retry count >= max", func(t *testing.T) {
		result := shouldSendToDLQ(3, 3)
		assert.True(t, result)

		result = shouldSendToDLQ(5, 3)
		assert.True(t, result)
	})

	t.Run("returns false when retry count < max", func(t *testing.T) {
		result := shouldSendToDLQ(2, 3)
		assert.False(t, result)

		result = shouldSendToDLQ(0, 3)
		assert.False(t, result)
	})
}

func TestCopyHeaders(t *testing.T) {
	t.Run("copies all headers", func(t *testing.T) {
		headers := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}
		copied := copyHeaders(headers)
		assert.EqualValues(t, headers, copied)
		copied["key3"] = "value3"
		_, exists := headers["key3"]
		assert.False(t, exists)
	})

	t.Run("returns nil when input is nil", func(t *testing.T) {
		copied := copyHeaders(nil)
		assert.Nil(t, copied)
	})

	t.Run("modifying copy does not affect original", func(t *testing.T) {
		headers := map[string]string{
			"key1": "value1",
		}
		copied := copyHeaders(headers)
		copied["key2"] = "value2"
		assert.NotContains(t, headers, "key2")
	})
}
