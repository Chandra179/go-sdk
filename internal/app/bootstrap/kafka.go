package bootstrap

import (
	"gosdk/cfg"
	"gosdk/pkg/kafka"
	"gosdk/pkg/logger"
)

func InitKafka(config *cfg.KafkaConfig, logger logger.Logger) (kafka.Client, error) {
	kafkaConfig := kafka.NewConfig(config)
	return kafka.NewClient(kafkaConfig, logger)
}
