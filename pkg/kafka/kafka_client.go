package kafka

import (
	"context"
	"sync"
)

type KafkaClient struct {
	brokers   []string
	producer  *KafkaProducer
	consumers map[string]*KafkaConsumer
	mu        sync.RWMutex
}

func NewClient(brokers []string) Client {
	return &KafkaClient{
		brokers:   brokers,
		consumers: make(map[string]*KafkaConsumer),
	}
}

func (c *KafkaClient) Producer() (Producer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.producer == nil {
		c.producer = NewKafkaProducer(c.brokers)
	}

	return c.producer, nil
}

func (c *KafkaClient) Consumer(groupID string) (Consumer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.consumers[groupID]; !exists {
		consumer := NewKafkaConsumer(c.brokers, groupID, "")
		c.consumers[groupID] = consumer
	}

	return c.consumers[groupID], nil
}

func (c *KafkaClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var err error

	if c.producer != nil {
		if closeErr := c.producer.Close(); closeErr != nil {
			err = closeErr
		}
	}

	for groupID, consumer := range c.consumers {
		if closeErr := consumer.Close(); closeErr != nil {
			err = closeErr
		}
		delete(c.consumers, groupID)
	}

	return err
}

func (c *KafkaClient) Ping(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, consumer := range c.consumers {
		if err := consumer.Ping(ctx); err != nil {
			return err
		}
	}

	return nil
}
