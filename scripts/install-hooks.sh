#!/bin/bash

HOOKS_DIR=$(git rev-parse --git-path hooks)
mkdir -p "$HOOKS_DIR"

cp .githooks/pre-commit "$HOOKS_DIR/pre-commit"
chmod +x "$HOOKS_DIR/pre-commit"

echo "Git hooks installed successfully!"
