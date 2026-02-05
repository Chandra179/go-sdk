package kafka

import (
	"context"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
)

func NewKafkaClient(slogInstance *slog.Logger, brokers []string, topics []string) (*kgo.Client, error) {
	k := kotel.NewKotel()
	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.ConsumeTopics(topics...),

		// Producer settings
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordRetries(10), // Increase retries
		kgo.RequestRetries(3),
		kgo.RetryBackoffFn(func(tries int) time.Duration {
			return time.Duration(tries) * 100 * time.Millisecond
		}),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),
		kgo.ProducerBatchMaxBytes(1000000),        // 1MB batches
		kgo.ProducerLinger(10 * time.Millisecond), // Batch window

		// Consumer settings
		kgo.ConsumerGroup("my-service-group"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()), // AtStart is dangerous in prod
		kgo.DisableAutoCommit(),                         // Manual commits for better control
		kgo.FetchMaxWait(500 * time.Millisecond),
		kgo.FetchMinBytes(1),
		kgo.FetchMaxBytes(50 * 1024 * 1024), // 50MB

		// Timeouts
		kgo.RequestTimeoutOverhead(10 * time.Second),
		kgo.ConnIdleTimeout(60 * time.Second),

		// Observability
		kgo.WithLogger(&SlogShim{L: slogInstance}),
		kgo.WithHooks(k.Hooks()...),
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	return client, nil
}

type SlogShim struct {
	L *slog.Logger
}

func (s *SlogShim) Level() kgo.LogLevel {
	return kgo.LogLevelInfo
}

func (s *SlogShim) Log(level kgo.LogLevel, msg string, keyvals ...any) {
	var slogLevel slog.Level
	switch level {
	case kgo.LogLevelError:
		slogLevel = slog.LevelError
	case kgo.LogLevelWarn:
		slogLevel = slog.LevelWarn
	case kgo.LogLevelInfo:
		slogLevel = slog.LevelInfo
	case kgo.LogLevelDebug:
		slogLevel = slog.LevelDebug
	default:
		return
	}
	s.L.Log(context.Background(), slogLevel, msg, keyvals...)
}
