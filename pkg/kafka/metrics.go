package kafka

// DEPRECATED: This file is kept for backward compatibility reference.
// All metrics have been migrated to OpenTelemetry in otel_metrics.go
// Use InitOtelMetrics() and helper functions instead.

// RegisterMetrics is deprecated and does nothing.
// OTEL metrics are now auto-registered via InitOtelMetrics()
func RegisterMetrics() {
	// No-op - metrics now handled by OTEL
}
