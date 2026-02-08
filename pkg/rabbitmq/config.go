package rabbitmq

import "time"

// QueueType represents the type of RabbitMQ queue
type QueueType string

const (
	// QueueTypeQuorum uses Raft consensus for data safety (recommended for production)
	QueueTypeQuorum QueueType = "quorum"
	// QueueTypeClassic is the traditional queue type (use only for transient data)
	QueueTypeClassic QueueType = "classic"
)

// Default configuration values
const (
	DefaultPrefetchCount      = 10
	DefaultRetryTTL           = 30 * time.Second
	DefaultMaxRetries         = 3
	DefaultHeartbeatInterval  = 60 * time.Second
	DefaultConnectionTimeout  = 30 * time.Second
	DefaultReconnectInitial   = 1 * time.Second
	DefaultReconnectMax       = 60 * time.Second
	DefaultChannelPoolSize    = 10
	DefaultPublisherConfirms  = true
	DefaultMandatory          = false
	DefaultPersistentDelivery = true
)

// Config holds RabbitMQ configuration
type Config struct {
	// Connection settings
	URL            string
	ConnectionName string

	// Producer settings
	PublisherConfirms  bool
	ChannelPoolSize    int
	Mandatory          bool
	PersistentDelivery bool

	// Consumer settings
	PrefetchCount int
	AutoAck       bool // Should always be false in production

	// Queue settings
	QueueType  QueueType
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	NoWait     bool

	// Retry/DLX settings
	RetryEnabled      bool
	RetryTTL          time.Duration
	MaxRetries        int
	DeadLetterEnabled bool

	// Reconnection settings
	ReconnectInitialInterval time.Duration
	ReconnectMaxInterval     time.Duration
}

// NewDefaultConfig returns a Config with sensible defaults for production
func NewDefaultConfig() *Config {
	return &Config{
		ConnectionName:           "go-service",
		PublisherConfirms:        DefaultPublisherConfirms,
		ChannelPoolSize:          DefaultChannelPoolSize,
		Mandatory:                DefaultMandatory,
		PersistentDelivery:       DefaultPersistentDelivery,
		PrefetchCount:            DefaultPrefetchCount,
		AutoAck:                  false,
		QueueType:                QueueTypeQuorum,
		Durable:                  true,
		AutoDelete:               false,
		Exclusive:                false,
		NoWait:                   false,
		RetryEnabled:             true,
		RetryTTL:                 DefaultRetryTTL,
		MaxRetries:               DefaultMaxRetries,
		DeadLetterEnabled:        true,
		ReconnectInitialInterval: DefaultReconnectInitial,
		ReconnectMaxInterval:     DefaultReconnectMax,
	}
}

// QueueConfig represents configuration for a specific queue
type QueueConfig struct {
	Name       string
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	NoWait     bool
	Args       map[string]interface{}
}

// ExchangeConfig represents configuration for an exchange
type ExchangeConfig struct {
	Name       string
	Kind       string // direct, fanout, topic, headers
	Durable    bool
	AutoDelete bool
	Internal   bool
	NoWait     bool
	Args       map[string]interface{}
}

// BindingConfig represents a queue binding
type BindingConfig struct {
	QueueName  string
	Exchange   string
	RoutingKey string
	NoWait     bool
	Args       map[string]interface{}
}
