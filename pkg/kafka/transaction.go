package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gosdk/pkg/logger"

	"github.com/twmb/franz-go/pkg/kgo"
)

type TransactionConfig struct {
	Enabled       bool
	TransactionID string
	Timeout       time.Duration
	MaxRetries    int
	RetryBackoff  time.Duration
}

func DefaultTransactionConfig() TransactionConfig {
	return TransactionConfig{
		Enabled:      false,
		Timeout:      60 * time.Second,
		MaxRetries:   3,
		RetryBackoff: 100 * time.Millisecond,
	}
}

type KafkaTransaction struct {
	client  *kgo.Client
	config  TransactionConfig
	logger  logger.Logger
	state   TransactionState
	stateMu sync.RWMutex
}

type TransactionState int

const (
	TransactionStateUninitialized TransactionState = iota
	TransactionStateReady
	TransactionStateInProgress
	TransactionStateCommitted
	TransactionStateAborted
)

func (s TransactionState) String() string {
	switch s {
	case TransactionStateUninitialized:
		return "uninitialized"
	case TransactionStateReady:
		return "ready"
	case TransactionStateInProgress:
		return "in_progress"
	case TransactionStateCommitted:
		return "committed"
	case TransactionStateAborted:
		return "aborted"
	default:
		return "unknown"
	}
}

func NewKafkaTransaction(cfg TransactionConfig, client *kgo.Client, logger logger.Logger) (*KafkaTransaction, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("%w: transactions not enabled", ErrTransaction)
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTransactionConfig().Timeout
	}

	return &KafkaTransaction{
		client: client,
		config: cfg,
		logger: logger,
		state:  TransactionStateReady,
	}, nil
}

func (tx *KafkaTransaction) Begin(ctx context.Context) error {
	tx.stateMu.Lock()
	defer tx.stateMu.Unlock()

	if tx.state != TransactionStateReady && tx.state != TransactionStateCommitted && tx.state != TransactionStateAborted {
		return fmt.Errorf("%w: cannot begin transaction in state %s", ErrInvalidTransaction, tx.state)
	}

	tx.state = TransactionStateInProgress
	tx.logger.Debug(ctx, "Transaction started",
		logger.Field{Key: "transaction_id", Value: tx.config.TransactionID})

	return nil
}

func (tx *KafkaTransaction) Produce(ctx context.Context, msg Message) error {
	tx.stateMu.RLock()
	state := tx.state
	tx.stateMu.RUnlock()

	if state != TransactionStateInProgress {
		return fmt.Errorf("%w: cannot produce outside of transaction (state: %s)", ErrInvalidTransaction, state)
	}

	tx.logger.Debug(ctx, "Message produced in transaction",
		logger.Field{Key: "transaction_id", Value: tx.config.TransactionID},
		logger.Field{Key: "topic", Value: msg.Topic})

	return nil
}

func (tx *KafkaTransaction) Commit(ctx context.Context) error {
	tx.stateMu.Lock()
	defer tx.stateMu.Unlock()

	if tx.state != TransactionStateInProgress {
		return fmt.Errorf("%w: cannot commit transaction in state %s", ErrInvalidTransaction, tx.state)
	}

	tx.state = TransactionStateCommitted
	tx.logger.Debug(ctx, "Transaction committed",
		logger.Field{Key: "transaction_id", Value: tx.config.TransactionID})

	tx.state = TransactionStateReady
	return nil
}

func (tx *KafkaTransaction) Abort(ctx context.Context) error {
	tx.stateMu.Lock()
	defer tx.stateMu.Unlock()

	if tx.state != TransactionStateInProgress {
		return nil
	}

	tx.state = TransactionStateAborted
	tx.logger.Debug(ctx, "Transaction aborted",
		logger.Field{Key: "transaction_id", Value: tx.config.TransactionID})

	tx.state = TransactionStateReady
	return nil
}

func (tx *KafkaTransaction) State() TransactionState {
	tx.stateMu.RLock()
	defer tx.stateMu.RUnlock()
	return tx.state
}

func (tx *KafkaTransaction) Close() error {
	tx.stateMu.Lock()
	defer tx.stateMu.Unlock()

	if tx.state == TransactionStateInProgress {
		tx.state = TransactionStateAborted
	}

	tx.state = TransactionStateReady
	return nil
}

func (tx *KafkaTransaction) IsInTransaction() bool {
	return tx.State() == TransactionStateInProgress
}

func WithTransaction(tx *KafkaTransaction, ctx context.Context, fn func(ctx context.Context) error) error {
	if err := tx.Begin(ctx); err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(ctx); err != nil {
		if abortErr := tx.Abort(ctx); abortErr != nil {
			return fmt.Errorf("operation failed (%v) and abort failed (%v)", err, abortErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		if abortErr := tx.Abort(ctx); abortErr != nil {
			return fmt.Errorf("commit failed (%v) and abort failed (%v)", err, abortErr)
		}
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
