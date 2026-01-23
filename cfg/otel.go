package cfg

type OtelConfig struct {
	OTLPEndpoint string
	ServiceName  string
	SamplerRatio float64
}

func (l *Loader) loadOtel() OtelConfig {
	return OtelConfig{
		OTLPEndpoint: l.requireEnv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		ServiceName:  l.requireEnv("OTEL_SERVICE_NAME"),
		SamplerRatio: l.requireFloat64("OTEL_SAMPLER_RATIO"),
	}
}
