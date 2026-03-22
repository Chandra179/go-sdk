package example

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var sharedURL string

// TestMain starts one RabbitMQ container for the whole suite.
func TestMain(m *testing.M) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "rabbitmq:4.2-management-alpine",
		ExposedPorts: []string{"5672/tcp"},
		WaitingFor:   wait.ForLog("Server startup complete").WithStartupTimeout(180 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		log.Fatalf("start rabbitmq container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		log.Fatalf("get container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5672")
	if err != nil {
		log.Fatalf("get container port: %v", err)
	}

	sharedURL = "amqp://guest:guest@" + host + ":" + port.Port() + "/"

	code := m.Run()

	if err := container.Terminate(ctx); err != nil {
		log.Printf("warn: terminate container: %v", err)
	}

	os.Exit(code)
}

// newClient creates a rabbitmq.Client for a test and registers cleanup.
func newClient(t *testing.T, cfg *rabbitmq.Config) *rabbitmq.Client {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client, err := rabbitmq.NewClient(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	return client
}

// defaultConfig returns a Config wired to the shared container URL.
func defaultConfig(name string) *rabbitmq.Config {
	cfg := rabbitmq.NewDefaultConfig()
	cfg.URL = sharedURL
	cfg.ConnectionName = name
	cfg.QueueType = rabbitmq.QueueTypeClassic // classic queues work without quorum node count
	cfg.ReconnectMaxInterval = 5 * time.Second
	return cfg
}

// OrderEvent is a sample domain message used across tests.
type OrderEvent struct {
	OrderID string    `json:"order_id"`
	Status  string    `json:"status"`
	At      time.Time `json:"at"`
}

// ---- Tests -----------------------------------------------------------------

func TestClientConnects(t *testing.T) {
	client := newClient(t, defaultConfig("test-connect"))
	assert.True(t, client.IsHealthy())
	assert.Equal(t, rabbitmq.StateConnected, client.GetState())
}

func TestPublishAndConsume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := newClient(t, defaultConfig("test-pub-sub"))
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Declare an isolated queue for this test.
	tm := rabbitmq.NewTopologyManager(client)
	queueName := fmt.Sprintf("test-pub-sub-%d", time.Now().UnixNano())
	_, err := tm.DeclareQueue(rabbitmq.QueueConfig{Name: queueName, Durable: false})
	require.NoError(t, err)

	producer := rabbitmq.NewProducer(client)
	consumer := rabbitmq.NewConsumer(client, logger)

	event := OrderEvent{OrderID: "ord-1", Status: "placed", At: time.Now()}

	received := make(chan OrderEvent, 1)

	go func() {
		consCtx, consCancel := context.WithTimeout(ctx, 20*time.Second)
		defer consCancel()

		handler := func(ctx context.Context, msg interface{}, _ amqp.Delivery) error {
			b, _ := json.Marshal(msg)
			var e OrderEvent
			if err := json.Unmarshal(b, &e); err != nil {
				return err
			}
			select {
			case received <- e:
			default:
			}
			cancel() // signal done
			return nil
		}
		consumer.ConsumeWithDefaults(consCtx, queueName, handler) //nolint:errcheck
	}()

	time.Sleep(500 * time.Millisecond) // let consumer register

	require.NoError(t, producer.SendMessage(ctx, queueName, event))

	select {
	case got := <-received:
		assert.Equal(t, event.OrderID, got.OrderID)
		assert.Equal(t, event.Status, got.Status)
	case <-time.After(15 * time.Second):
		t.Fatal("timeout: message not received")
	}
}

func TestPublishWithHeaders(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := newClient(t, defaultConfig("test-headers"))
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	tm := rabbitmq.NewTopologyManager(client)
	queueName := fmt.Sprintf("test-headers-%d", time.Now().UnixNano())
	_, err := tm.DeclareQueue(rabbitmq.QueueConfig{Name: queueName, Durable: false})
	require.NoError(t, err)

	producer := rabbitmq.NewProducer(client)
	consumer := rabbitmq.NewConsumer(client, logger)

	headerReceived := make(chan string, 1)

	go func() {
		consCtx, consCancel := context.WithTimeout(ctx, 20*time.Second)
		defer consCancel()

		handler := func(ctx context.Context, _ interface{}, d amqp.Delivery) error {
			if v, ok := d.Headers["x-source"].(string); ok {
				select {
				case headerReceived <- v:
				default:
				}
			}
			cancel()
			return nil
		}
		consumer.ConsumeWithDefaults(consCtx, queueName, handler) //nolint:errcheck
	}()

	time.Sleep(500 * time.Millisecond)

	err = producer.SendMessageWithHeaders(ctx, queueName, OrderEvent{OrderID: "ord-2"}, map[string]interface{}{
		"x-source": "integration-test",
	})
	require.NoError(t, err)

	select {
	case src := <-headerReceived:
		assert.Equal(t, "integration-test", src)
	case <-time.After(15 * time.Second):
		t.Fatal("timeout: header message not received")
	}
}

func TestTopicExchangeRouting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := newClient(t, defaultConfig("test-topic"))
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	tm := rabbitmq.NewTopologyManager(client)
	exchangeName := "orders.topic." + suffix
	queueName := "orders.placed." + suffix

	require.NoError(t, tm.DeclareExchange(rabbitmq.ExchangeConfig{
		Name:    exchangeName,
		Kind:    "topic",
		Durable: false,
	}))
	_, err := tm.DeclareQueue(rabbitmq.QueueConfig{Name: queueName, Durable: false})
	require.NoError(t, err)
	require.NoError(t, tm.BindQueue(rabbitmq.BindingConfig{
		QueueName:  queueName,
		Exchange:   exchangeName,
		RoutingKey: "orders.placed",
	}))

	producer := rabbitmq.NewProducer(client)
	consumer := rabbitmq.NewConsumer(client, logger)

	received := make(chan struct{}, 1)

	go func() {
		consCtx, consCancel := context.WithTimeout(ctx, 20*time.Second)
		defer consCancel()

		handler := func(_ context.Context, _ interface{}, _ amqp.Delivery) error {
			select {
			case received <- struct{}{}:
			default:
			}
			cancel()
			return nil
		}
		consumer.ConsumeWithDefaults(consCtx, queueName, handler) //nolint:errcheck
	}()

	time.Sleep(500 * time.Millisecond)

	// This message should be routed to the queue.
	require.NoError(t, producer.SendToTopic(ctx, exchangeName, "orders.placed", OrderEvent{OrderID: "ord-3"}))
	// This message should NOT be routed (different key).
	require.NoError(t, producer.SendToTopic(ctx, exchangeName, "orders.cancelled", OrderEvent{OrderID: "ord-4"}))

	select {
	case <-received:
		// success
	case <-time.After(15 * time.Second):
		t.Fatal("timeout: topic message not received")
	}
}

func TestManualAckOnFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := defaultConfig("test-manual-ack")
	cfg.AutoAck = false

	client := newClient(t, cfg)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	tm := rabbitmq.NewTopologyManager(client)
	queueName := fmt.Sprintf("test-ack-%d", time.Now().UnixNano())
	_, err := tm.DeclareQueue(rabbitmq.QueueConfig{Name: queueName, Durable: false})
	require.NoError(t, err)

	producer := rabbitmq.NewProducer(client)
	require.NoError(t, producer.SendMessage(ctx, queueName, OrderEvent{OrderID: "ack-test"}))

	consumer := rabbitmq.NewConsumer(client, logger)
	var callCount atomic.Int32

	go func() {
		consCtx, consCancel := context.WithTimeout(ctx, 10*time.Second)
		defer consCancel()

		handler := func(_ context.Context, _ interface{}, _ amqp.Delivery) error {
			callCount.Add(1)
			return fmt.Errorf("simulated failure") // triggers nack + requeue
		}
		consumer.ConsumeWithDefaults(consCtx, queueName, handler) //nolint:errcheck
	}()

	// Wait for at least two deliveries — proves nack+requeue is working.
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout: message was not redelivered")
		default:
			if callCount.Load() >= 2 {
				cancel()
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func TestDeadLetterExchange(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := newClient(t, defaultConfig("test-dlx"))

	tm := rabbitmq.NewTopologyManager(client)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	mainQueue := "orders.main." + suffix
	dlxName := "orders.dlx." + suffix

	require.NoError(t, tm.SetupDeadLetterExchange(mainQueue, dlxName, 1000 /* 1 s TTL */))
	_, err := tm.SetupQueueWithDLX(mainQueue, dlxName)
	require.NoError(t, err)

	producer := rabbitmq.NewProducer(client)
	err = producer.SendMessage(ctx, mainQueue, OrderEvent{OrderID: "dlx-test", Status: "pending"})
	require.NoError(t, err)
}

func TestPublishBatch(t *testing.T) {
	ctx := context.Background()

	client := newClient(t, defaultConfig("test-batch"))
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	tm := rabbitmq.NewTopologyManager(client)
	queueName := fmt.Sprintf("test-batch-%d", time.Now().UnixNano())
	_, err := tm.DeclareQueue(rabbitmq.QueueConfig{Name: queueName, Durable: false})
	require.NoError(t, err)

	producer := rabbitmq.NewProducer(client)

	payloads := make([]interface{}, 5)
	for i := range payloads {
		payloads[i] = OrderEvent{OrderID: fmt.Sprintf("batch-%d", i)}
	}

	opts := rabbitmq.PublishOptions{RoutingKey: queueName}
	require.NoError(t, producer.PublishBatch(ctx, opts, payloads))

	// Verify all 5 are in the queue by consuming them.
	consumer := rabbitmq.NewConsumer(client, logger)
	consCtx, consCancel := context.WithTimeout(ctx, 15*time.Second)
	defer consCancel()

	var count atomic.Int32
	go func() {
		handler := func(_ context.Context, _ interface{}, _ amqp.Delivery) error {
			if count.Add(1) >= 5 {
				consCancel()
			}
			return nil
		}
		consumer.ConsumeWithDefaults(consCtx, queueName, handler) //nolint:errcheck
	}()

	<-consCtx.Done()
	assert.EqualValues(t, 5, count.Load())
}

func TestClientInvalidURL(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := rabbitmq.NewDefaultConfig()
	cfg.URL = ""

	_, err := rabbitmq.NewClient(cfg, logger)
	require.Error(t, err)
	assert.ErrorIs(t, err, rabbitmq.ErrURLRequired)
}
