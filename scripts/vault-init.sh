#!/bin/bash
# Vault Initialization Script
# This script initializes Vault and sets up the necessary configuration

set -e

# Configuration
VAULT_ADDR=${VAULT_ADDR:-"http://localhost:8200"}
VAULT_TOKEN=${VAULT_TOKEN:-"root"}
POLICY_FILE=${POLICY_FILE:-"vault/gosdk-policy.hcl"}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
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

# Check if Vault is reachable
check_vault() {
    log_info "Checking Vault connection at $VAULT_ADDR..."
    if ! curl -s "$VAULT_ADDR/v1/sys/health" > /dev/null 2>&1; then
        log_error "Vault is not reachable at $VAULT_ADDR"
        log_error "Please start Vault first: docker-compose up -d vault"
        exit 1
    fi
    log_info "Vault is reachable"
}

# Enable KV secrets engine
enable_kv_engine() {
    log_info "Enabling KV secrets engine..."
    
    # Check if already enabled
    if curl -s -H "X-Vault-Token: $VAULT_TOKEN" "$VAULT_ADDR/v1/sys/mounts" | grep -q '"secret/"'; then
        log_warn "KV secrets engine already enabled"
    else
        curl -s -X POST -H "X-Vault-Token: $VAULT_TOKEN" \
            -H "Content-Type: application/json" \
            -d '{"type": "kv", "options": {"version": "2"}}' \
            "$VAULT_ADDR/v1/sys/mounts/secret" > /dev/null
        log_info "KV secrets engine enabled"
    fi
}

# Create policy
create_policy() {
    log_info "Creating go-sdk policy from $POLICY_FILE..."
    
    if [ ! -f "$POLICY_FILE" ]; then
        log_error "Policy file not found: $POLICY_FILE"
        exit 1
    fi
    
    # Read policy content
    POLICY_CONTENT=$(cat "$POLICY_FILE" | sed 's/"/\\"/g' | tr '\n' ' ')
    
    curl -s -X PUT -H "X-Vault-Token: $VAULT_TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"policy\": \"$POLICY_CONTENT\"}" \
        "$VAULT_ADDR/v1/sys/policies/acl/gosdk-app" > /dev/null
    
    log_info "Policy 'gosdk-app' created/updated"
}

# Create token for application
create_token() {
    log_info "Creating token for go-sdk app..."
    
    TOKEN_RESPONSE=$(curl -s -X POST -H "X-Vault-Token: $VAULT_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"policies": ["gosdk-app"], "ttl": "768h", "renewable": true}' \
        "$VAULT_ADDR/v1/auth/token/create")
    
    CLIENT_TOKEN=$(echo "$TOKEN_RESPONSE" | grep -o '"client_token":"[^"]*"' | cut -d'"' -f4)
    
    if [ -n "$CLIENT_TOKEN" ]; then
        log_info "Token created successfully"
        log_warn "Save this token securely: $CLIENT_TOKEN"
        echo "$CLIENT_TOKEN" > .vault-token
        log_info "Token saved to .vault-token file"
    else
        log_error "Failed to create token"
        exit 1
    fi
}

# Create directory structure for local dev
setup_directories() {
    log_info "Setting up Vault directories..."
    
    # Create directories for Vault Agent
    mkdir -p vault/token
    mkdir -p vault/secrets
    
    # Save root token for local dev
    if [ "$VAULT_TOKEN" = "root" ]; then
        echo "root" > vault/token/root-token
        log_info "Root token saved to vault/token/root-token"
    fi
}

# Main execution
main() {
    log_info "Starting Vault initialization..."
    
    check_vault
    setup_directories
    enable_kv_engine
    create_policy
    
    log_info "Vault initialization completed successfully!"
    log_info "Next steps:"
    log_info "  1. Run: ./scripts/vault-setup-secrets.sh"
    log_info "  2. Start Vault Agent: docker-compose up -d vault-agent"
    log_info "  3. Start application: docker-compose up -d gosdk-app"
}

# Show help
show_help() {
    cat << EOF
Vault Initialization Script

Usage: $0 [OPTIONS]

Options:
  -h, --help          Show this help message
  -a, --addr          Vault address (default: http://localhost:8200)
  -t, --token         Vault token (default: root)
  -p, --policy        Path to policy file (default: vault/gosdk-policy.hcl)

Examples:
  $0                                    # Run with defaults
  $0 -a http://vault:8200 -t my-token  # Use custom address and token

Environment Variables:
  VAULT_ADDR          Vault server address
  VAULT_TOKEN         Vault authentication token
  POLICY_FILE         Path to the HCL policy file
EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        -a|--addr)
            VAULT_ADDR="$2"
            shift 2
            ;;
        -t|--token)
            VAULT_TOKEN="$2"
            shift 2
            ;;
        -p|--policy)
            POLICY_FILE="$2"
            shift 2
            ;;
        *)
            log_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

main
