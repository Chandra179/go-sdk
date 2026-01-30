package kafka

import (
	"context"
	"fmt"
	"strconv"
	"time"

	log "gosdk/pkg/logger"
)

const (
	headerRetryCount    = "x-retry-count"
	headerFirstFailedAt = "x-first-failed-at"
	headerLastFailedAt  = "x-last-failed-at"
	headerErrorMessage  = "x-error-message"
	headerOriginalTopic = "original_topic"
)

func SendToRetryTopic(
	ctx context.Context,
	client Client,
	logger log.Logger,
	retryTopic string,
	originalMsg Message,
	handlerErr error,
	retryCount int,
	firstFailedAt time.Time,
) error {
	logger.Info(ctx, "Sending message to retry topic",
		log.Field{Key: "error", Value: handlerErr},
		log.Field{Key: "original_topic", Value: originalMsg.Topic},
		log.Field{Key: "retry_topic", Value: retryTopic},
		log.Field{Key: "retry_count", Value: retryCount})

	producer, err := client.Producer()
	if err != nil {
		return fmt.Errorf("failed to get producer for retry topic: %w", err)
	}

	retryMsg := Message{
		Topic:   retryTopic,
		Key:     originalMsg.Key,
		Value:   originalMsg.Value,
		Headers: copyHeaders(originalMsg.Headers),
	}

	if retryMsg.Headers == nil {
		retryMsg.Headers = make(map[string]string)
	}

	retryMsg.Headers[headerRetryCount] = strconv.Itoa(retryCount)
	retryMsg.Headers[headerFirstFailedAt] = firstFailedAt.UTC().Format(time.RFC3339Nano)
	retryMsg.Headers[headerLastFailedAt] = time.Now().UTC().Format(time.RFC3339Nano)
	retryMsg.Headers[headerErrorMessage] = handlerErr.Error()
	retryMsg.Headers[headerOriginalTopic] = originalMsg.Topic

	return producer.Publish(ctx, retryMsg)
}

func SendToDLQ(
	ctx context.Context,
	client Client,
	logger log.Logger,
	topic string,
	originalMsg Message,
	handlerErr error,
) error {
	logger.Info(ctx, "Sending message to DLQ",
		log.Field{Key: "error", Value: handlerErr},
		log.Field{Key: "original_topic", Value: originalMsg.Topic},
		log.Field{Key: "dlq_topic", Value: topic})

	producer, err := client.Producer()
	if err != nil {
		return fmt.Errorf("failed to get producer for DLQ: %w", err)
	}

	dlqMsg := Message{
		Topic:   topic,
		Key:     originalMsg.Key,
		Value:   originalMsg.Value,
		Headers: copyHeaders(originalMsg.Headers),
	}

	if dlqMsg.Headers == nil {
		dlqMsg.Headers = make(map[string]string)
	}

	dlqMsg.Headers[headerOriginalTopic] = originalMsg.Topic
	dlqMsg.Headers[headerErrorMessage] = handlerErr.Error()
	dlqMsg.Headers[headerLastFailedAt] = time.Now().UTC().Format(time.RFC3339Nano)

	if firstFailedAt, ok := originalMsg.Headers[headerFirstFailedAt]; ok {
		dlqMsg.Headers[headerFirstFailedAt] = firstFailedAt
	} else {
		dlqMsg.Headers[headerFirstFailedAt] = time.Now().UTC().Format(time.RFC3339Nano)
	}

	if retryCount, ok := originalMsg.Headers[headerRetryCount]; ok {
		dlqMsg.Headers[headerRetryCount] = retryCount
	}

	return producer.Publish(ctx, dlqMsg)
}

func copyHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	copied := make(map[string]string, len(headers))
	for k, v := range headers {
		copied[k] = v
	}
	return copied
}

func extractRetryCount(headers map[string]string) int {
	if headers == nil {
		return 0
	}
	if countStr, ok := headers[headerRetryCount]; ok {
		if count, err := strconv.Atoi(countStr); err == nil {
			return count
		}
	}
	return 0
}

func shouldSendToDLQ(retryCount, maxLongRetries int) bool {
	return retryCount >= maxLongRetries
}
