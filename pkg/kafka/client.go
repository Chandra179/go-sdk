package kafka

import (
	"context"
	"fmt"
	"sync"

	kafkago "github.com/segmentio/kafka-go"
)

type KafkaClient struct {
	config    *Config
	brokers   []string
	dialer    *kafkago.Dialer
	producer  *KafkaProducer
	consumers map[string]*KafkaConsumer
	mu        sync.RWMutex
}

func NewClient(cfg *Config) (Client, error) {
	dialer, err := CreateDialer(&cfg.Security)
	if err != nil {
		return nil, fmt.Errorf("failed to create dialer: %w", err)
	}

	return &KafkaClient{
		config:    cfg,
		brokers:   cfg.Brokers,
		dialer:    dialer,
		consumers: make(map[string]*KafkaConsumer),
	}, nil
}

func (c *KafkaClient) Producer() (Producer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.producer == nil {
		c.producer = NewKafkaProducer(&c.config.Producer, c.brokers, c.dialer)
	}

	return c.producer, nil
}

func (c *KafkaClient) Consumer(groupID string) (Consumer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.consumers[groupID]; !exists {
		consumer := NewKafkaConsumer(&c.config.Consumer, c.brokers, groupID, c.dialer)
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
	c.mu.RLock()
	defer c.mu.RUnlock()

	conn, err := c.dialer.DialContext(ctx, "tcp", c.brokers[0])
	if err != nil {
		return fmt.Errorf("%w: %v", ErrKafkaConnection, err)
	}
	defer conn.Close()

	return nil
}
