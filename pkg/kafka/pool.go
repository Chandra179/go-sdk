package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// ConnectionPool manages a pool of Kafka connections for better resource utilization
type ConnectionPool struct {
	dialer      *kafkago.Dialer
	maxConns    int
	idleTimeout time.Duration
	conns       chan *PooledConn
	mu          sync.RWMutex
	closed      bool
}

// PooledConn wraps a kafka connection with metadata
type PooledConn struct {
	conn     *kafkago.Conn
	pool     *ConnectionPool
	lastUsed time.Time
	network  string
	address  string
}

// ConnectionPoolConfig holds configuration for the connection pool
type ConnectionPoolConfig struct {
	MaxConnections int           // Maximum number of connections in the pool
	IdleTimeout    time.Duration // How long to keep idle connections
}

// DefaultConnectionPoolConfig returns default pool configuration
func DefaultConnectionPoolConfig() ConnectionPoolConfig {
	return ConnectionPoolConfig{
		MaxConnections: 10,
		IdleTimeout:    5 * time.Minute,
	}
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(dialer *kafkago.Dialer, cfg ConnectionPoolConfig) *ConnectionPool {
	if cfg.MaxConnections <= 0 {
		cfg.MaxConnections = 10
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 5 * time.Minute
	}

	return &ConnectionPool{
		dialer:      dialer,
		maxConns:    cfg.MaxConnections,
		idleTimeout: cfg.IdleTimeout,
		conns:       make(chan *PooledConn, cfg.MaxConnections),
	}
}

// Acquire gets a connection from the pool or creates a new one
func (p *ConnectionPool) Acquire(ctx context.Context, network, address string) (*PooledConn, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, fmt.Errorf("connection pool is closed")
	}
	p.mu.RUnlock()

	// Try to get an existing connection from the pool
	select {
	case pooledConn := <-p.conns:
		// Check if connection is still valid and matches the requested address
		if pooledConn.network == network && pooledConn.address == address {
			if time.Since(pooledConn.lastUsed) < p.idleTimeout {
				pooledConn.lastUsed = time.Now()
				return pooledConn, nil
			}
		}
		// Connection is expired or doesn't match, close it
		pooledConn.conn.Close()
	default:
		// No connection available, create new one
	}

	// Create new connection
	conn, err := p.dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s %s: %w", network, address, err)
	}

	return &PooledConn{
		conn:     conn,
		pool:     p,
		lastUsed: time.Now(),
		network:  network,
		address:  address,
	}, nil
}

// Release returns a connection to the pool
func (p *PooledConn) Release() error {
	if p.pool == nil || p.conn == nil {
		return nil
	}

	p.pool.mu.RLock()
	if p.pool.closed {
		p.pool.mu.RUnlock()
		return p.conn.Close()
	}
	p.pool.mu.RUnlock()

	// Try to return to pool
	select {
	case p.pool.conns <- p:
		// Successfully returned to pool
		return nil
	default:
		// Pool is full, close the connection
		return p.conn.Close()
	}
}

// Close closes the pooled connection
func (p *PooledConn) Close() error {
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// Conn returns the underlying connection
func (p *PooledConn) Conn() *kafkago.Conn {
	return p.conn
}

// Close closes the connection pool and all its connections
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	close(p.conns)

	var errs []error
	for pooledConn := range p.conns {
		if err := pooledConn.conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connections: %v", errs)
	}
	return nil
}

// Stats returns pool statistics
func (p *ConnectionPool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return PoolStats{
		MaxConnections: p.maxConns,
		IdleTimeout:    p.idleTimeout,
		Closed:         p.closed,
	}
}

// PoolStats contains connection pool statistics
type PoolStats struct {
	AvailableConnections int
	MaxConnections       int
	IdleTimeout          time.Duration
	Closed               bool
}

// ConnectionManager manages multiple connection pools for different brokers
type ConnectionManager struct {
	pools  map[string]*ConnectionPool
	dialer *kafkago.Dialer
	cfg    ConnectionPoolConfig
	mu     sync.RWMutex
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(dialer *kafkago.Dialer, cfg ConnectionPoolConfig) *ConnectionManager {
	return &ConnectionManager{
		pools:  make(map[string]*ConnectionPool),
		dialer: dialer,
		cfg:    cfg,
	}
}

// GetPool gets or creates a connection pool for a specific broker
func (cm *ConnectionManager) GetPool(broker string) *ConnectionPool {
	cm.mu.RLock()
	pool, exists := cm.pools[broker]
	cm.mu.RUnlock()

	if exists {
		return pool
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Double-check after acquiring write lock
	if pool, exists = cm.pools[broker]; exists {
		return pool
	}

	pool = NewConnectionPool(cm.dialer, cm.cfg)
	cm.pools[broker] = pool
	return pool
}

// Close closes all connection pools
func (cm *ConnectionManager) Close() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var errs []error
	for broker, pool := range cm.pools {
		if err := pool.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close pool for %s: %w", broker, err))
		}
		delete(cm.pools, broker)
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connection pools: %v", errs)
	}
	return nil
}
