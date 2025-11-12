package main

import (
	"fmt"
	"gosdk/cfg"
	"gosdk/pkg/logger"
	"log"
)

func main() {
	config, errCfg := cfg.Load()
	if errCfg != nil {
		log.Fatal(errCfg)
	}
	zlog := logger.NewZeroLog(config.AppEnv)
	fmt.Println(zlog)
}
