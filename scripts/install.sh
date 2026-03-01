#!/bin/bash

# Installation script for Go SDK development tools
set -e

echo "Installing Go development tools..."

# Install sqlc for SQL code generation
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Install swag for Swagger documentation generation
go install github.com/swaggo/swag/cmd/swag@latest

# Install golangci-lint
GOLANGCI_VERSION="v2.1.5"
if ! command -v golangci-lint &> /dev/null; then
    echo "Installing golangci-lint ${GOLANGCI_VERSION}..."
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(go env GOPATH)/bin ${GOLANGCI_VERSION}
else
    echo "golangci-lint is already installed"
fi

# Install golang-migrate for database migrations
MIGRATE_VERSION=$(curl -s https://api.github.com/repos/golang-migrate/migrate/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | cut -c2-)
echo "Installing golang-migrate ${MIGRATE_VERSION}..."
curl -L https://github.com/golang-migrate/migrate/releases/latest/download/migrate.linux-amd64.tar.gz | tar xvz
chmod +x migrate
mv migrate $(go env GOPATH)/bin/

# Install HashiCorp Vault
if ! command -v vault &> /dev/null; then
    echo "Installing HashiCorp Vault..."
    VAULT_VERSION=$(curl -s https://api.github.com/repos/hashicorp/vault/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | tr -d 'v')
    curl -LO "https://releases.hashicorp.com/vault/${VAULT_VERSION}/vault_${VAULT_VERSION}_linux_amd64.zip"
    unzip -o "vault_${VAULT_VERSION}_linux_amd64.zip" -d $(go env GOPATH)/bin/
    rm -f "vault_${VAULT_VERSION}_linux_amd64.zip"
else
    echo "HashiCorp Vault is already installed"
fi

echo ""
echo "=============================================="
echo "All tools installed successfully!"
echo "=============================================="
echo ""
echo "Next steps for Vault setup:"
echo "  1. Copy environment file: cp .env.example .env"
echo "  2. Copy Vault config: cp .env.vault.example .env.vault"
echo "  3. Start Vault in dev mode: ./scripts/vault-dev-mode.sh"
echo "  4. Or setup Vault manually:"
echo "     - docker-compose up -d vault"
echo "     - ./scripts/vault-init.sh"
echo "     - ./scripts/vault-setup-secrets.sh -i"
echo ""
echo "For detailed Vault setup instructions, see: docs/VAULT_SETUP.md"
echo ""
