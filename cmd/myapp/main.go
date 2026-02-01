package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gosdk/cfg"
	"gosdk/internal/app"

	// Import swagger docs - the init() function registers the swagger spec
	_ "gosdk/api"
)

func main() {
	config, err := cfg.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize the Provider (dependency injection container)
	// This sets up all infrastructure and services
	provider, err := app.NewProvider(ctx, config)
	if err != nil {
		log.Fatalf("Failed to initialize provider: %v", err)
	}

	// Create the HTTP Server using the Provider
	server, err := app.NewServer(provider)
	if err != nil {
		provider.Infra.Close(ctx) // Clean up infrastructure on failure
		log.Fatalf("Failed to create server: %v", err)
	}

	// Setup graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Received shutdown signal, initiating graceful shutdown...")
		cancel()
	}()

	// Run server in a goroutine so we can handle shutdown
	errCh := make(chan error, 1)
	go func() {
		if err := server.Run(); err != nil {
			errCh <- err
		}
	}()

	// Wait for either shutdown signal or server error
	select {
	case <-ctx.Done():
		// Graceful shutdown triggered by signal
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		// Shutdown HTTP server first
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}

		// Then close infrastructure resources
		if err := provider.Infra.Close(shutdownCtx); err != nil {
			log.Printf("Infrastructure shutdown error: %v", err)
		}

		log.Println("Shutdown complete")

	case err := <-errCh:
		// Server encountered an error
		log.Fatalf("Server error: %v", err)
	}
}
