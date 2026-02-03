package kafka

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

func CreateTLSConfig(cfg *SecurityConfig) (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS key pair: %w", err)
	}

	caCert, err := os.ReadFile(cfg.TLSCAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func CreateDialerOptions(cfg *SecurityConfig) ([]kgo.Opt, error) {
	var opts []kgo.Opt

	if cfg.Enabled {
		tlsConfig, err := CreateTLSConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrTLSConfiguration, err)
		}
		opts = append(opts, kgo.DialTLSConfig(tlsConfig))
	}

	return opts, nil
}

func CreateDialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout:   timeout,
		DualStack: true,
	}
}
