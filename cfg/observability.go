package cfg

type ObservabilityConfig struct {
	OTLPEndpoint string
	ServiceName  string
	SamplerRatio float64
}

func (l *Loader) loadObservability() ObservabilityConfig {
	return ObservabilityConfig{
		OTLPEndpoint: l.requireEnv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		ServiceName:  l.requireEnv("OTEL_SERVICE_NAME"),
		SamplerRatio: l.requireFloat64("OTEL_SAMPLER_RATIO"),
	}
}
