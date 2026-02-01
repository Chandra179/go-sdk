package kafka

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// CircuitState represents the state of the circuit breaker
type CircuitState int

const (
	StateClosed   CircuitState = iota // Normal operation
	StateOpen                         // Failing, reject requests
	StateHalfOpen                     // Testing if recovered
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ErrCircuitBreakerOpen is returned when the circuit breaker is open
var ErrCircuitBreakerOpen = errors.New("circuit breaker is open")

// CircuitBreakerConfig holds configuration for the circuit breaker
type CircuitBreakerConfig struct {
	FailureThreshold int           // Number of failures before opening circuit
	SuccessThreshold int           // Number of successes in half-open state to close circuit
	Timeout          time.Duration // Time before attempting to reset
	HalfOpenMaxCalls int           // Maximum calls in half-open state
}

// DefaultCircuitBreakerConfig returns sensible defaults
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		HalfOpenMaxCalls: 3,
	}
}

// CircuitBreaker implements the circuit breaker pattern for resilient operations
type CircuitBreaker struct {
	config          CircuitBreakerConfig
	state           CircuitState
	failures        int
	successes       int
	lastFailureTime time.Time
	halfOpenCalls   int
	mu              sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config: cfg,
		state:  StateClosed,
	}
}

// Execute runs the given function if the circuit allows it
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.allow() {
		return ErrCircuitBreakerOpen
	}

	err := fn()
	cb.recordResult(err)
	return err
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats returns current statistics about the circuit breaker
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:           cb.state.String(),
		Failures:        cb.failures,
		Successes:       cb.successes,
		LastFailureTime: cb.lastFailureTime,
	}
}

// CircuitBreakerStats holds statistics about the circuit breaker
type CircuitBreakerStats struct {
	State           string    `json:"state"`
	Failures        int       `json:"failures"`
	Successes       int       `json:"successes"`
	LastFailureTime time.Time `json:"last_failure_time,omitempty"`
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenCalls = 0
}

func (cb *CircuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if we should transition to half-open
		if time.Since(cb.lastFailureTime) > cb.config.Timeout {
			cb.state = StateHalfOpen
			cb.failures = 0
			cb.successes = 0
			cb.halfOpenCalls = 0
			return true
		}
		return false
	case StateHalfOpen:
		return cb.halfOpenCalls < cb.config.HalfOpenMaxCalls
	default:
		return false
	}
}

func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.halfOpenCalls++
	}

	if err == nil {
		cb.handleSuccess()
	} else {
		cb.handleFailure()
	}
}

func (cb *CircuitBreaker) handleSuccess() {
	switch cb.state {
	case StateClosed:
		// Reset failures on success in closed state
		if cb.failures > 0 {
			cb.failures--
		}
	case StateHalfOpen:
		cb.successes++
		// Transition to closed if we have enough successes
		if cb.successes >= cb.config.SuccessThreshold {
			cb.state = StateClosed
			cb.failures = 0
			cb.successes = 0
			cb.halfOpenCalls = 0
		}
	}
}

func (cb *CircuitBreaker) handleFailure() {
	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.config.FailureThreshold {
			cb.state = StateOpen
		}
	case StateHalfOpen:
		// Immediately open on failure in half-open state
		cb.state = StateOpen
		cb.successes = 0
	}
}

// CircuitBreakerMiddleware wraps a Producer with circuit breaker protection
type CircuitBreakerMiddleware struct {
	producer Producer
	breaker  *CircuitBreaker
}

// NewCircuitBreakerMiddleware creates a new circuit breaker middleware
func NewCircuitBreakerMiddleware(producer Producer, cfg CircuitBreakerConfig) *CircuitBreakerMiddleware {
	return &CircuitBreakerMiddleware{
		producer: producer,
		breaker:  NewCircuitBreaker(cfg),
	}
}

// Publish implements the Producer interface with circuit breaker protection
func (m *CircuitBreakerMiddleware) Publish(ctx context.Context, msg Message) error {
	err := m.breaker.Execute(func() error {
		return m.producer.Publish(ctx, msg)
	})

	if err == ErrCircuitBreakerOpen {
		return fmt.Errorf("%w: kafka producer circuit breaker is open", ErrKafkaPublish)
	}

	return err
}

// Close implements the Producer interface
func (m *CircuitBreakerMiddleware) Close() error {
	return m.producer.Close()
}

// Stats returns the current circuit breaker statistics
func (m *CircuitBreakerMiddleware) Stats() CircuitBreakerStats {
	return m.breaker.Stats()
}

// State returns the current circuit breaker state
func (m *CircuitBreakerMiddleware) State() CircuitState {
	return m.breaker.State()
}
