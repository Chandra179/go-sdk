package kafka

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type KafkaConsumer struct {
	reader  *kafkago.Reader
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	groupID string
}

func NewKafkaConsumer(cfg *ConsumerConfig, brokers []string, groupID string, dialer *kafkago.Dialer) *KafkaConsumer {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:               brokers,
		GroupID:               groupID,
		MinBytes:              int(cfg.MinBytes),
		MaxBytes:              int(cfg.MaxBytes),
		CommitInterval:        cfg.CommitInterval,
		WatchPartitionChanges: cfg.WatchPartitionChanges,
		HeartbeatInterval:     cfg.HeartbeatInterval,
		SessionTimeout:        cfg.SessionTimeout,
		Dialer:                dialer,
		Logger:                kafkago.LoggerFunc(logf),
		ErrorLogger:           kafkago.LoggerFunc(logError),
	})

	return &KafkaConsumer{
		reader:  reader,
		groupID: groupID,
	}
}

func logf(msg string, a ...interface{}) {
	log.Printf(msg, a...)
}

func logError(msg string, a ...interface{}) {
	log.Printf(msg, a...)
}

func (c *KafkaConsumer) Subscribe(ctx context.Context, topics []string, handler ConsumerHandler) error {
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
					continue
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
					logError("Error handling message: %v, topic: %s, partition: %d, offset: %d\n",
						err, msg.Topic, msg.Partition, msg.Offset)
					continue
				}

				if err := c.reader.CommitMessages(ctx, msg); err != nil {
					logError("Error committing message: %v, offset: %d\n", err, msg.Offset)
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

func StartConsumer(
	ctx context.Context,
	client Client,
	groupID string,
	topics []string,
	handler ConsumerHandler,
	config RetryConfig,
) error {
	consumer, err := client.Consumer(groupID)
	if err != nil {
		return fmt.Errorf("failed to create consumer: %w", err)
	}

	wrappedHandler := func(msg Message) error {
		retryCount := extractRetryCount(msg.Headers)
		firstFailedAt := parseFirstFailedAt(msg.Headers)

		shortRetryAttempts := config.ShortRetryAttempts
		if shortRetryAttempts <= 0 {
			shortRetryAttempts = defaultShortRetryAttempts
		}

		lastErr := RetryWithBackoff(ctx, int64(shortRetryAttempts), config.InitialBackoff, config.MaxBackoff, func() error {
			return handler(msg)
		})

		if lastErr == nil {
			return nil
		}

		if shouldSendToDLQ(retryCount, config.MaxLongRetryAttempts) {
			if config.DLQEnabled {
				dlqTopic := msg.Topic + config.DLQTopicPrefix
				if dlqErr := SendToDLQ(ctx, client, dlqTopic, msg, lastErr); dlqErr != nil {
					return fmt.Errorf("handler error: %w, DLQ error: %w", lastErr, dlqErr)
				}
				return nil
			}
			return lastErr
		}

		retryTopic := msg.Topic + config.RetryTopicSuffix
		if firstFailedAt.IsZero() {
			firstFailedAt = time.Now()
		}

		if retryErr := SendToRetryTopic(ctx, client, retryTopic, msg, lastErr, retryCount+1, firstFailedAt); retryErr != nil {
			return fmt.Errorf("handler error: %w, retry topic error: %w", lastErr, retryErr)
		}

		return nil
	}

	return consumer.Subscribe(ctx, topics, wrappedHandler)
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
