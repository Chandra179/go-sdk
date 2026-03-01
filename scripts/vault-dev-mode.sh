#!/bin/bash
# Vault Development Mode Script
# Quick setup for local development with Vault in dev mode

set -e

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

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

# Cleanup function
cleanup() {
    log_info "Cleaning up..."
    docker-compose stop vault vault-agent 2>/dev/null || true
}

# Setup trap for cleanup on exit
trap cleanup EXIT

# Check prerequisites
check_prerequisites() {
    log_step "Checking prerequisites..."
    
    if ! command -v docker-compose &> /dev/null; then
        log_error "docker-compose is required but not installed"
        exit 1
    fi
    
    if ! command -v curl &> /dev/null; then
        log_error "curl is required but not installed"
        exit 1
    fi
    
    log_info "Prerequisites check passed"
}

# Start Vault in dev mode
start_vault() {
    log_step "Starting Vault in development mode..."
    
    # Create necessary directories
    mkdir -p vault/token
    mkdir -p vault/secrets
    
    # Start just the Vault service
    docker-compose up -d vault
    
    # Wait for Vault to be ready
    log_info "Waiting for Vault to be ready..."
    local attempts=0
    local max_attempts=30
    
    while [ $attempts -lt $max_attempts ]; do
        if curl -s http://localhost:8200/v1/sys/health > /dev/null 2>&1; then
            log_info "Vault is ready!"
            break
        fi
        attempts=$((attempts + 1))
        sleep 2
    done
    
    if [ $attempts -eq $max_attempts ]; then
        log_error "Vault failed to start within 60 seconds"
        exit 1
    fi
    
    # Save root token
    echo "root" > vault/token/root-token
    log_info "Root token saved to vault/token/root-token"
}

# Initialize Vault
init_vault() {
    log_step "Initializing Vault..."
    
    export VAULT_ADDR="http://localhost:8200"
    export VAULT_TOKEN="root"
    
    ./scripts/vault-init.sh
}

# Setup secrets
setup_secrets() {
    log_step "Setting up secrets..."
    
    # Auto-generate a JWT secret if not set
    if [ -z "$JWT_SECRET" ]; then
        JWT_SECRET=$(openssl rand -base64 48)
        log_info "Generated JWT secret"
    fi
    
    # Set defaults for other secrets
    export POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-"devpassword"}
    export REDIS_PASSWORD=${REDIS_PASSWORD:-"redispass"}
    export RABBITMQ_URL=${RABBITMQ_URL:-"amqp://guest:guest@rabbitmq:5672/"}
    
    # Load TLS certificates if available in secrets directory
    if [ -f "./secrets/kafka_cert.pem" ]; then
        export KAFKA_TLS_CERT_FILE="./secrets/kafka_cert.pem"
        log_info "Found Kafka TLS certificate"
    fi
    
    if [ -f "./secrets/kafka_key.pem" ]; then
        export KAFKA_TLS_KEY_FILE="./secrets/kafka_key.pem"
        log_info "Found Kafka TLS key"
    fi
    
    if [ -f "./secrets/kafka_ca.pem" ]; then
        export KAFKA_TLS_CA_FILE="./secrets/kafka_ca.pem"
        log_info "Found Kafka TLS CA"
    fi
    
    if [ -f "./secrets/temporal_cert.pem" ]; then
        export TEMPORAL_TLS_CERT_FILE="./secrets/temporal_cert.pem"
        log_info "Found Temporal TLS certificate"
    fi
    
    if [ -f "./secrets/temporal_key.pem" ]; then
        export TEMPORAL_TLS_KEY_FILE="./secrets/temporal_key.pem"
        log_info "Found Temporal TLS key"
    fi
    
    if [ -f "./secrets/temporal_ca.pem" ]; then
        export TEMPORAL_TLS_CA_FILE="./secrets/temporal_ca.pem"
        log_info "Found Temporal TLS CA"
    fi
    
    # Run setup script
    ./scripts/vault-setup-secrets.sh
}

# Start Vault Agent
start_vault_agent() {
    log_step "Starting Vault Agent..."
    
    docker-compose up -d vault-agent
    
    # Wait for Vault Agent to write secrets
    log_info "Waiting for Vault Agent to fetch secrets..."
    local attempts=0
    local max_attempts=30
    
    while [ $attempts -lt $max_attempts ]; do
        if [ -f "vault/secrets/postgres.env" ]; then
            log_info "Vault Agent has written secrets!"
            break
        fi
        attempts=$((attempts + 1))
        sleep 2
    done
    
    if [ $attempts -eq $max_attempts ]; then
        log_warn "Vault Agent may still be initializing"
    fi
}

# Show status
show_status() {
    log_step "Vault Development Mode Status"
    echo ""
    echo "Vault UI: http://localhost:8200"
    echo "Token: root"
    echo ""
    echo "Secret files created:"
    ls -la vault/secrets/ 2>/dev/null || echo "  (none yet)"
    echo ""
    echo "To view secrets:"
    echo "  cat vault/secrets/postgres.env"
    echo "  cat vault/secrets/oauth2.env"
    echo ""
    echo "Next steps:"
    echo "  1. Start all services: docker-compose up -d"
    echo "  2. View logs: docker-compose logs -f gosdk-app"
    echo "  3. Access Vault UI: http://localhost:8200 (token: root)"
    echo ""
    echo "To stop Vault: docker-compose stop vault vault-agent"
}

# Main execution
main() {
    log_info "Starting Vault Development Mode Setup"
    log_warn "This runs Vault in DEV mode - DO NOT use for production!"
    echo ""
    
    check_prerequisites
    start_vault
    init_vault
    setup_secrets
    start_vault_agent
    show_status
    
    log_info "Development mode setup complete!"
}

# Show help
show_help() {
    cat << EOF
Vault Development Mode Script

This script quickly sets up a local Vault instance in development mode
with all necessary secrets for the go-sdk application.

Usage: $0 [OPTIONS]

Options:
  -h, --help          Show this help message
  --no-cleanup        Don't cleanup on exit (keep Vault running)

Environment Variables:
  POSTGRES_PASSWORD   Set a custom Postgres password (default: devpassword)
  REDIS_PASSWORD      Set a custom Redis password (default: redispass)
  JWT_SECRET          Set a custom JWT secret (auto-generated if not set)
  RABBITMQ_URL        Set a custom RabbitMQ URL

Examples:
  $0                              # Quick start with defaults
  POSTGRES_PASSWORD=mypass $0     # Start with custom password
  $0 --no-cleanup                 # Start and keep running after exit

What this script does:
  1. Starts Vault in development mode (in-memory, unsealed)
  2. Initializes Vault (enables KV engine, creates policy)
  3. Sets up all application secrets with dev values
  4. Starts Vault Agent to fetch and cache secrets
  5. Creates secret files in vault/secrets/

After running, you can start the full application:
  docker-compose up -d

Or access Vault UI at http://localhost:8200 (token: root)
EOF
}

# Parse arguments
NO_CLEANUP=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        --no-cleanup)
            NO_CLEANUP=true
            shift
            ;;
        *)
            log_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

if [ "$NO_CLEANUP" = true ]; then
    trap - EXIT
fi

main