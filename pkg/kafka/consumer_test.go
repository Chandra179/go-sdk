package kafka

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockProducerForStartConsumer struct {
	mock.Mock
}

func (m *MockProducerForStartConsumer) Publish(ctx context.Context, msg Message) error {
	args := m.Called(msg)
	return args.Error(0)
}

func (m *MockProducerForStartConsumer) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockConsumerForStartConsumer struct {
	msg           Message
	handlerCalled bool
	handlerResult error
	commitCalled  bool
}

func (m *MockConsumerForStartConsumer) Subscribe(ctx context.Context, topics []string, handler ConsumerHandler) error {
	m.handlerCalled = true
	m.handlerResult = handler(m.msg)
	if m.handlerResult == nil {
		m.commitCalled = true
	}
	return m.handlerResult
}

func (m *MockConsumerForStartConsumer) Close() error {
	return nil
}

func (m *MockConsumerForStartConsumer) Ping(ctx context.Context) error {
	return nil
}

func TestStartConsumer_DLQCommit(t *testing.T) {
	t.Run("returns nil when message successfully sent to DLQ", func(t *testing.T) {
		mockClient := new(MockKafkaClient)
		mockProducer := new(MockProducerForStartConsumer)
		mockConsumer := &MockConsumerForStartConsumer{
			msg: Message{
				Topic:   "test-topic",
				Key:     []byte("key"),
				Value:   []byte("value"),
				Headers: map[string]string{headerRetryCount: "1"},
			},
		}

		mockClient.On("Producer").Return(mockProducer, nil)
		mockClient.On("Consumer", "test-group").Return(mockConsumer, nil)
		mockProducer.On("Publish", mock.MatchedBy(func(msg Message) bool {
			return msg.Topic == "test-topic-dlq"
		})).Return(nil)

		config := RetryConfig{
			ShortRetryAttempts:   1,
			MaxLongRetryAttempts: 0,
			DLQEnabled:           true,
			DLQTopicPrefix:       "-dlq",
		}

		handlerErr := assert.AnError
		err := StartConsumer(context.Background(), mockClient, "test-group", []string{"test-topic"}, func(msg Message) error {
			return handlerErr
		}, config)

		assert.NoError(t, err)
		assert.True(t, mockConsumer.handlerCalled)
		assert.True(t, mockConsumer.commitCalled, "offset should be committed when DLQ send succeeds")
		mockClient.AssertExpectations(t)
		mockProducer.AssertExpectations(t)
	})

	t.Run("returns error when DLQ send fails", func(t *testing.T) {
		mockClient := new(MockKafkaClient)
		mockProducer := new(MockProducerForStartConsumer)
		mockConsumer := &MockConsumerForStartConsumer{
			msg: Message{
				Topic:   "test-topic",
				Key:     []byte("key"),
				Value:   []byte("value"),
				Headers: map[string]string{headerRetryCount: "1"},
			},
		}

		mockClient.On("Producer").Return(mockProducer, nil)
		mockClient.On("Consumer", "test-group").Return(mockConsumer, nil)
		mockProducer.On("Publish", mock.MatchedBy(func(msg Message) bool {
			return true
		})).Return(assert.AnError)

		config := RetryConfig{
			ShortRetryAttempts:   1,
			MaxLongRetryAttempts: 0,
			DLQEnabled:           true,
			DLQTopicPrefix:       "-dlq",
		}

		err := StartConsumer(context.Background(), mockClient, "test-group", []string{"test-topic"}, func(msg Message) error {
			return assert.AnError
		}, config)

		assert.Error(t, err)
		assert.True(t, mockConsumer.handlerCalled)
		assert.False(t, mockConsumer.commitCalled, "offset should NOT be committed when DLQ send fails")
		mockClient.AssertExpectations(t)
		mockProducer.AssertExpectations(t)
	})
}

func TestStartConsumer_RetryTopicCommit(t *testing.T) {
	t.Run("returns nil when message successfully sent to retry topic", func(t *testing.T) {
		mockClient := new(MockKafkaClient)
		mockProducer := new(MockProducerForStartConsumer)
		mockConsumer := &MockConsumerForStartConsumer{
			msg: Message{
				Topic:   "test-topic",
				Key:     []byte("key"),
				Value:   []byte("value"),
				Headers: make(map[string]string),
			},
		}

		mockClient.On("Producer").Return(mockProducer, nil)
		mockClient.On("Consumer", "test-group").Return(mockConsumer, nil)
		mockProducer.On("Publish", mock.MatchedBy(func(msg Message) bool {
			return msg.Topic == "test-topic.retry"
		})).Return(nil)

		config := RetryConfig{
			ShortRetryAttempts:   1,
			MaxLongRetryAttempts: 10,
			DLQEnabled:           false,
			RetryTopicSuffix:     ".retry",
		}

		err := StartConsumer(context.Background(), mockClient, "test-group", []string{"test-topic"}, func(msg Message) error {
			return assert.AnError
		}, config)

		assert.NoError(t, err)
		assert.True(t, mockConsumer.handlerCalled)
		assert.True(t, mockConsumer.commitCalled, "offset should be committed when retry topic send succeeds")
		mockClient.AssertExpectations(t)
		mockProducer.AssertExpectations(t)
	})

	t.Run("returns error when retry topic send fails", func(t *testing.T) {
		mockClient := new(MockKafkaClient)
		mockProducer := new(MockProducerForStartConsumer)
		mockConsumer := &MockConsumerForStartConsumer{
			msg: Message{
				Topic:   "test-topic",
				Key:     []byte("key"),
				Value:   []byte("value"),
				Headers: make(map[string]string),
			},
		}

		mockClient.On("Producer").Return(mockProducer, nil)
		mockClient.On("Consumer", "test-group").Return(mockConsumer, nil)
		mockProducer.On("Publish", mock.MatchedBy(func(msg Message) bool {
			return true
		})).Return(assert.AnError)

		config := RetryConfig{
			ShortRetryAttempts:   1,
			MaxLongRetryAttempts: 10,
			DLQEnabled:           false,
			RetryTopicSuffix:     ".retry",
		}

		err := StartConsumer(context.Background(), mockClient, "test-group", []string{"test-topic"}, func(msg Message) error {
			return assert.AnError
		}, config)

		assert.Error(t, err)
		assert.True(t, mockConsumer.handlerCalled)
		assert.False(t, mockConsumer.commitCalled, "offset should NOT be committed when retry topic send fails")
		mockClient.AssertExpectations(t)
		mockProducer.AssertExpectations(t)
	})
}

func TestStartConsumer_HandlerSuccess(t *testing.T) {
	t.Run("returns nil when handler succeeds initially", func(t *testing.T) {
		mockClient := new(MockKafkaClient)
		mockConsumer := &MockConsumerForStartConsumer{
			msg: Message{
				Topic:   "test-topic",
				Key:     []byte("key"),
				Value:   []byte("value"),
				Headers: make(map[string]string),
			},
		}

		mockClient.On("Consumer", "test-group").Return(mockConsumer, nil)

		config := RetryConfig{
			ShortRetryAttempts: 1,
		}

		err := StartConsumer(context.Background(), mockClient, "test-group", []string{"test-topic"}, func(msg Message) error {
			return nil
		}, config)

		assert.NoError(t, err)
		assert.True(t, mockConsumer.handlerCalled)
		assert.True(t, mockConsumer.commitCalled, "offset should be committed when handler succeeds")
		mockClient.AssertExpectations(t)
	})
}
