package kafka

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	kafkago "github.com/segmentio/kafka-go"
)

func CreateDialer(cfg *SecurityConfig) (*kafkago.Dialer, error) {
	dialer := &kafkago.Dialer{
		Timeout:   10 * 1000000000,
		DualStack: true,
	}

	if cfg.Enabled {
		tlsConfig, err := loadTLSConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrTLSConfiguration, err)
		}
		dialer.TLS = tlsConfig
	}

	return dialer, nil
}

func loadTLSConfig(cfg *SecurityConfig) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS key pair: %w", err)
	}

	caCert, err := os.ReadFile(cfg.TLSCAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}
