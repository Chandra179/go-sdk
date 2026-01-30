package kafka

import (
	"context"
	"sync"
	"time"

	"gosdk/pkg/logger"

	kafkago "github.com/segmentio/kafka-go"
)

type KafkaConsumer struct {
	reader  *kafkago.Reader
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	groupID string
	logger  logger.Logger
}

func NewKafkaConsumer(cfg *ConsumerConfig, brokers []string, groupID string, topics []string, dialer *kafkago.Dialer, logger logger.Logger) *KafkaConsumer {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:               brokers,
		GroupID:               groupID,
		GroupTopics:           topics,
		MinBytes:              int(cfg.MinBytes),
		MaxBytes:              int(cfg.MaxBytes),
		CommitInterval:        cfg.CommitInterval,
		WatchPartitionChanges: cfg.WatchPartitionChanges,
		HeartbeatInterval:     cfg.HeartbeatInterval,
		// SessionTimeout:        cfg.SessionTimeout, // Let kafka-go use default
		Dialer:      dialer,
		StartOffset: kafkago.FirstOffset, // Read from beginning
	})

	return &KafkaConsumer{
		reader:  reader,
		groupID: groupID,
		logger:  logger,
	}
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
				msg, err := c.reader.FetchMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					c.logError(ctx, "FetchMessage error", logger.Field{Key: "error", Value: err})
					continue
				}

				// Record consumer lag with simple error handling
				if c.reader != nil {
					lag := c.reader.Lag()
					if lag >= 0 {
						// log
					}
				}

				headers := make(map[string]string)
				for _, h := range msg.Headers {
					headers[h.Key] = string(h.Value)
				}

				message := Message{
					Topic:   msg.Topic,
					Key:     msg.Key,
					Value:   msg.Value,
					Headers: headers,
				}

				if err := handler(message); err != nil {
					c.logError(ctx, "Error handling message",
						logger.Field{Key: "error", Value: err},
						logger.Field{Key: "topic", Value: msg.Topic},
						logger.Field{Key: "partition", Value: msg.Partition},
						logger.Field{Key: "offset", Value: msg.Offset})
					continue
				}

				if err := c.reader.CommitMessages(ctx, msg); err != nil {
					c.logError(ctx, "Error committing message",
						logger.Field{Key: "error", Value: err},
						logger.Field{Key: "offset", Value: msg.Offset})
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
	if c.reader != nil {
		return c.reader.Close()
	}
	return nil
}

func parseFirstFailedAt(headers map[string]string) time.Time {
	if headers == nil {
		return time.Time{}
	}
	if firstFailedAtStr, ok := headers[headerFirstFailedAt]; ok {
		if t, err := time.Parse(time.RFC3339Nano, firstFailedAtStr); err == nil {
			return t
		}
	}
	return time.Time{}
}
