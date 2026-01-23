package cfg

import "time"

type HTTPServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func (l *Loader) loadHTTPServer() HTTPServerConfig {
	return HTTPServerConfig{
		Port:         l.requireEnv("HTTP_PORT"),
		ReadTimeout:  l.requireDuration("HTTP_READ_TIMEOUT"),
		WriteTimeout: l.requireDuration("HTTP_WRITE_TIMEOUT"),
	}
}
