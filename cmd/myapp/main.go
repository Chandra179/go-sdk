package main

import (
	"fmt"
	"gosdk/cfg"
	"gosdk/pkg/cache"
	"gosdk/pkg/logger"
	"log"
	"net/http"
)

func main() {
	config, errCfg := cfg.Load()
	if errCfg != nil {
		log.Fatal(errCfg)
	}
	zlog := logger.NewZeroLog(config.AppEnv)
	fmt.Println(zlog)

	red := cache.NewRedisCache(config.Redis.Host + ":" + config.Redis.Port)
	fmt.Println(red)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello from dummy handler!")
	})
	fmt.Println("Server starting on :8080")
	http.ListenAndServe(":8080", nil) // This blocks forever
}
