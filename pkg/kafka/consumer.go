package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gosdk/pkg/logger"

	kafkago "github.com/segmentio/kafka-go"
)

type KafkaConsumer struct {
	reader   *kafkago.Reader
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	groupID  string
	logger   logger.Logger
	client   Client
	retryCfg RetryConfig
	topics   []string
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
		topics:  topics,
	}
}

// SetRetryConfig sets the retry configuration and client for DLQ/retry operations
func (c *KafkaConsumer) SetRetryConfig(client Client, retryCfg RetryConfig) {
	c.client = client
	c.retryCfg = retryCfg
}

func (c *KafkaConsumer) logError(ctx context.Context, msg string, fields ...logger.Field) {
	c.logger.Error(ctx, msg, fields...)
}

// processMessageWithRetry handles message processing with short-term retries and DLQ/routing
func (c *KafkaConsumer) processMessageWithRetry(
	ctx context.Context,
	message Message,
	handler ConsumerHandler,
	retryCount int,
	firstFailedAt time.Time,
) error {
	// Track first failure time
	if firstFailedAt.IsZero() {
		firstFailedAt = time.Now()
	}

	// Try short-term retries with exponential backoff
	shortRetries := c.retryCfg.ShortRetryAttempts
	if shortRetries <= 0 {
		shortRetries = defaultShortRetryAttempts
	}

	var handlerErr error
	for attempt := 0; attempt < shortRetries; attempt++ {
		handlerErr = handler(message)
		if handlerErr == nil {
			return nil // Success!
		}

		// Don't retry on context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Exponential backoff before next attempt
		if attempt < shortRetries-1 {
			delay := time.Duration(c.retryCfg.InitialBackoff) * time.Millisecond * time.Duration(1<<attempt)
			if delay > time.Duration(c.retryCfg.MaxBackoff)*time.Millisecond {
				delay = time.Duration(c.retryCfg.MaxBackoff) * time.Millisecond
			}
			time.Sleep(delay)
		}
	}

	// Short-term retries exhausted - check if we should send to DLQ or retry topic
	if c.client == nil {
		// No retry/DLQ configured, return the error
		return handlerErr
	}

	// Increment retry count for this message
	newRetryCount := retryCount + 1

	// Check if we've exceeded max long-term retries
	maxLongRetries := c.retryCfg.MaxLongRetryAttempts
	if maxLongRetries <= 0 {
		maxLongRetries = defaultMaxLongRetries
	}

	if shouldSendToDLQ(newRetryCount, maxLongRetries) && c.retryCfg.DLQEnabled {
		// Send to DLQ
		dlqTopic := buildDLQTopic(message.Topic, c.retryCfg.DLQTopicPrefix)
		if err := SendToDLQ(ctx, c.client, c.logger, dlqTopic, message, handlerErr); err != nil {
			return fmt.Errorf("failed to send to DLQ: %w", err)
		}
		return nil // Message successfully routed to DLQ
	}

	// Send to retry topic
	retryTopic := buildRetryTopic(message.Topic, c.retryCfg.RetryTopicSuffix)
	if err := SendToRetryTopic(ctx, c.client, c.logger, retryTopic, message, handlerErr, newRetryCount, firstFailedAt); err != nil {
		return fmt.Errorf("failed to send to retry topic: %w", err)
	}

	return nil // Message successfully routed to retry topic
}

func buildDLQTopic(originalTopic, prefix string) string {
	if prefix == "" {
		return originalTopic + ".dlq"
	}
	return prefix + originalTopic
}

func buildRetryTopic(originalTopic, suffix string) string {
	if suffix == "" {
		return originalTopic + defaultRetryTopicSuffix
	}
	return originalTopic + suffix
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
						c.logger.Debug(ctx, "Consumer lag",
							logger.Field{Key: "partition", Value: msg.Partition},
							logger.Field{Key: "lag", Value: lag})
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

				// Extract retry metadata from headers
				retryCount := extractRetryCount(headers)
				firstFailedAt := parseFirstFailedAt(headers)

				// Process message with retry logic
				processErr := c.processMessageWithRetry(ctx, message, handler, retryCount, firstFailedAt)

				if processErr != nil {
					c.logError(ctx, "Failed to process message after all retry attempts",
						logger.Field{Key: "error", Value: processErr},
						logger.Field{Key: "topic", Value: msg.Topic},
						logger.Field{Key: "partition", Value: msg.Partition},
						logger.Field{Key: "offset", Value: msg.Offset})
					// Don't commit - message will be redelivered
					continue
				}

				// Commit message after successful processing or successful retry/DLQ routing
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
