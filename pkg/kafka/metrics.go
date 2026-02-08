package kafka

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds Prometheus metrics for Kafka operations.
type Metrics struct {
	messagesProduced *prometheus.CounterVec
	messagesConsumed *prometheus.CounterVec
	produceLatency   *prometheus.HistogramVec
	consumeLatency   *prometheus.HistogramVec
	consumerLag      *prometheus.GaugeVec
	dlqMessages      *prometheus.CounterVec
	errors           *prometheus.CounterVec
	handlerRetries   *prometheus.CounterVec
}

// NewMetrics creates a new Metrics instance and registers it with the provided registerer.
// If registerer is nil, prometheus.DefaultRegisterer is used.
func NewMetrics(registerer prometheus.Registerer) *Metrics {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	m := &Metrics{
		messagesProduced: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kafka",
				Subsystem: "producer",
				Name:      "messages_total",
				Help:      "Total number of messages produced",
			},
			[]string{"topic", "status"},
		),
		messagesConsumed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kafka",
				Subsystem: "consumer",
				Name:      "messages_total",
				Help:      "Total number of messages consumed",
			},
			[]string{"topic", "group", "status"},
		),
		produceLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "kafka",
				Subsystem: "producer",
				Name:      "latency_seconds",
				Help:      "Latency of produce operations",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"topic"},
		),
		consumeLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "kafka",
				Subsystem: "consumer",
				Name:      "handler_latency_seconds",
				Help:      "Latency of message handler execution",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"topic"},
		),
		consumerLag: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "kafka",
				Subsystem: "consumer",
				Name:      "lag",
				Help:      "Current consumer lag (difference between latest offset and committed offset)",
			},
			[]string{"topic", "partition", "group"},
		),
		dlqMessages: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kafka",
				Subsystem: "consumer",
				Name:      "dlq_messages_total",
				Help:      "Total number of messages sent to DLQ",
			},
			[]string{"topic", "reason"},
		),
		errors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kafka",
				Name:      "errors_total",
				Help:      "Total number of errors",
			},
			[]string{"type", "topic"},
		),
		handlerRetries: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kafka",
				Subsystem: "consumer",
				Name:      "handler_retries_total",
				Help:      "Total number of handler retries",
			},
			[]string{"topic"},
		),
	}

	// Register all metrics
	registerer.MustRegister(
		m.messagesProduced,
		m.messagesConsumed,
		m.produceLatency,
		m.consumeLatency,
		m.consumerLag,
		m.dlqMessages,
		m.errors,
		m.handlerRetries,
	)

	return m
}

// RecordProduce records a produce operation.
func (m *Metrics) RecordProduce(topic string, duration time.Duration, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	m.messagesProduced.WithLabelValues(topic, status).Inc()
	if success {
		m.produceLatency.WithLabelValues(topic).Observe(duration.Seconds())
	}
}

// RecordConsume records a consume operation.
func (m *Metrics) RecordConsume(topic, group string, duration time.Duration, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	m.messagesConsumed.WithLabelValues(topic, group, status).Inc()
	m.consumeLatency.WithLabelValues(topic).Observe(duration.Seconds())
}

// RecordLag records consumer lag for a partition.
func (m *Metrics) RecordLag(topic string, partition int32, group string, lag int64) {
	m.consumerLag.WithLabelValues(topic, partitionStr(partition), group).Set(float64(lag))
}

// RecordDLQ records a message sent to DLQ.
func (m *Metrics) RecordDLQ(topic, reason string) {
	m.dlqMessages.WithLabelValues(topic, reason).Inc()
}

// RecordError records an error.
func (m *Metrics) RecordError(errorType, topic string) {
	m.errors.WithLabelValues(errorType, topic).Inc()
}

// RecordRetry records a handler retry.
func (m *Metrics) RecordRetry(topic string) {
	m.handlerRetries.WithLabelValues(topic).Inc()
}

func partitionStr(p int32) string {
	// Simple int32 to string conversion
	if p == 0 {
		return "0"
	}

	var result []byte
	negative := p < 0
	if negative {
		p = -p
	}

	for p > 0 {
		result = append([]byte{byte('0' + p%10)}, result...)
		p /= 10
	}

	if negative {
		result = append([]byte{'-'}, result...)
	}

	return string(result)
}
