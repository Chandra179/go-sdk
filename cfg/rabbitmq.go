package cfg

import (
	"errors"
	"time"

	"gosdk/pkg/rabbitmq"
)

const (
	defaultPrefetchCount        = 10
	defaultRetryTTLSeconds      = 30
	defaultMaxRetries           = 3
	defaultChannelPoolSize      = 10
	defaultReconnectInitialSecs = 1
	defaultReconnectMaxSecs     = 60
)

// RabbitMQConfig extends the base rabbitmq.Config with environment-based loading
type RabbitMQConfig struct {
	rabbitmq.Config
}

func (l *Loader) loadRabbitMQ() *RabbitMQConfig {
	url := l.getEnvWithDefault("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	connectionName := l.getEnvWithDefault("RABBITMQ_CONNECTION_NAME", "go-service")

	// Producer settings
	publisherConfirms := l.getEnvBoolWithDefault("RABBITMQ_PUBLISHER_CONFIRMS", true)
	channelPoolSize := l.getEnvIntWithDefault("RABBITMQ_CHANNEL_POOL_SIZE", defaultChannelPoolSize)
	mandatory := l.getEnvBoolWithDefault("RABBITMQ_MANDATORY", false)
	persistentDelivery := l.getEnvBoolWithDefault("RABBITMQ_PERSISTENT_DELIVERY", true)

	// Consumer settings
	prefetchCount := l.getEnvIntWithDefault("RABBITMQ_PREFETCH_COUNT", defaultPrefetchCount)
	autoAck := l.getEnvBoolWithDefault("RABBITMQ_AUTO_ACK", false)

	// Queue settings
	queueTypeStr := l.getEnvWithDefault("RABBITMQ_QUEUE_TYPE", string(rabbitmq.QueueTypeQuorum))
	var queueType rabbitmq.QueueType
	switch queueTypeStr {
	case "quorum":
		queueType = rabbitmq.QueueTypeQuorum
	case "classic":
		queueType = rabbitmq.QueueTypeClassic
	default:
		l.errs = append(l.errs, errors.New("invalid RABBITMQ_QUEUE_TYPE: "+queueTypeStr+", must be 'quorum' or 'classic'"))
		queueType = rabbitmq.QueueTypeQuorum
	}

	durable := l.getEnvBoolWithDefault("RABBITMQ_DURABLE", true)
	autoDelete := l.getEnvBoolWithDefault("RABBITMQ_AUTO_DELETE", false)
	exclusive := l.getEnvBoolWithDefault("RABBITMQ_EXCLUSIVE", false)
	noWait := l.getEnvBoolWithDefault("RABBITMQ_NO_WAIT", false)

	// Retry/DLX settings
	retryEnabled := l.getEnvBoolWithDefault("RABBITMQ_RETRY_ENABLED", true)
	retryTTLSeconds := l.getEnvIntWithDefault("RABBITMQ_RETRY_TTL_SECONDS", defaultRetryTTLSeconds)
	maxRetries := l.getEnvIntWithDefault("RABBITMQ_MAX_RETRIES", defaultMaxRetries)
	deadLetterEnabled := l.getEnvBoolWithDefault("RABBITMQ_DEAD_LETTER_ENABLED", true)

	// Reconnection settings
	reconnectInitialSecs := l.getEnvIntWithDefault("RABBITMQ_RECONNECT_INITIAL_SECONDS", defaultReconnectInitialSecs)
	reconnectMaxSecs := l.getEnvIntWithDefault("RABBITMQ_RECONNECT_MAX_SECONDS", defaultReconnectMaxSecs)

	return &RabbitMQConfig{
		Config: rabbitmq.Config{
			URL:                      url,
			ConnectionName:           connectionName,
			PublisherConfirms:        publisherConfirms,
			ChannelPoolSize:          channelPoolSize,
			Mandatory:                mandatory,
			PersistentDelivery:       persistentDelivery,
			PrefetchCount:            prefetchCount,
			AutoAck:                  autoAck,
			QueueType:                queueType,
			Durable:                  durable,
			AutoDelete:               autoDelete,
			Exclusive:                exclusive,
			NoWait:                   noWait,
			RetryEnabled:             retryEnabled,
			RetryTTL:                 time.Duration(retryTTLSeconds) * time.Second,
			MaxRetries:               maxRetries,
			DeadLetterEnabled:        deadLetterEnabled,
			ReconnectInitialInterval: time.Duration(reconnectInitialSecs) * time.Second,
			ReconnectMaxInterval:     time.Duration(reconnectMaxSecs) * time.Second,
		},
	}
}
