package kafka

import (
	"context"
	"time"

	"github.com/avast/retry-go/v4"
)

const (
	defaultShortRetryAttempts = 3
	defaultMaxLongRetries     = 3
	defaultRetryTopicSuffix   = ".retry"
)

func RetryWithBackoff(
	ctx context.Context,
	maxRetries int64,
	initialBackoff, maxBackoff int64,
	fn func() error,
) error {
	if maxRetries <= 0 {
		maxRetries = 3
	}

	opts := []retry.Option{
		retry.Attempts(uint(maxRetries)),
		retry.Delay(time.Duration(initialBackoff) * time.Millisecond),
		retry.MaxDelay(time.Duration(maxBackoff) * time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
		retry.Context(ctx),
	}

	return retry.Do(fn, opts...)
}
