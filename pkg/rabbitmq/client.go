package rabbitmq

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ConnectionState represents the current state of the connection
type ConnectionState int32

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
	StateClosing
)

// Client manages RabbitMQ connections with auto-reconnection
type Client struct {
	config *Config
	logger *slog.Logger

	// Connections
	publisherConn *amqp.Connection
	consumerConn  *amqp.Connection

	// Channel pools
	publisherChannels chan *amqp.Channel
	consumerChannels  chan *amqp.Channel

	// State management
	state       atomic.Int32
	closeChan   chan struct{}
	reconnectWg sync.WaitGroup

	// Topology cache for reconnection
	topology *TopologyCache

	mu sync.RWMutex
}

// TopologyCache stores declared queues, exchanges, and bindings for reconnection
type TopologyCache struct {
	queues    []QueueConfig
	exchanges []ExchangeConfig
	bindings  []BindingConfig
	mu        sync.RWMutex
}

// NewClient creates a new RabbitMQ client with auto-reconnection
func NewClient(config *Config, logger *slog.Logger) (*Client, error) {
	if config == nil {
		config = NewDefaultConfig()
	}

	if config.URL == "" {
		return nil, fmt.Errorf("rabbitmq URL is required")
	}

	// Ensure defaults for critical config values
	if config.ChannelPoolSize <= 0 {
		config.ChannelPoolSize = DefaultChannelPoolSize
	}
	if config.PrefetchCount <= 0 {
		config.PrefetchCount = DefaultPrefetchCount
	}

	client := &Client{
		config:            config,
		logger:            logger,
		publisherChannels: make(chan *amqp.Channel, config.ChannelPoolSize),
		consumerChannels:  make(chan *amqp.Channel, config.ChannelPoolSize),
		closeChan:         make(chan struct{}),
		topology: &TopologyCache{
			queues:    make([]QueueConfig, 0),
			exchanges: make([]ExchangeConfig, 0),
			bindings:  make([]BindingConfig, 0),
		},
	}

	client.state.Store(int32(StateDisconnected))

	// Initial connection
	if err := client.connect(); err != nil {
		return nil, fmt.Errorf("initial connection failed: %w", err)
	}

	// Start reconnection monitor
	client.reconnectWg.Add(1)
	go client.reconnectionMonitor()

	return client, nil
}

// connect establishes initial connections
func (c *Client) connect() error {
	c.state.Store(int32(StateConnecting))

	// Connect publisher
	pubConn, err := c.dial("publisher")
	if err != nil {
		return fmt.Errorf("publisher connection failed: %w", err)
	}
	c.publisherConn = pubConn

	// Connect consumer
	consConn, err := c.dial("consumer")
	if err != nil {
		c.publisherConn.Close()
		return fmt.Errorf("consumer connection failed: %w", err)
	}
	c.consumerConn = consConn

	// Clear any old channels from pools before reinitializing
	c.clearChannelPools()

	// Initialize channel pools
	if err := c.initializeChannelPools(); err != nil {
		c.publisherConn.Close()
		c.consumerConn.Close()
		return fmt.Errorf("channel pool initialization failed: %w", err)
	}

	c.state.Store(int32(StateConnected))
	c.logger.Info("rabbitmq connected successfully",
		"connection_name", c.config.ConnectionName,
	)

	return nil
}

// dial creates a new connection with the given name
func (c *Client) dial(connType string) (*amqp.Connection, error) {
	config := amqp.Config{
		Heartbeat: DefaultHeartbeatInterval,
		Locale:    "en_US",
		Properties: amqp.Table{
			"connection_name": fmt.Sprintf("%s-%s", c.config.ConnectionName, connType),
		},
	}

	conn, err := amqp.DialConfig(c.config.URL, config)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// initializeChannelPools creates initial channel pools
func (c *Client) initializeChannelPools() error {
	// Initialize publisher channels with confirms
	for i := 0; i < c.config.ChannelPoolSize; i++ {
		ch, err := c.publisherConn.Channel()
		if err != nil {
			return fmt.Errorf("failed to create publisher channel: %w", err)
		}

		if c.config.PublisherConfirms {
			if err := ch.Confirm(false); err != nil {
				return fmt.Errorf("failed to enable publisher confirms: %w", err)
			}
		}

		c.publisherChannels <- ch
	}

	// Initialize consumer channels
	for i := 0; i < c.config.ChannelPoolSize; i++ {
		ch, err := c.consumerConn.Channel()
		if err != nil {
			return fmt.Errorf("failed to create consumer channel: %w", err)
		}
		c.consumerChannels <- ch
	}

	return nil
}

// reconnectionMonitor watches for connection closures and reconnects
func (c *Client) reconnectionMonitor() {
	defer c.reconnectWg.Done()

	for {
		select {
		case <-c.closeChan:
			return
		default:
			if c.state.Load() == int32(StateClosing) {
				return
			}

			// Check if connections are alive
			if !c.isConnected() {
				c.logger.Warn("rabbitmq connection lost, attempting reconnection")
				if err := c.reconnectWithBackoff(); err != nil {
					c.logger.Error("reconnection failed", "error", err)
				}
			}

			time.Sleep(1 * time.Second)
		}
	}
}

// reconnectWithBackoff attempts reconnection with exponential backoff
func (c *Client) reconnectWithBackoff() error {
	backoff := c.config.ReconnectInitialInterval
	maxBackoff := c.config.ReconnectMaxInterval

	for {
		select {
		case <-c.closeChan:
			return fmt.Errorf("client is closing")
		case <-time.After(backoff):
			// Check if client is closing before attempting reconnection
			if c.state.Load() == int32(StateClosing) {
				c.logger.Info("client closing, skipping reconnection")
				return nil
			}

			c.logger.Info("attempting reconnection", "backoff", backoff)

			if err := c.connect(); err != nil {
				c.logger.Error("reconnection attempt failed", "error", err, "next_backoff", backoff*2)

				// Exponential backoff
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}

			// Restore topology
			if err := c.restoreTopology(); err != nil {
				c.logger.Error("failed to restore topology", "error", err)
			}

			c.logger.Info("reconnection successful")
			return nil
		}
	}
}

// isConnected checks if both connections are alive
func (c *Client) isConnected() bool {
	// Check connections without holding mutex to avoid deadlock with Close()
	// which holds mutex while waiting for reconnectionMonitor to exit
	if c.publisherConn == nil || c.consumerConn == nil {
		return false
	}

	return !c.publisherConn.IsClosed() && !c.consumerConn.IsClosed()
}

// GetPublisherChannel returns a channel from the publisher pool
func (c *Client) GetPublisherChannel() (*amqp.Channel, error) {
	if !c.isConnected() {
		return nil, fmt.Errorf("client not connected")
	}

	select {
	case ch := <-c.publisherChannels:
		return ch, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout waiting for publisher channel")
	}
}

// ReturnPublisherChannel returns a channel to the publisher pool
func (c *Client) ReturnPublisherChannel(ch *amqp.Channel) {
	if ch == nil || ch.IsClosed() {
		return
	}

	select {
	case c.publisherChannels <- ch:
	default:
		// Pool is full, close the channel
		ch.Close()
	}
}

// GetConsumerChannel returns a channel from the consumer pool
func (c *Client) GetConsumerChannel() (*amqp.Channel, error) {
	if !c.isConnected() {
		return nil, fmt.Errorf("client not connected")
	}

	select {
	case ch := <-c.consumerChannels:
		return ch, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout waiting for consumer channel")
	}
}

// ReturnConsumerChannel returns a channel to the consumer pool
func (c *Client) ReturnConsumerChannel(ch *amqp.Channel) {
	if ch == nil || ch.IsClosed() {
		return
	}

	select {
	case c.consumerChannels <- ch:
	default:
		// Pool is full, close the channel
		ch.Close()
	}
}

// clearChannelPools drains and closes all channels in both pools
func (c *Client) clearChannelPools() {
	// Drain publisher channels
	drainChannelPool(c.publisherChannels)
	// Drain consumer channels
	drainChannelPool(c.consumerChannels)
}

// drainChannelPool drains and closes all channels in a pool
func drainChannelPool(pool chan *amqp.Channel) {
	for {
		select {
		case ch := <-pool:
			if ch != nil && !ch.IsClosed() {
				ch.Close()
			}
		default:
			// Pool is empty
			return
		}
	}
}

// Close gracefully closes all connections
func (c *Client) Close() error {
	c.state.Store(int32(StateClosing))
	close(c.closeChan)

	// Wait for reconnection monitor to stop
	c.reconnectWg.Wait()

	// Close channel pools
	close(c.publisherChannels)
	close(c.consumerChannels)

	for ch := range c.publisherChannels {
		ch.Close()
	}

	for ch := range c.consumerChannels {
		ch.Close()
	}

	// Close connections
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.publisherConn != nil && !c.publisherConn.IsClosed() {
		c.publisherConn.Close()
	}

	if c.consumerConn != nil && !c.consumerConn.IsClosed() {
		c.consumerConn.Close()
	}

	c.logger.Info("rabbitmq client closed")
	return nil
}

// IsHealthy returns true if the client is connected and healthy
func (c *Client) IsHealthy() bool {
	return c.state.Load() == int32(StateConnected) && c.isConnected()
}

// GetState returns the current connection state
func (c *Client) GetState() ConnectionState {
	return ConnectionState(c.state.Load())
}
