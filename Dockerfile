FROM golang:latest AS builder

WORKDIR /app

# Download dependencies first (faster builds)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/myapp

FROM alpine:latest

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/main .

# Copy migrations directory
COPY --from=builder /app/db/migrations ./db/migrations

# Expose app ports (HTTP and gRPC)
EXPOSE 8080 9090

# Run the server
CMD ["./main"]