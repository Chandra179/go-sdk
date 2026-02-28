package temporal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"go.temporal.io/sdk/worker"
)

// WorkerManager manages multiple Temporal workers for different task queues.
type WorkerManager struct {
	workers map[string]worker.Worker
	client  *Client
	config  *Config
	logger  *slog.Logger
	mu      sync.RWMutex
	started bool
}

// NewWorkerManager creates a new worker manager with the given client and configuration.
func NewWorkerManager(client *Client, config *Config) *WorkerManager {
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &WorkerManager{
		workers: make(map[string]worker.Worker),
		client:  client,
		config:  config,
		logger:  logger,
	}
}

// RegisterWorker registers a worker for a specific task queue with workflows and activities.
// This method must be called before Start().
func (wm *WorkerManager) RegisterWorker(
	taskQueue string,
	workflows []interface{},
	activities []interface{},
) error {
	if wm.started {
		return fmt.Errorf("cannot register worker after manager has started")
	}

	if taskQueue == "" {
		return fmt.Errorf("task queue name is required")
	}

	wm.mu.Lock()
	defer wm.mu.Unlock()

	if _, exists := wm.workers[taskQueue]; exists {
		return fmt.Errorf("worker for task queue %q already registered", taskQueue)
	}

	if wm.client == nil || wm.client.UnderlyingClient() == nil {
		return fmt.Errorf("temporal client is not initialized")
	}

	// Create worker with configuration
	w := worker.New(wm.client.UnderlyingClient(), taskQueue, worker.Options{
		MaxConcurrentActivityExecutionSize:     wm.config.MaxConcurrentActivityExecutionSize,
		MaxConcurrentWorkflowTaskExecutionSize: wm.config.MaxConcurrentWorkflowTaskExecutionSize,
	})

	// Register workflows
	for _, workflow := range workflows {
		w.RegisterWorkflow(workflow)
	}

	// Register activities
	for _, activity := range activities {
		w.RegisterActivity(activity)
	}

	wm.workers[taskQueue] = w
	wm.logger.Info("registered temporal worker", "task_queue", taskQueue, "workflows", len(workflows), "activities", len(activities))

	return nil
}

// Start starts all registered workers.
func (wm *WorkerManager) Start() error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if wm.started {
		return fmt.Errorf("worker manager is already started")
	}

	if len(wm.workers) == 0 {
		wm.logger.Warn("no workers registered, nothing to start")
		return nil
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(wm.workers))

	for taskQueue, w := range wm.workers {
		wg.Add(1)
		go func(queue string, worker worker.Worker) {
			defer wg.Done()
			wm.logger.Info("starting temporal worker", "task_queue", queue)
			if err := worker.Start(); err != nil {
				errChan <- fmt.Errorf("failed to start worker for task queue %q: %w", queue, err)
			}
		}(taskQueue, w)
	}

	// Wait for all workers to start (non-blocking since Start() is async)
	wg.Wait()
	close(errChan)

	// Check for any startup errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("worker startup errors: %v", errs)
	}

	wm.started = true
	wm.logger.Info("all temporal workers started", "count", len(wm.workers))

	return nil
}

// Stop gracefully stops all workers.
func (wm *WorkerManager) Stop(ctx context.Context) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if !wm.started {
		return nil
	}

	var wg sync.WaitGroup

	for taskQueue, w := range wm.workers {
		wg.Add(1)
		go func(queue string, wrk worker.Worker) {
			defer wg.Done()
			wm.logger.Info("stopping temporal worker", "task_queue", queue)
			wrk.Stop()
		}(taskQueue, w)
	}

	// Wait for all workers to stop with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		wm.logger.Info("all temporal workers stopped gracefully")
	case <-ctx.Done():
		wm.logger.Warn("timeout waiting for workers to stop, some may be forcefully terminated")
		return fmt.Errorf("worker stop timeout: %w", ctx.Err())
	}

	wm.started = false
	return nil
}

// IsStarted returns true if the worker manager has been started.
func (wm *WorkerManager) IsStarted() bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.started
}

// GetRegisteredTaskQueues returns a list of registered task queue names.
func (wm *WorkerManager) GetRegisteredTaskQueues() []string {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	queues := make([]string, 0, len(wm.workers))
	for queue := range wm.workers {
		queues = append(queues, queue)
	}
	return queues
}
