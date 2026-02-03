package kafka

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

type KafkaAdmin struct {
	admClient  *kadm.Client
	timeout    time.Duration
	maxRetries uint
	retryDelay time.Duration
}

type AdminConfig struct {
	DefaultPartitions  int
	DefaultReplication int
	Timeout            time.Duration
	MaxRetries         uint
	RetryDelay         time.Duration
}

func DefaultAdminConfig() AdminConfig {
	return AdminConfig{
		DefaultPartitions:  3,
		DefaultReplication: 2,
		Timeout:            10 * time.Second,
		MaxRetries:         3,
		RetryDelay:         1 * time.Second,
	}
}

func NewKafkaAdmin(brokers []string, cfg *AdminConfig) (*KafkaAdmin, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("%w: at least one broker required", ErrRequiredBrokers)
	}

	if cfg == nil {
		defaultCfg := DefaultAdminConfig()
		cfg = &defaultCfg
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.Dialer((&net.Dialer{Timeout: cfg.Timeout}).DialContext),
	}

	admClient, err := kadm.NewOptClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create admin client: %w", err)
	}

	return &KafkaAdmin{
		admClient:  admClient,
		timeout:    cfg.Timeout,
		maxRetries: cfg.MaxRetries,
		retryDelay: cfg.RetryDelay,
	}, nil
}

func (a *KafkaAdmin) TopicExists(ctx context.Context, topic string) (bool, error) {
	if topic == "" {
		return false, fmt.Errorf("topic name cannot be empty")
	}

	topics, err := a.admClient.ListTopics(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to list topics: %w", err)
	}

	for _, t := range topics {
		if t.Topic == topic {
			return true, nil
		}
	}

	return false, nil
}

func (a *KafkaAdmin) ValidateTopicsExist(ctx context.Context, topics []string) error {
	if len(topics) == 0 {
		return fmt.Errorf("no topics specified for validation")
	}

	existingTopics, err := a.admClient.ListTopics(ctx)
	if err != nil {
		return fmt.Errorf("failed to list topics: %w", err)
	}

	existingSet := make(map[string]bool)
	for _, t := range existingTopics {
		existingSet[t.Topic] = true
	}

	var missingTopics []string
	for _, topic := range topics {
		if topic == "" {
			continue
		}
		if !existingSet[topic] {
			missingTopics = append(missingTopics, topic)
		}
	}

	if len(missingTopics) > 0 {
		return fmt.Errorf("topics not found: %v. Ensure topics are created via your infrastructure tooling", missingTopics)
	}

	return nil
}

func (a *KafkaAdmin) ValidateTopicsExistWithRetry(ctx context.Context, topics []string) error {
	return retry.Do(
		func() error {
			return a.ValidateTopicsExist(ctx, topics)
		},
		retry.Context(ctx),
		retry.Attempts(a.maxRetries),
		retry.Delay(a.retryDelay),
		retry.DelayType(retry.BackOffDelay),
		retry.MaxDelay(5*time.Second),
		retry.LastErrorOnly(true),
	)
}

func (a *KafkaAdmin) CreateTopic(ctx context.Context, topic string, partitions int, replicationFactor int, configEntries map[string]string) error {
	if topic == "" {
		return fmt.Errorf("topic name cannot be empty")
	}
	if partitions <= 0 {
		partitions = 3
	}
	if replicationFactor <= 0 {
		replicationFactor = 1
	}

	configs := make(map[string]*string)
	for k, v := range configEntries {
		configs[k] = &v
	}

	resp, err := a.admClient.CreateTopics(ctx, int32(partitions), int16(replicationFactor), configs, topic)
	if err != nil {
		return fmt.Errorf("failed to create topic %s: %w", topic, err)
	}

	for _, t := range resp {
		if t.Err != nil {
			if containsSubstring(t.Err.Error(), "topic already exists") {
				continue
			}
			return fmt.Errorf("failed to create topic %s: %w", topic, t.Err)
		}
	}

	return nil
}

func (a *KafkaAdmin) DeleteTopic(ctx context.Context, topic string) error {
	if topic == "" {
		return fmt.Errorf("topic name cannot be empty")
	}

	resp, err := a.admClient.DeleteTopics(ctx, topic)
	if err != nil {
		return fmt.Errorf("failed to delete topic %s: %w", topic, err)
	}

	for _, t := range resp {
		if t.Err != nil {
			return fmt.Errorf("failed to delete topic %s: %w", topic, t.Err)
		}
	}

	return nil
}

func (a *KafkaAdmin) Close() error {
	a.admClient.Close()
	return nil
}

func containsSubstring(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
