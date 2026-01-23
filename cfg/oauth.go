package cfg

import (
	"errors"
	"time"
)

type Oauth2Config struct {
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectUrl  string
	GoogleLogoutUrl    string
	JWTSecret          string
	JWTExpiration      time.Duration
	StateTimeout       time.Duration
}

func (l *Loader) loadOAuth2() Oauth2Config {
	jwtSecret := l.requireEnv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		l.errs = append(l.errs, errors.New("JWT_SECRET must be at least 32 characters"))
	}

	return Oauth2Config{
		GoogleClientID:     l.requireEnv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: l.requireEnv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectUrl:  l.requireEnv("GOOGLE_REDIRECT_URL"),
		JWTSecret:          jwtSecret,
		JWTExpiration:      l.requireDuration("JWT_EXPIRATION"),
		StateTimeout:       l.requireDuration("STATE_TIMEOUT"),
	}
}
