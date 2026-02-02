// Package producer provides a Kafka producer example that publishes order events.
package producer

import (
	"context"
	"encoding/json"
	"fmt"
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

// NewOrderEvent creates a new order event with given parameters
func NewOrderEvent(orderID, userID string, amount float64, status string) OrderEvent {
	return OrderEvent{
		OrderID:   orderID,
		UserID:    userID,
		Amount:    amount,
		Status:    status,
		Timestamp: time.Now(),
	}
}

// ToJSON serializes OrderEvent to JSON bytes
func (o OrderEvent) ToJSON() ([]byte, error) {
	return json.Marshal(o)
}

// getEnv retrieves environment variable with fallback default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Run starts the producer example
func Run() {
	ctx := context.Background()

	// Initialize OpenTelemetry for observability
	otelConfig := &cfg.OtelConfig{
		OTLPEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		ServiceName:  getEnv("OTEL_SERVICE_NAME", "producer-example"),
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
	// Idempotency is enabled by default - messages with identical content within the
	// deduplication window will be automatically skipped
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
		// Idempotency configuration - enabled by default with 5-minute window
		Idempotency: kafka.IdempotencyConfig{
			Enabled:      true,
			WindowSize:   5 * time.Minute,
			MaxCacheSize: 10000,
		},
	}

	client, err := kafka.NewClient(kafkaConfig, logger)
	if err != nil {
		log.Fatalf("Failed to create Kafka client: %v", err)
	}
	defer client.Close()

	producer, err := client.Producer()
	if err != nil {
		log.Fatalf("Failed to create producer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Get topic from environment or use default
	ordersTopic := "orders"
	if topic := os.Getenv("KAFKA_ORDERS_TOPIC"); topic != "" {
		ordersTopic = topic
	}

	// Start producing messages in a goroutine
	go func() {
		for i := 1; i <= 10; i++ {
			orderEvent := NewOrderEvent(
				fmt.Sprintf("order-%d", i),
				fmt.Sprintf("user-%d", i%3+1),
				float64(i)*10.50,
				"created",
			)

			messageData, err := orderEvent.ToJSON()
			if err != nil {
				log.Printf("Failed to serialize order event: %v", err)
				continue
			}

			err = producer.Publish(ctx, kafka.Message{
				Topic: ordersTopic,
				Key:   []byte(orderEvent.OrderID),
				Value: messageData,
				Headers: map[string]string{
					"event_type": "order_created",
					"source":     "example_producer",
					"version":    "1.0",
				},
			})

			if err != nil {
				log.Printf("Failed to publish message: %v", err)
			} else {
				log.Printf("Published order %s (amount: $%.2f)", orderEvent.OrderID, orderEvent.Amount)
			}

			// Wait a bit between messages
			time.Sleep(1 * time.Second)
		}
	}()

	// Wait for shutdown signal
	<-sigCh
	log.Println("Received shutdown signal, stopping producer...")
	cancel()

	// Give some time for in-flight messages to complete
	time.Sleep(2 * time.Second)
	log.Println("Producer shutdown complete")
}
