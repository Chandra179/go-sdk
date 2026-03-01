#!/bin/bash
# Vault Secrets Setup Script
# This script populates Vault with all application secrets

set -e

# Configuration
VAULT_ADDR=${VAULT_ADDR:-"http://localhost:8200"}
VAULT_TOKEN=${VAULT_TOKEN:-"root"}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_prompt() {
    echo -e "${BLUE}[PROMPT]${NC} $1"
}

# Check if Vault is reachable
check_vault() {
    log_info "Checking Vault connection at $VAULT_ADDR..."
    if ! curl -s -H "X-Vault-Token: $VAULT_TOKEN" "$VAULT_ADDR/v1/sys/health" > /dev/null 2>&1; then
        log_error "Vault is not reachable at $VAULT_ADDR"
        log_error "Please start Vault first: docker-compose up -d vault"
        exit 1
    fi
    log_info "Vault is reachable and healthy"
}

# Helper function to put secrets
put_secret() {
    local path=$1
    local data=$2
    
    curl -s -X POST -H "X-Vault-Token: $VAULT_TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"data\": $data}" \
        "$VAULT_ADDR/v1/secret/$path" > /dev/null
}

# Setup Postgres secrets
setup_postgres() {
    log_info "Setting up Postgres secrets..."
    
    local password=${POSTGRES_PASSWORD:-"devpassword"}
    
    if [ -z "$POSTGRES_PASSWORD" ]; then
        log_warn "POSTGRES_PASSWORD not set, using default: devpassword"
        log_warn "Consider setting a stronger password for production"
    fi
    
    put_secret "secret/go-sdk/postgres" "{\"password\": \"$password\"}"
    log_info "Postgres secrets configured"
}

# Setup OAuth2/JWT secrets
setup_oauth2() {
    log_info "Setting up OAuth2/JWT secrets..."
    
    local client_id=${GOOGLE_CLIENT_ID:-""}
    local client_secret=${GOOGLE_CLIENT_SECRET:-""}
    local jwt_secret=${JWT_SECRET:-"dev-jwt-secret-must-be-at-least-32-chars-long"}
    
    # Validate JWT secret length
    if [ ${#jwt_secret} -lt 32 ]; then
        log_error "JWT_SECRET must be at least 32 characters"
        exit 1
    fi
    
    if [ -z "$GOOGLE_CLIENT_ID" ] || [ -z "$GOOGLE_CLIENT_SECRET" ]; then
        log_warn "Google OAuth credentials not set"
        log_warn "Authentication will not work until these are configured"
    fi
    
    put_secret "secret/go-sdk/oauth2" "{\"client_id\": \"$client_id\", \"client_secret\": \"$client_secret\", \"jwt_secret\": \"$jwt_secret\"}"
    log_info "OAuth2/JWT secrets configured"
}

# Setup Redis secrets
setup_redis() {
    log_info "Setting up Redis secrets..."
    
    local password=${REDIS_PASSWORD:-"redispass"}
    
    if [ -z "$REDIS_PASSWORD" ]; then
        log_warn "REDIS_PASSWORD not set, using default: redispass"
    fi
    
    put_secret "secret/go-sdk/redis" "{\"password\": \"$password\"}"
    log_info "Redis secrets configured"
}

# Setup RabbitMQ secrets
setup_rabbitmq() {
    log_info "Setting up RabbitMQ secrets..."
    
    local url=${RABBITMQ_URL:-"amqp://guest:guest@rabbitmq:5672/"}
    
    put_secret "secret/go-sdk/rabbitmq" "{\"url\": \"$url\"}"
    log_info "RabbitMQ secrets configured"
}

# Setup Kafka secrets
setup_kafka() {
    log_info "Setting up Kafka secrets..."
    
    # Build JSON dynamically based on which vars are set
    local json_parts=()
    
    # Load TLS certificates from files if paths are provided
    if [ -n "$KAFKA_TLS_CERT_FILE" ] && [ -f "$KAFKA_TLS_CERT_FILE" ]; then
        local cert_content=$(cat "$KAFKA_TLS_CERT_FILE" | sed 's/"/\\"/g' | tr '\n' ' ')
        json_parts+=("\"tls_cert\": \"$cert_content\"")
        log_info "Loaded Kafka TLS certificate from $KAFKA_TLS_CERT_FILE"
    fi
    
    if [ -n "$KAFKA_TLS_KEY_FILE" ] && [ -f "$KAFKA_TLS_KEY_FILE" ]; then
        local key_content=$(cat "$KAFKA_TLS_KEY_FILE" | sed 's/"/\\"/g' | tr '\n' ' ')
        json_parts+=("\"tls_key\": \"$key_content\"")
        log_info "Loaded Kafka TLS key from $KAFKA_TLS_KEY_FILE"
    fi
    
    if [ -n "$KAFKA_TLS_CA_FILE" ] && [ -f "$KAFKA_TLS_CA_FILE" ]; then
        local ca_content=$(cat "$KAFKA_TLS_CA_FILE" | sed 's/"/\\"/g' | tr '\n' ' ')
        json_parts+=("\"tls_ca\": \"$ca_content\"")
        log_info "Loaded Kafka TLS CA from $KAFKA_TLS_CA_FILE"
    fi
    
    if [ -n "$KAFKA_SCHEMA_REGISTRY_USERNAME" ]; then
        json_parts+=("\"schema_registry_username\": \"$KAFKA_SCHEMA_REGISTRY_USERNAME\"")
    fi
    
    if [ -n "$KAFKA_SCHEMA_REGISTRY_PASSWORD" ]; then
        json_parts+=("\"schema_registry_password\": \"$KAFKA_SCHEMA_REGISTRY_PASSWORD\"")
    fi
    
    if [ -n "$KAFKA_TRANSACTION_ID" ]; then
        json_parts+=("\"transaction_id\": \"$KAFKA_TRANSACTION_ID\"")
    fi
    
    # Join parts with commas
    local json_data="{"
    local first=true
    for part in "${json_parts[@]}"; do
        if [ "$first" = true ]; then
            first=false
        else
            json_data+=","
        fi
        json_data+="$part"
    done
    json_data+="}"
    
    if [ ${#json_parts[@]} -eq 0 ]; then
        log_warn "No Kafka secrets set, storing empty object"
        json_data="{}"
    fi
    
    put_secret "secret/go-sdk/kafka" "$json_data"
    log_info "Kafka secrets configured"
}

# Setup Temporal secrets
setup_temporal() {
    log_info "Setting up Temporal secrets..."
    
    # Build JSON dynamically
    local json_parts=()
    
    # Load TLS certificates from files if paths are provided
    if [ -n "$TEMPORAL_TLS_CERT_FILE" ] && [ -f "$TEMPORAL_TLS_CERT_FILE" ]; then
        local cert_content=$(cat "$TEMPORAL_TLS_CERT_FILE" | sed 's/"/\\"/g' | tr '\n' ' ')
        json_parts+=("\"tls_cert\": \"$cert_content\"")
        log_info "Loaded Temporal TLS certificate from $TEMPORAL_TLS_CERT_FILE"
    fi
    
    if [ -n "$TEMPORAL_TLS_KEY_FILE" ] && [ -f "$TEMPORAL_TLS_KEY_FILE" ]; then
        local key_content=$(cat "$TEMPORAL_TLS_KEY_FILE" | sed 's/"/\\"/g' | tr '\n' ' ')
        json_parts+=("\"tls_key\": \"$key_content\"")
        log_info "Loaded Temporal TLS key from $TEMPORAL_TLS_KEY_FILE"
    fi
    
    if [ -n "$TEMPORAL_TLS_CA_FILE" ] && [ -f "$TEMPORAL_TLS_CA_FILE" ]; then
        local ca_content=$(cat "$TEMPORAL_TLS_CA_FILE" | sed 's/"/\\"/g' | tr '\n' ' ')
        json_parts+=("\"tls_ca\": \"$ca_content\"")
        log_info "Loaded Temporal TLS CA from $TEMPORAL_TLS_CA_FILE"
    fi
    
    # Join parts with commas
    local json_data="{"
    local first=true
    for part in "${json_parts[@]}"; do
        if [ "$first" = true ]; then
            first=false
        else
            json_data+=","
        fi
        json_data+="$part"
    done
    json_data+="}"
    
    if [ ${#json_parts[@]} -eq 0 ]; then
        log_warn "No Temporal secrets set, storing empty object"
        json_data="{}"
    fi
    
    put_secret "secret/go-sdk/temporal" "$json_data"
    log_info "Temporal secrets configured"
}

# Verify all secrets
verify_secrets() {
    log_info "Verifying secrets in Vault..."
    
    local paths=(
        "secret/go-sdk/postgres"
        "secret/go-sdk/oauth2"
        "secret/go-sdk/redis"
        "secret/go-sdk/rabbitmq"
        "secret/go-sdk/kafka"
        "secret/go-sdk/temporal"
    )
    
    for path in "${paths[@]}"; do
        if curl -s -H "X-Vault-Token: $VAULT_TOKEN" "$VAULT_ADDR/v1/$path" | grep -q '"data"'; then
            log_info "✓ $path exists"
        else
            log_warn "✗ $path not found"
        fi
    done
}

# Interactive mode for missing secrets
interactive_mode() {
    log_info "Running in interactive mode..."
    
    log_prompt "Enter Postgres password (or press Enter for default 'devpassword'):"
    read -s input
    echo
    POSTGRES_PASSWORD=${input:-"devpassword"}
    
    log_prompt "Enter JWT secret (min 32 chars, or press Enter for auto-generated):"
    read -s input
    echo
    if [ -z "$input" ]; then
        JWT_SECRET=$(openssl rand -base64 32)
        log_info "Generated JWT secret: $JWT_SECRET"
    else
        JWT_SECRET="$input"
    fi
    
    log_prompt "Enter Google Client ID (or press Enter to skip):"
    read input
    GOOGLE_CLIENT_ID="$input"
    
    log_prompt "Enter Google Client Secret (or press Enter to skip):"
    read -s input
    echo
    GOOGLE_CLIENT_SECRET="$input"
    
    log_prompt "Enter Redis password (or press Enter for default 'redispass'):"
    read -s input
    echo
    REDIS_PASSWORD=${input:-"redispass"}
}

# Show current configuration
show_config() {
    log_info "Current configuration:"
    echo "  Vault Address: $VAULT_ADDR"
    echo "  Postgres Password: ${POSTGRES_PASSWORD:+****set****}${POSTGRES_PASSWORD:-not set (will use default)}"
    echo "  JWT Secret: ${JWT_SECRET:+****set****}${JWT_SECRET:-not set (will use generated)}"
    echo "  Google Client ID: ${GOOGLE_CLIENT_ID:+****set****}${GOOGLE_CLIENT_ID:-not set}"
    echo "  Redis Password: ${REDIS_PASSWORD:+****set****}${REDIS_PASSWORD:-not set (will use default)}"
}

# Main execution
main() {
    log_info "Starting Vault secrets setup..."
    
    check_vault
    
    # Check if we should run in interactive mode
    if [ "$INTERACTIVE" = "true" ] || [ -z "$POSTGRES_PASSWORD" -a -z "$JWT_SECRET" ]; then
        interactive_mode
    fi
    
    show_config
    
    # Setup all services
    setup_postgres
    setup_oauth2
    setup_redis
    setup_rabbitmq
    setup_kafka
    setup_temporal
    
    verify_secrets
    
    log_info "Secrets setup completed successfully!"
    log_info ""
    log_info "To use these secrets:"
    log_info "  1. Start Vault Agent: docker-compose up -d vault-agent"
    log_info "  2. Vault Agent will fetch secrets and write to /vault/secrets/"
    log_info "  3. Application will read secrets from environment files"
}

# Show help
show_help() {
    cat << EOF
Vault Secrets Setup Script

Usage: $0 [OPTIONS]

Options:
  -h, --help          Show this help message
  -i, --interactive   Run in interactive mode (prompt for secrets)
  -a, --addr          Vault address (default: http://localhost:8200)
  -t, --token         Vault token (default: root)
  --verify-only       Only verify existing secrets, don't create

Environment Variables:
  VAULT_ADDR                  Vault server address
  VAULT_TOKEN                 Vault authentication token
  POSTGRES_PASSWORD           Postgres password
  GOOGLE_CLIENT_ID            Google OAuth client ID
  GOOGLE_CLIENT_SECRET        Google OAuth client secret
  JWT_SECRET                  JWT signing secret (min 32 chars)
  REDIS_PASSWORD              Redis password
  RABBITMQ_URL                RabbitMQ connection URL
  KAFKA_TLS_CERT_FILE         Kafka TLS certificate file path
  KAFKA_TLS_KEY_FILE          Kafka TLS key file path
  KAFKA_TLS_CA_FILE           Kafka TLS CA file path
  KAFKA_SCHEMA_REGISTRY_USERNAME  Schema registry username
  KAFKA_SCHEMA_REGISTRY_PASSWORD  Schema registry password
  KAFKA_TRANSACTION_ID        Kafka transaction ID
  TEMPORAL_TLS_CERT_FILE      Temporal TLS certificate file path
  TEMPORAL_TLS_KEY_FILE       Temporal TLS key file path
  TEMPORAL_TLS_CA_FILE        Temporal TLS CA file path

Examples:
  $0 -i                                    # Interactive mode
  $0                                       # Use environment variables
  POSTGRES_PASSWORD=mypass JWT_SECRET=... $0  # Set specific secrets
EOF
}

# Parse arguments
VERIFY_ONLY=false
INTERACTIVE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        -i|--interactive)
            INTERACTIVE=true
            shift
            ;;
        -a|--addr)
            VAULT_ADDR="$2"
            shift 2
            ;;
        -t|--token)
            VAULT_TOKEN="$2"
            shift 2
            ;;
        --verify-only)
            VERIFY_ONLY=true
            shift
            ;;
        *)
            log_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

if [ "$VERIFY_ONLY" = true ]; then
    check_vault
    verify_secrets
    exit 0
fi

main
