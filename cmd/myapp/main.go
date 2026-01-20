package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gosdk/cfg"
	"gosdk/internal/app"
)

func main() {
	config, err := cfg.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := app.NewServer(ctx, config)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}
	defer server.Shutdown(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Received shutdown signal")
		cancel()
	}()

	if err := server.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
