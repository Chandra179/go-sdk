# Go Template Project

This project is a Go template demonstrating reusable packages and runnable example services.

## Project Structure
```
├── cmd/                               # Runnable application
│   ├── myapp/                         # calling server.go
│       └── main.go
├── internal/                          # internal services
│   ├── app/                           
│	 │   ├── server.go                  # service init, dependency injection, etc..
│   └── auth/                          # example: package name auth
│	     ├── handler.go                 # endpoint handler using gin
│	     ├── service.go                 # service logic (business logic, query, etc..)
│	     ├── types.go                   # struct, const, etc..
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
├── cfg/                               # Centralized config files (YAML, JSON, HCL, os env)
│   └── config.go
├── k8s/                               # Kubernetes manifests (Deployment, Service, ConfigMap)
│   ├── deployment.yaml
│   ├── service.yaml
│   └── configmap.yaml
│── docker-compose.yml
│── Dockerfile
```

## Key Points
1. **`cmd/` folder**  
   - Each subdirectory represents a **separate runnable service or example**.  

2. **`pkg/` folder**  
   - Contains **reusable packages** for core functionality.