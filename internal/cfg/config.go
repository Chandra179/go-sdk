package cfg

import (
	"time"
)

type Config struct {
	AppEnv          string
	Redis           RedisConfig
	Postgres        PostgresConfig
	OAuth2          Oauth2Config
	Observability   OtelConfig
	Kafka           *KafkaConfig
	RabbitMQ        *RabbitMQConfig
	Temporal        *TemporalConfig
	HTTPServer      HTTPServerConfig
	ShutdownTimeout time.Duration
}

func Load() (*Config, error) {
	l := NewLoader()

	cfg := &Config{
		AppEnv:          l.requireEnv("APP_ENV"),
		Redis:           l.loadRedis(),
		OAuth2:          l.loadOAuth2(),
		Postgres:        l.loadPostgres(),
		Observability:   l.loadOtel(),
		Kafka:           l.loadKafka(),
		RabbitMQ:        l.loadRabbitMQ(),
		Temporal:        l.loadTemporal(),
		HTTPServer:      l.loadHTTPServer(),
		ShutdownTimeout: l.requireDuration("SHUTDOWN_TIMEOUT"),
	}

	if l.HasErrors() {
		return nil, l.Error()
	}

	return cfg, nil
}
