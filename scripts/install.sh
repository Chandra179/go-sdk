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

echo "All tools installed successfully!"
