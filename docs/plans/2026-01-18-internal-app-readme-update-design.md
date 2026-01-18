# Internal App README Update Design

## Overview
Update `internal/app/README.md` to reflect the current state of `server.go` and `routes.go`.

## Changes

### 1. server.go Section
Replace the outdated `Server` struct and `NewServer` function with the current implementation which includes:
- `kafkaClient`
- `messageBrokerSvc`
- `messageBrokerHandler`
- Initialization of Event subsystem
- Wiring of `authService`

### 2. routes.go Section
Replace placeholders with actual route setup functions:
- `setupInfraRoutes` (Health checks, metrics, swagger)
- `setupAuthRoutes` (Login, logout, callback)
- `setupMessageBrokerRoutes` (Publish, subscribe)

## Verification
- Visual inspection of the generated README.md to ensure it matches the code.
