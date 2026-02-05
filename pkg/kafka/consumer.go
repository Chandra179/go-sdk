package kafka

import (
	"context"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"
)

func StartConsumer(ctx context.Context, client *kgo.Client, handler func(*kgo.Record) error) error {
	defer client.Close()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		fetches := client.PollFetches(ctx)
		if ctx.Err() != nil {
			// Context cancelled during poll, no messages fetched
			return ctx.Err()
		}

		if fetches.IsClientClosed() {
			return nil
		}

		if errs := fetches.Errors(); len(errs) > 0 {
			for _, err := range errs {
				slog.Error("fetch error", "error", err)
			}
			continue
		}

		var lastErr error
		fetches.EachRecord(func(r *kgo.Record) {
			// Check if context cancelled during processing
			if ctx.Err() != nil {
				lastErr = ctx.Err()
				return // Stop processing this batch
			}

			if err := handler(r); err != nil {
				slog.Error("handler error", "topic", r.Topic, "partition", r.Partition, "offset", r.Offset, "error", err)
				lastErr = err
			}
		})

		// Only commit if we completed the batch without cancellation
		if lastErr == nil {
			if err := client.CommitUncommittedOffsets(ctx); err != nil {
				slog.Error("commit error", "error", err)
			}
		} else if ctx.Err() != nil {
			// Context cancelled mid-batch, don't commit partial work
			slog.Warn("shutdown during batch processing, not committing")
			return ctx.Err()
		}
	}
}
