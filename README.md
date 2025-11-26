# Go Template Project

This project is a Go template demonstrating reusable packages and runnable example services.

## Project Structure

```
├── cmd/                               # Runnable applications / entrypoints
│   ├── oauth2/                        # Example OAuth2 service
│   │   └── main.go
│   ├── otel/                          # OpenTelemetry demo service
│   │   └── main.go
│   ├── redis/                         # Redis caching example
│   │   └── main.go
│   ├── cache/                         # Cache service example
│   │   └── main.go
│   ├── db/                            # Database service example
│   │   └── main.go
│   ├── logger/                        # Logger example service
│   │   └── main.go
│   ├── openapi/                       # OpenAPI server demo
│   │   └── main.go
│   ├── myapp/                         # Complete application using all packages
│   │   └── main.go
│   ├── passkey/                       # Passkey webauthn examples
│   │   └── main.go
│   └── proto/                         # gRPC server/client examples
│       └── main.go
├── pkg/                               # Reusable library packages
│   ├── cache/                         # Cache interfaces, Redis helpers, wrappers
│   ├── db/                            # Database connectors, migrations, helpers
│   ├── logger/                        # Zerolog wrapper & helpers
│   ├── oauth2/                        # OAuth2 manager & token helpers
│   └── otel/                          # OpenTelemetry setup utilities
├── api/
│   └── proto/
│       ├── user/                      # Proto definitions
│       │   └── user.proto
│       └── gen/                       # Generated .pb.go & _grpc.pb.go (ignored by Git)
├── cfg/                               # Centralized config files (YAML, JSON, HCL)
│   ├── app.yaml
│   ├── db.yaml
│   └── redis.yaml
├── k8s/                               # Kubernetes manifests (Deployment, Service, ConfigMap)
│   ├── deployment.yaml
│   ├── service.yaml
│   └── configmap.yaml

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

## Proto
Before generating .proto files, install protoc and the required Go plugins. see
api/proto/Makefile for the installation

## DB
```
curl -L https://github.com/golang-migrate/migrate/releases/latest/download/migrate.linux-amd64.tar.gz | tar xvz
sudo mv migrate /usr/local/bin/

migrate create -ext sql -dir db/migrations -seq initial

migrate -database "postgres://user:pass@localhost:5432/mydb?sslmode=disable" \
        -path db/migrations up
migrate -database "postgres://user:pass@localhost:5432/mydb?sslmode=disable" \
        -path db/migrations down 1

```