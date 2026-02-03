package kafka

import (
	"context"
	"errors"
	"time"

	"github.com/sony/gobreaker"
)

var ErrCircuitBreakerOpen = errors.New("circuit breaker is open")

type CircuitBreakerSettings struct {
	FailureThreshold int
	SuccessThreshold int
	Timeout          time.Duration
	HalfOpenMaxCalls int
}

func DefaultCircuitBreakerSettings() CircuitBreakerSettings {
	return CircuitBreakerSettings{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		HalfOpenMaxCalls: 3,
	}
}

type CircuitBreaker struct {
	breaker *gobreaker.CircuitBreaker
}

func NewCircuitBreaker(settings CircuitBreakerSettings) *CircuitBreaker {
	gobreakerSettings := gobreaker.Settings{
		Name:        "kafka-producer",
		MaxRequests: uint32(settings.HalfOpenMaxCalls),
		Interval:    settings.Timeout,
		Timeout:     settings.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return uint32(counts.ConsecutiveFailures) >= uint32(settings.FailureThreshold)
		},
	}

	return &CircuitBreaker{
		breaker: gobreaker.NewCircuitBreaker(gobreakerSettings),
	}
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
	_, err := cb.breaker.Execute(func() (interface{}, error) {
		return nil, fn()
	})
	if err != nil {
		if err == gobreaker.ErrOpenState {
			return ErrCircuitBreakerOpen
		}
		return err
	}
	return nil
}

func (cb *CircuitBreaker) State() gobreaker.State {
	return cb.breaker.State()
}

type CircuitBreakerMiddleware struct {
	producer Producer
	breaker  *CircuitBreaker
}

func NewCircuitBreakerMiddleware(producer Producer, settings CircuitBreakerSettings) *CircuitBreakerMiddleware {
	return &CircuitBreakerMiddleware{
		producer: producer,
		breaker:  NewCircuitBreaker(settings),
	}
}

func (m *CircuitBreakerMiddleware) Publish(ctx context.Context, msg Message) error {
	err := m.breaker.Execute(func() error {
		return m.producer.Publish(ctx, msg)
	})
	return err
}

func (m *CircuitBreakerMiddleware) Close() error {
	return m.producer.Close()
}

func (m *CircuitBreakerMiddleware) State() gobreaker.State {
	return m.breaker.State()
}
