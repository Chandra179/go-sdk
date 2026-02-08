package kafka

import (
	"context"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// CommitMode defines when offsets are committed.
type CommitMode int

const (
	// CommitBatch commits after processing an entire fetch batch.
	CommitBatch CommitMode = iota
	// CommitRecord commits after each record is processed.
	CommitRecord
)

// ConsumerOptions configures consumer behavior.
type ConsumerOptions struct {
	DLQProducer  *Producer
	OnDLQPublish func(topic string, err error)
	MaxRetries   int
	RetryBackoff time.Duration
	Logger       *slog.Logger
	Metrics      *Metrics
	CommitMode   CommitMode
	GroupID      string // For metrics labeling
}

// StartConsumer starts consuming messages and processing them with the handler.
// It blocks until the context is cancelled or an error occurs.
func StartConsumer(ctx context.Context, client *kgo.Client, handler func(*kgo.Record) error, opts ...ConsumerOptions) error {
	options := ConsumerOptions{
		MaxRetries:   3,
		RetryBackoff: 100 * time.Millisecond,
		Logger:       slog.Default(),
		CommitMode:   CommitBatch,
	}
	for _, o := range opts {
		if o.MaxRetries > 0 {
			options.MaxRetries = o.MaxRetries
		}
		if o.RetryBackoff > 0 {
			options.RetryBackoff = o.RetryBackoff
		}
		if o.Logger != nil {
			options.Logger = o.Logger
		}
		if o.Metrics != nil {
			options.Metrics = o.Metrics
		}
		if o.CommitMode != 0 {
			options.CommitMode = o.CommitMode
		}
		if o.GroupID != "" {
			options.GroupID = o.GroupID
		}
		options.DLQProducer = o.DLQProducer
		options.OnDLQPublish = o.OnDLQPublish
	}

	logger := options.Logger

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		fetches := client.PollFetches(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if fetches.IsClientClosed() {
			return nil
		}

		if errs := fetches.Errors(); len(errs) > 0 {
			for _, err := range errs {
				logger.Error("fetch error", "error", err)
				if options.Metrics != nil {
					options.Metrics.RecordError("fetch", err.Topic)
				}
			}
			continue
		}

		var commitErr error
		fetches.EachRecord(func(r *kgo.Record) {
			if ctx.Err() != nil {
				return
			}

			start := time.Now()
			var lastErr error

			for attempt := 0; attempt <= options.MaxRetries; attempt++ {
				if attempt > 0 {
					backoff := time.Duration(attempt) * options.RetryBackoff
					select {
					case <-ctx.Done():
						return
					case <-time.After(backoff):
					}
					if options.Metrics != nil {
						options.Metrics.RecordRetry(r.Topic)
					}
				}

				if err := handler(r); err != nil {
					lastErr = err
					logger.Warn("handler error, retrying",
						"topic", r.Topic,
						"partition", r.Partition,
						"offset", r.Offset,
						"attempt", attempt+1,
						"max_retries", options.MaxRetries,
						"error", err,
					)
					continue
				}
				lastErr = nil
				break
			}

			duration := time.Since(start)
			success := lastErr == nil

			if options.Metrics != nil {
				options.Metrics.RecordConsume(r.Topic, options.GroupID, duration, success)
			}

			if lastErr != nil {
				if options.DLQProducer != nil {
					dlqErr := options.DLQProducer.SendToDLQSync(ctx, r.Topic, r.Value, r.Key, lastErr)
					if dlqErr != nil {
						logger.Error("failed to send to DLQ", "error", dlqErr)
						if options.Metrics != nil {
							options.Metrics.RecordError("dlq_send", r.Topic)
						}
					} else {
						logger.Error("message sent to DLQ",
							"topic", r.Topic,
							"partition", r.Partition,
							"offset", r.Offset,
							"dlq_topic", r.Topic+".dlq",
							"error", lastErr,
						)
						if options.Metrics != nil {
							options.Metrics.RecordDLQ(r.Topic, "handler_failed")
						}
					}
					if options.OnDLQPublish != nil {
						options.OnDLQPublish(r.Topic, lastErr)
					}
				} else {
					logger.Error("handler failed, no DLQ configured",
						"topic", r.Topic,
						"partition", r.Partition,
						"offset", r.Offset,
						"error", lastErr,
					)
				}
			}

			// Per-record commit if configured
			if options.CommitMode == CommitRecord {
				if err := client.CommitRecords(ctx, r); err != nil {
					logger.Error("per-record commit error", "error", err)
					commitErr = err
				}
			}
		})

		// Batch commit after processing all records
		if options.CommitMode == CommitBatch {
			if err := client.CommitUncommittedOffsets(ctx); err != nil {
				logger.Error("batch commit error", "error", err)
				commitErr = err
			}
		}

		_ = commitErr // Track but don't fail on commit errors
	}
}

// StartConsumerWithDLQ is a convenience function to start a consumer with DLQ support.
func StartConsumerWithDLQ(ctx context.Context, client *kgo.Client, handler func(*kgo.Record) error, dlqProducer *Producer) error {
	return StartConsumer(ctx, client, handler, ConsumerOptions{
		DLQProducer: dlqProducer,
		MaxRetries:  3,
	})
}

// GracefulShutdown waits for in-flight processing to complete.
// Call this before closing the client.
func GracefulShutdown(ctx context.Context, client *kgo.Client, timeout time.Duration) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Commit any remaining offsets
	if err := client.CommitUncommittedOffsets(shutdownCtx); err != nil {
		return err
	}

	// Leave the consumer group gracefully
	client.LeaveGroup()

	return nil
}
