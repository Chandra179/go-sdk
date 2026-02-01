// Package consumer provides a Kafka consumer example that processes order events.
package consumer

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gosdk/cfg"
	"gosdk/internal/app/bootstrap"
	"gosdk/pkg/kafka"
	"gosdk/pkg/logger"
)

// OrderEvent represents an order-related event in system
type OrderEvent struct {
	OrderID   string    `json:"order_id"`
	UserID    string    `json:"user_id"`
	Amount    float64   `json:"amount"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// FromJSON deserializes JSON bytes to an OrderEvent
func FromJSON(data []byte) (OrderEvent, error) {
	var event OrderEvent
	err := json.Unmarshal(data, &event)
	if err != nil {
		return OrderEvent{}, err
	}
	return event, nil
}

// getEnv retrieves environment variable with fallback default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Run starts the consumer example
func Run() {
	ctx := context.Background()

	// Initialize OpenTelemetry for observability
	otelConfig := &cfg.OtelConfig{
		OTLPEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		ServiceName:  getEnv("OTEL_SERVICE_NAME", "consumer-example"),
	}
	shutdown, _, err := bootstrap.InitOtel(ctx, otelConfig, 1.0)
	if err != nil {
		log.Printf("Warning: Failed to initialize OTEL: %v", err)
		// Continue without OTEL - logs will still go to stdout
	} else {
		defer func() {
			if err := shutdown(ctx); err != nil {
				log.Printf("Error shutting down OTEL: %v", err)
			}
		}()
	}

	// For examples, we'll create a minimal config using environment variables
	// In production, you would use cfg.Load() to get full config
	appEnv := "development"
	if env := os.Getenv("APP_ENV"); env != "" {
		appEnv = env
	}

	// Create logger using pkg/logger (now with OTEL support)
	logger := logger.NewLogger(appEnv)

	// Get Kafka brokers from environment or use default
	brokers := []string{"localhost:9092"}
	if brokersEnv := os.Getenv("KAFKA_BROKERS"); brokersEnv != "" {
		brokers = strings.Split(brokersEnv, ",")
	}

	// Create minimal kafka config for examples
	kafkaConfig := &kafka.Config{
		Brokers: brokers,
		Producer: kafka.ProducerConfig{
			RequiredAcks:    "all",
			BatchSize:       1000000,
			LingerMs:        10,
			CompressionType: "snappy",
			MaxAttempts:     5,
			Async:           false,
		},
		Consumer: kafka.ConsumerConfig{
			MinBytes:              1024,
			MaxBytes:              10000000,
			CommitInterval:        1000 * time.Millisecond,
			MaxPollRecords:        100,
			HeartbeatInterval:     3000 * time.Millisecond,
			SessionTimeout:        6000 * time.Millisecond,
			WatchPartitionChanges: true,
		},
		Security: kafka.SecurityConfig{
			Enabled: false,
		},
		Retry: kafka.RetryConfig{
			MaxRetries:           3,
			InitialBackoff:       100,
			MaxBackoff:           1000,
			DLQEnabled:           true,
			DLQTopicPrefix:       ".dlq",
			ShortRetryAttempts:   3,
			MaxLongRetryAttempts: 3,
			RetryTopicSuffix:     ".retry",
		},
	}

	client, err := kafka.NewClient(kafkaConfig, logger)
	if err != nil {
		log.Fatalf("Failed to create Kafka client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Get topic from environment or use default
	ordersTopic := "orders"
	if topic := os.Getenv("KAFKA_ORDERS_TOPIC"); topic != "" {
		ordersTopic = topic
	}

	// Consumer handler function
	handler := func(msg kafka.Message) error {
		orderEvent, err := FromJSON(msg.Value)
		if err != nil {
			log.Printf("Failed to deserialize order event: %v", err)
			return err
		}

		log.Printf("Processing order: ID=%s, UserID=%s, Amount=$%.2f, Status=%s",
			orderEvent.OrderID, orderEvent.UserID, orderEvent.Amount, orderEvent.Status)

		// Simulate some processing
		if orderEvent.Amount > 50.0 {
			log.Printf("High value order %s requires special handling", orderEvent.OrderID)
		}

		return nil
	}

	// Start consumer in a goroutine
	go func() {
		log.Printf("Starting consumer for topic: %s", ordersTopic)
		consumer, err := client.Consumer("order-processor-group", []string{ordersTopic})
		if err != nil {
			log.Printf("Failed to create consumer: %v", err)
			return
		}

		err = consumer.Start(ctx, handler)
		if err != nil {
			log.Printf("Consumer error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-sigCh
	log.Println("Received shutdown signal, stopping consumer...")
	cancel()

	// Give some time for cleanup
	log.Println("Consumer shutdown complete")
}
