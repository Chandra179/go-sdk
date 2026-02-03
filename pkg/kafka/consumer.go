package kafka

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"gosdk/pkg/logger"

	"github.com/twmb/franz-go/pkg/kgo"
)

type KafkaConsumer struct {
	kgoClient      *kgo.Client
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	groupID        string
	logger         logger.Logger
	kafkaClient    Client
	retryCfg       RetryConfig
	topics         []string
	metrics        *KafkaMetrics
	schemaRegistry *SchemaRegistry
}

func NewKafkaConsumer(cfg *ConsumerConfig, brokers []string, groupID string, topics []string, logger logger.Logger) (*KafkaConsumer, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.Dialer((&net.Dialer{Timeout: 10 * time.Second}).DialContext),
		kgo.ConsumerGroup(groupID),
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka consumer client: %w", err)
	}

	return &KafkaConsumer{
		kgoClient: client,
		groupID:   groupID,
		logger:    logger,
		topics:    topics,
	}, nil
}

func (c *KafkaConsumer) SetRetryConfig(kafkaClient Client, retryCfg RetryConfig) {
	c.kafkaClient = kafkaClient
	c.retryCfg = retryCfg
}

func (c *KafkaConsumer) SetMetrics(metrics *KafkaMetrics) {
	c.metrics = metrics
}

func (c *KafkaConsumer) SetSchemaRegistry(schemaRegistry *SchemaRegistry) {
	c.schemaRegistry = schemaRegistry
}

func (c *KafkaConsumer) logError(ctx context.Context, msg string, fields ...logger.Field) {
	c.logger.Error(ctx, msg, fields...)
}

func (c *KafkaConsumer) Start(ctx context.Context, handler ConsumerHandler) error {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				fetches := c.kgoClient.PollFetches(ctx)
				if fetches.Err() != nil {
					if ctx.Err() != nil {
						return
					}
					c.logError(ctx, "PollFetches error", logger.Field{Key: "error", Value: fetches.Err()})
					continue
				}

				records := fetches.Records()
				if len(records) == 0 {
					continue
				}

				for _, record := range records {
					headers := make(map[string]string, len(record.Headers))
					for _, h := range record.Headers {
						headers[h.Key] = string(h.Value)
					}

					message := fromRecord(record)

					if err := handler(message); err != nil {
						c.logError(ctx, "Handler error", logger.Field{Key: "error", Value: err})
					}

					c.kgoClient.CommitUncommittedOffsets(ctx)
				}
			}
		}
	}()

	return nil
}

func (c *KafkaConsumer) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	if c.kgoClient != nil {
		c.kgoClient.Close()
	}
	return nil
}
