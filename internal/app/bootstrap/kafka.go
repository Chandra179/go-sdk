package bootstrap

import (
	"gosdk/cfg"
	"gosdk/pkg/kafka"
)

func InitKafka(config *cfg.KafkaConfig) (kafka.Client, error) {
	kafkaConfig := kafka.NewConfig(config)
	return kafka.NewClient(kafkaConfig)
}
