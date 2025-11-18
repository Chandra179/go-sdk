# Go Template Project

This project is a Go template demonstrating reusable packages and runnable example services.

## Project Structure

```
.
├── cmd/
│   ├── oauth2/       # Example service for OAuth2 flows
│   │   └── main.go   # Entry point for the OAuth2 service
│   ├── otel/         # Example service for OpenTelemetry setup
│   │   └── main.go   # Entry point for OTEL tracing & metrics demo
│   └── redis/        # Example service for Redis caching
│       └── main.go   # Entry point for Redis demo
├── pkg/              # Reusable library packages
│   ├── logger/       # ZeroLogger implementation
│   ├── oauth2/       # OAuth2 manager & handlers
│   └── otel/         # OpenTelemetry initialization and helpers
└── go.mod
```

## Key Points

1. **`cmd/` folder**  
   - Each subdirectory represents a **separate runnable service or example**.  
   - Demonstrates **service configuration and execution**.

2. **`pkg/` folder**  
   - Contains **reusable packages** for core functionality.  
   - Standard Go convention for libraries.  

3. **Usage Examples**  
   - Run the OAuth2 service:  
     ```bash
     go run ./cmd/oauth2
     ```  
   - Run the OTEL service:  
     ```bash
     go run ./cmd/otel
     ```  
   - Each service uses reusable logic from `pkg/`.