# Vault Agent Configuration for go-sdk
# This agent runs as a sidecar and fetches secrets from Vault,
# writing them to files that the application can read.

# Auto-auth configuration for local development
# In production, use Kubernetes auth or AppRole
auto_auth {
  method "token_file" {
    config = {
      token_file_path = "/vault/token/root-token"
    }
  }

  sink "file" {
    config = {
      path = "/vault/token/vault-agent-token"
    }
  }
}

# Vault connection
vault {
  address = "http://vault:8200"
}

# Template for Postgres secrets
template {
  destination = "/vault/secrets/postgres.env"
  command     = "echo 'Postgres secrets updated'"
  contents    = <<EOT
{{ with secret "secret/go-sdk/postgres" }}
POSTGRES_PASSWORD={{ .Data.data.password }}
{{ end }}
EOT
}

# Template for OAuth2/JWT secrets
template {
  destination = "/vault/secrets/oauth2.env"
  command     = "echo 'OAuth2 secrets updated'"
  contents    = <<EOT
{{ with secret "secret/go-sdk/oauth2" }}
GOOGLE_CLIENT_ID={{ .Data.data.client_id }}
GOOGLE_CLIENT_SECRET={{ .Data.data.client_secret }}
JWT_SECRET={{ .Data.data.jwt_secret }}
{{ end }}
EOT
}

# Template for Redis secrets
template {
  destination = "/vault/secrets/redis.env"
  command     = "echo 'Redis secrets updated'"
  contents    = <<EOT
{{ with secret "secret/go-sdk/redis" }}
REDIS_PASSWORD={{ .Data.data.password }}
{{ end }}
EOT
}

# Template for RabbitMQ secrets
template {
  destination = "/vault/secrets/rabbitmq.env"
  command     = "echo 'RabbitMQ secrets updated'"
  contents    = <<EOT
{{ with secret "secret/go-sdk/rabbitmq" }}
RABBITMQ_URL={{ .Data.data.url }}
{{ end }}
EOT
}

# Template for Kafka secrets
template {
  destination = "/vault/secrets/kafka.env"
  command     = "echo 'Kafka secrets updated'"
  contents    = <<EOT
{{ with secret "secret/go-sdk/kafka" }}
KAFKA_TLS_CERT_FILE=/vault/secrets/kafka_cert.pem
KAFKA_TLS_KEY_FILE=/vault/secrets/kafka_key.pem
KAFKA_TLS_CA_FILE=/vault/secrets/kafka_ca.pem
{{- if .Data.data.schema_registry_username }}
KAFKA_SCHEMA_REGISTRY_USERNAME={{ .Data.data.schema_registry_username }}
{{ end }}
{{- if .Data.data.schema_registry_password }}
KAFKA_SCHEMA_REGISTRY_PASSWORD={{ .Data.data.schema_registry_password }}
{{ end }}
{{- if .Data.data.transaction_id }}
KAFKA_TRANSACTION_ID={{ .Data.data.transaction_id }}
{{ end }}
{{ end }}
EOT
}

# Template for Kafka TLS certificate files
template {
  destination = "/vault/secrets/kafka_cert.pem"
  contents    = <<EOT
{{ with secret "secret/go-sdk/kafka" }}{{ .Data.data.tls_cert }}{{ end }}
EOT
}

template {
  destination = "/vault/secrets/kafka_key.pem"
  contents    = <<EOT
{{ with secret "secret/go-sdk/kafka" }}{{ .Data.data.tls_key }}{{ end }}
EOT
}

template {
  destination = "/vault/secrets/kafka_ca.pem"
  contents    = <<EOT
{{ with secret "secret/go-sdk/kafka" }}{{ .Data.data.tls_ca }}{{ end }}
EOT
}

# Template for Temporal secrets
template {
  destination = "/vault/secrets/temporal.env"
  command     = "echo 'Temporal secrets updated'"
  contents    = <<EOT
{{ with secret "secret/go-sdk/temporal" }}
TEMPORAL_TLS_CERT_FILE=/vault/secrets/temporal_cert.pem
TEMPORAL_TLS_KEY_FILE=/vault/secrets/temporal_key.pem
TEMPORAL_TLS_CA_FILE=/vault/secrets/temporal_ca.pem
{{ end }}
EOT
}

# Template for Temporal TLS certificate files
template {
  destination = "/vault/secrets/temporal_cert.pem"
  contents    = <<EOT
{{ with secret "secret/go-sdk/temporal" }}{{ .Data.data.tls_cert }}{{ end }}
EOT
}

template {
  destination = "/vault/secrets/temporal_key.pem"
  contents    = <<EOT
{{ with secret "secret/go-sdk/temporal" }}{{ .Data.data.tls_key }}{{ end }}
EOT
}

template {
  destination = "/vault/secrets/temporal_ca.pem"
  contents    = <<EOT
{{ with secret "secret/go-sdk/temporal" }}{{ .Data.data.tls_ca }}{{ end }}
EOT
}

# Optional: Enable metrics
# telemetry {
#   prometheus_retention_time = "30s"
#   disable_hostname          = true
# }
