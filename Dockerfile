FROM golang:latest AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

# Install Delve in the builder stage
RUN go install github.com/go-delve/delve/cmd/dlv@latest

COPY . .

# IMPORTANT: Build with optimizations disabled (-N -l)
RUN CGO_ENABLED=0 GOOS=linux go build -gcflags="all=-N -l" -o main ./cmd/myapp

FROM alpine:latest

# Use --no-cache to keep the image small
RUN apk add --no-cache libc6-compat

WORKDIR /app

# Copy binary AND the dlv tool from builder
COPY --from=builder /app/main .
COPY --from=builder /go/bin/dlv /usr/local/bin/dlv
COPY --from=builder /app/db/migrations ./db/migrations

# Expose App ports and Delve port (40000)
EXPOSE 8080 9090 40000

# Run Delve in headless mode, which then starts your app
CMD ["dlv", "--listen=:40000", "--headless=true", "--api-version=2", "--accept-multiclient", "exec", "./main", "--continue"]