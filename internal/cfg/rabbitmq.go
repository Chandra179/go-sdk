package cfg

import (
	"errors"
	"os"
	"time"

	"gosdk/pkg/rabbitmq"

	"github.com/goccy/go-yaml"
)

// RabbitMQ YAML config file path
const rabbitmqYAMLPath = "internal/cfg/rabbitmq.yaml"

// Environment variable names for RabbitMQ secrets
const (
	envRABBITMQURL = "RABBITMQ_URL"
)

// RabbitMQConfig extends the base rabbitmq.Config with environment-based loading
type RabbitMQConfig struct {
	rabbitmq.Config
}

// YAML config structures - mirrors the yaml file structure
type RabbitMQYAMLConfig struct {
	ConnectionName       string `yaml:"connection_name"`
	PublisherConfirms    bool   `yaml:"publisher_confirms"`
	ChannelPoolSize      int    `yaml:"channel_pool_size"`
	Mandatory            bool   `yaml:"mandatory"`
	PersistentDelivery   bool   `yaml:"persistent_delivery"`
	PrefetchCount        int    `yaml:"prefetch_count"`
	AutoAck              bool   `yaml:"auto_ack"`
	QueueType            string `yaml:"queue_type"`
	Durable              bool   `yaml:"durable"`
	AutoDelete           bool   `yaml:"auto_delete"`
	Exclusive            bool   `yaml:"exclusive"`
	NoWait               bool   `yaml:"no_wait"`
	RetryEnabled         bool   `yaml:"retry_enabled"`
	RetryTTLSeconds      int    `yaml:"retry_ttl_seconds"`
	MaxRetries           int    `yaml:"max_retries"`
	DeadLetterEnabled    bool   `yaml:"dead_letter_enabled"`
	ReconnectInitialSecs int    `yaml:"reconnect_initial_seconds"`
	ReconnectMaxSecs     int    `yaml:"reconnect_max_seconds"`
}

func (l *Loader) loadRabbitMQ() *RabbitMQConfig {
	// Load YAML config
	yamlCfg, err := l.loadRabbitMQYAML()
	if err != nil {
		l.errs = append(l.errs, errors.New("failed to load rabbitmq yaml config: "+err.Error()))
		return nil
	}

	// URL always comes from env (contains credentials)
	url := l.getEnvWithDefault(envRABBITMQURL, "amqp://guest:guest@localhost:5672/")

	// Parse queue type
	var queueType rabbitmq.QueueType
	switch yamlCfg.QueueType {
	case "quorum":
		queueType = rabbitmq.QueueTypeQuorum
	case "classic":
		queueType = rabbitmq.QueueTypeClassic
	default:
		l.errs = append(l.errs, errors.New("invalid rabbitmq queue_type: "+yamlCfg.QueueType+", must be 'quorum' or 'classic'"))
		queueType = rabbitmq.QueueTypeQuorum
	}

	return &RabbitMQConfig{
		Config: rabbitmq.Config{
			URL:                      url,
			ConnectionName:           yamlCfg.ConnectionName,
			PublisherConfirms:        yamlCfg.PublisherConfirms,
			ChannelPoolSize:          yamlCfg.ChannelPoolSize,
			Mandatory:                yamlCfg.Mandatory,
			PersistentDelivery:       yamlCfg.PersistentDelivery,
			PrefetchCount:            yamlCfg.PrefetchCount,
			AutoAck:                  yamlCfg.AutoAck,
			QueueType:                queueType,
			Durable:                  yamlCfg.Durable,
			AutoDelete:               yamlCfg.AutoDelete,
			Exclusive:                yamlCfg.Exclusive,
			NoWait:                   yamlCfg.NoWait,
			RetryEnabled:             yamlCfg.RetryEnabled,
			RetryTTL:                 time.Duration(yamlCfg.RetryTTLSeconds) * time.Second,
			MaxRetries:               yamlCfg.MaxRetries,
			DeadLetterEnabled:        yamlCfg.DeadLetterEnabled,
			ReconnectInitialInterval: time.Duration(yamlCfg.ReconnectInitialSecs) * time.Second,
			ReconnectMaxInterval:     time.Duration(yamlCfg.ReconnectMaxSecs) * time.Second,
		},
	}
}

// loadRabbitMQYAML loads RabbitMQ configuration from internal/cfg/rabbitmq.yaml
func (l *Loader) loadRabbitMQYAML() (*RabbitMQYAMLConfig, error) {
	yamlData, err := os.ReadFile(rabbitmqYAMLPath)
	if err != nil {
		return nil, errors.New("failed to read " + rabbitmqYAMLPath + ": " + err.Error())
	}

	var cfg RabbitMQYAMLConfig
	if err := yaml.Unmarshal(yamlData, &cfg); err != nil {
		return nil, errors.New("failed to parse " + rabbitmqYAMLPath + ": " + err.Error())
	}

	// Set defaults for zero values
	if cfg.ConnectionName == "" {
		cfg.ConnectionName = "go-service"
	}
	if cfg.ChannelPoolSize == 0 {
		cfg.ChannelPoolSize = 10
	}
	if cfg.PrefetchCount == 0 {
		cfg.PrefetchCount = 10
	}
	if cfg.QueueType == "" {
		cfg.QueueType = "quorum"
	}
	if cfg.RetryTTLSeconds == 0 {
		cfg.RetryTTLSeconds = 30
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.ReconnectInitialSecs == 0 {
		cfg.ReconnectInitialSecs = 1
	}
	if cfg.ReconnectMaxSecs == 0 {
		cfg.ReconnectMaxSecs = 60
	}

	return &cfg, nil
}
