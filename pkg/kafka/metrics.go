package kafka

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ProducerMessagesSent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_producer_messages_sent_total",
			Help: "Total number of messages sent to Kafka",
		},
		[]string{"topic", "compression"},
	)

	ProducerSendErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_producer_send_errors_total",
			Help: "Total number of send errors",
		},
		[]string{"topic", "error_type"},
	)

	ProducerSendLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kafka_producer_send_latency_seconds",
			Help:    "Time to send messages to Kafka",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"topic"},
	)

	ConsumerMessagesProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_consumer_messages_processed_total",
			Help: "Total number of messages processed",
		},
		[]string{"topic", "group_id"},
	)

	ConsumerProcessingErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_consumer_processing_errors_total",
			Help: "Total number of processing errors",
		},
		[]string{"topic", "error_type"},
	)

	ConsumerLag = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kafka_consumer_lag",
			Help: "Consumer lag by topic and partition",
		},
		[]string{"topic", "partition", "group_id"},
	)

	ConsumerRebalanceEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_consumer_rebalance_events_total",
			Help: "Total number of consumer group rebalance events",
		},
		[]string{"group_id", "event_type"},
	)

	DLQMessagesSent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_dlq_messages_sent_total",
			Help: "Total number of messages sent to DLQ",
		},
		[]string{"original_topic", "dlq_topic"},
	)

	RetryMessagesSent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_retry_messages_sent_total",
			Help: "Total number of messages sent to retry topic",
		},
		[]string{"original_topic", "retry_topic"},
	)
)

func RegisterMetrics() {
	prometheus.MustRegister(ProducerMessagesSent)
	prometheus.MustRegister(ProducerSendErrors)
	prometheus.MustRegister(ProducerSendLatency)
	prometheus.MustRegister(ConsumerMessagesProcessed)
	prometheus.MustRegister(ConsumerProcessingErrors)
	prometheus.MustRegister(ConsumerLag)
	prometheus.MustRegister(ConsumerRebalanceEvents)
	prometheus.MustRegister(DLQMessagesSent)
	prometheus.MustRegister(RetryMessagesSent)
}
