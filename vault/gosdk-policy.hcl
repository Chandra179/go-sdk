# Vault Policy for go-sdk Application
# This policy grants read access to all application secrets

# Read access to Postgres secrets
path "secret/go-sdk/postgres" {
  capabilities = ["read"]
}

# Read access to OAuth2/JWT secrets
path "secret/go-sdk/oauth2" {
  capabilities = ["read"]
}

# Read access to Redis secrets
path "secret/go-sdk/redis" {
  capabilities = ["read"]
}

# Read access to RabbitMQ secrets
path "secret/go-sdk/rabbitmq" {
  capabilities = ["read"]
}

# Read access to Kafka secrets
path "secret/go-sdk/kafka" {
  capabilities = ["read"]
}

# Read access to Temporal secrets
path "secret/go-sdk/temporal" {
  capabilities = ["read"]
}

# List access to the go-sdk metadata
path "secret/go-sdk" {
  capabilities = ["list"]
}

# Read access to system health (for health checks)
path "sys/health" {
  capabilities = ["read"]
}

# Read access to seal status
path "sys/seal-status" {
  capabilities = ["read"]
}
