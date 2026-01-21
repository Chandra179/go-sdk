package bootstrap

import (
	"gosdk/pkg/cache"
)

func InitCache(host, port string) cache.Cache {
	addr := host + ":" + port
	return cache.NewRedisCache(addr)
}
