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
	reader         *kafkago.Reader
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	groupID        string
	logger         logger.Logger
	client         Client
	retryCfg       RetryConfig
	topics         []string
	metrics        *KafkaMetrics
	schemaRegistry *SchemaRegistry
}

func NewKafkaConsumer(cfg *ConsumerConfig, brokers []string, groupID string, topics []string, dialer *kafkago.Dialer, logger logger.Logger) *KafkaConsumer {
	// Determine start offset based on configuration
	var startOffset int64
	switch cfg.StartOffset {
	case StartOffsetLatest:
		startOffset = kafkago.LastOffset
	case StartOffsetNone:
		// For "none", we'll use LastOffset but will need additional logic
		// to detect if consumer group is new (not yet implemented)
		startOffset = kafkago.LastOffset
	default: // StartOffsetEarliest or unset
		startOffset = kafkago.FirstOffset
	}

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
		StartOffset: startOffset,
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

// SetMetrics sets the metrics collector for the consumer
func (c *KafkaConsumer) SetMetrics(metrics *KafkaMetrics) {
	c.metrics = metrics
}

// SetSchemaRegistry sets the schema registry for schema-aware consuming
func (c *KafkaConsumer) SetSchemaRegistry(schemaRegistry *SchemaRegistry) {
	c.schemaRegistry = schemaRegistry
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

		// Exponential backoff with jitter before next attempt
		if attempt < shortRetries-1 {
			delay := calculateBackoff(c.retryCfg.InitialBackoff, c.retryCfg.MaxBackoff, attempt)

			// Non-blocking sleep with context cancellation
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
				// Continue to next attempt
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
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
		// Record DLQ metrics
		if c.metrics != nil {
			c.metrics.RecordDLQ(ctx, message.Topic, dlqTopic)
		}
		return nil // Message successfully routed to DLQ
	}

	// Send to retry topic
	retryTopic := buildRetryTopic(message.Topic, c.retryCfg.RetryTopicSuffix)
	if err := SendToRetryTopic(ctx, c.client, c.logger, retryTopic, message, handlerErr, newRetryCount, firstFailedAt); err != nil {
		return fmt.Errorf("failed to send to retry topic: %w", err)
	}
	// Record retry metrics
	if c.metrics != nil {
		c.metrics.RecordRetry(ctx, message.Topic, newRetryCount)
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
				var lag int64 = -1
				if c.reader != nil {
					lag = c.reader.Lag()
					if lag >= 0 {
						c.logger.Debug(ctx, "Consumer lag",
							logger.Field{Key: "partition", Value: msg.Partition},
							logger.Field{Key: "lag", Value: lag})
					}
				}

				// Record metrics if available
				if c.metrics != nil {
					c.metrics.RecordConsume(ctx, msg.Topic, msg.Partition, lag)
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
				commitStart := time.Now()
				commitErr := c.reader.CommitMessages(ctx, msg)

				// Record commit metrics if available
				if c.metrics != nil {
					c.metrics.RecordCommit(ctx, msg.Topic, msg.Partition, time.Since(commitStart), commitErr)
				}

				if commitErr != nil {
					c.logError(ctx, "Error committing message",
						logger.Field{Key: "error", Value: commitErr},
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
	c.schemaRegistry = nil
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

// calculateBackoff calculates exponential backoff with jitter
// initialBackoff and maxBackoff are in milliseconds
func calculateBackoff(initialBackoff, maxBackoff int64, attempt int) time.Duration {
	// Calculate exponential delay: initialBackoff * 2^attempt
	delay := time.Duration(initialBackoff) * time.Millisecond * time.Duration(1<<attempt)

	// Cap at maxBackoff
	if delay > time.Duration(maxBackoff)*time.Millisecond {
		delay = time.Duration(maxBackoff) * time.Millisecond
	}

	// Add jitter: ±20% to prevent thundering herd
	// Use crypto/rand for better randomness in production
	jitter := time.Duration(float64(delay) * 0.2 * (0.5 - float64(time.Now().UnixNano()%100)/100.0))

	return delay + jitter
}
