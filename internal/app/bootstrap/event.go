package bootstrap

import (
	"gosdk/internal/service/event"
	"gosdk/pkg/kafka"
)

func InitEvent(brokers []string) (kafka.Client, *event.Service, *event.Handler) {
	kafkaClient := kafka.NewClient(brokers)
	messageBrokerSvc := event.NewService(kafkaClient)
	messageBrokerHandler := event.NewHandler(messageBrokerSvc)
	return kafkaClient, messageBrokerSvc, messageBrokerHandler
}
