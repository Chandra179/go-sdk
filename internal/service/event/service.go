package event

import (
	"context"
	"fmt"
	"sync"

	"gosdk/pkg/kafka"

	"github.com/google/uuid"
)

type Service struct {
	kafkaClient kafka.Client
	consumers   map[string]*kafka.KafkaConsumer
	mu          sync.RWMutex
}

func NewService(kafkaClient kafka.Client) *Service {
	return &Service{
		kafkaClient: kafkaClient,
		consumers:   make(map[string]*kafka.KafkaConsumer),
	}
}

func (s *Service) PublishMessage(ctx context.Context, topic, key, value string, headers map[string]string) error {
	producer, err := s.kafkaClient.Producer()
	if err != nil {
		return fmt.Errorf("failed to get producer: %w", kafka.ErrKafkaConnection)
	}

	message := kafka.Message{
		Topic:   topic,
		Key:     []byte(key),
		Value:   []byte(value),
		Headers: headers,
	}

	return producer.Publish(ctx, message)
}

func (s *Service) SubscribeToTopic(ctx context.Context, topic, groupID string) (string, error) {
	subscriptionID := uuid.NewString()

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.consumers[subscriptionID]; !exists {
		consumer, err := s.kafkaClient.Consumer(groupID)
		if err != nil {
			return "", fmt.Errorf("failed to get consumer: %w", kafka.ErrKafkaConnection)
		}

		s.consumers[subscriptionID] = consumer.(*kafka.KafkaConsumer)
	}

	handler := func(msg kafka.Message) error {
		fmt.Printf("Received message: topic=%s, key=%s, value=%s\n", msg.Topic, msg.Key, msg.Value)
		if msg.Headers != nil {
			fmt.Printf("Headers: %v\n", msg.Headers)
		}
		return nil
	}

	consumer := s.consumers[subscriptionID]
	if err := consumer.Subscribe(ctx, []string{topic}, handler); err != nil {
		return "", fmt.Errorf("failed to subscribe: %w", kafka.ErrKafkaPublish)
	}

	return subscriptionID, nil
}

func (s *Service) Unsubscribe(subscriptionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if consumer, exists := s.consumers[subscriptionID]; exists {
		if err := consumer.Close(); err != nil {
			return fmt.Errorf("failed to close consumer: %w", kafka.ErrKafkaConnection)
		}
		delete(s.consumers, subscriptionID)
	}

	return nil
}
