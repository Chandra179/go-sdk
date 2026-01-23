package cfg

type RedisConfig struct {
	Host     string
	Port     string
	Password string
}

func (l *Loader) loadRedis() RedisConfig {
	return RedisConfig{
		Host:     l.requireEnv("REDIS_HOST"),
		Port:     l.requireEnv("REDIS_PORT"),
		Password: l.requireEnv("REDIS_PASSWORD"),
	}
}
