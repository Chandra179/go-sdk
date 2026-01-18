package kafka

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	t.Run("successful creation", func(t *testing.T) {
		brokers := []string{"localhost:9092"}
		client := NewClient(brokers)

		assert.NotNil(t, client)
		kafkaClient, ok := client.(*KafkaClient)
		assert.True(t, ok)
		assert.Equal(t, brokers, kafkaClient.brokers)
	})

	t.Run("Ping method exists", func(t *testing.T) {
		brokers := []string{"localhost:9092"}
		client := NewClient(brokers)

		err := client.Ping(context.Background())
		assert.NoError(t, err)
	})
}
