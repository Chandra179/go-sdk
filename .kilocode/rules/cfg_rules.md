# Configuration Guide

This guide explains how to manage configuration within the internal/cfg package using a hybrid strategy of local YAML files and HashiCorp Vault environment injection.

## Overview

The configuration system follows a strict separation of concerns:
- Non-sensitive settings (structural config) are stored in YAML files within internal/cfg/. These define how the application behaves and are committed to version control.
- Secrets (sensitive config) are stored in HashiCorp Vault. These are injected into the application environment at runtime and must never be committed to version control.

## Adding a New Configuration Source

### Step 1: Create the YAML File

Create a new YAML file in internal/cfg/ (e.g., internal/cfg/temporal.yaml). Only include non-sensitive fields.

```yaml
# internal/cfg/temporal.yaml
host: "temporal"
port: 7233
namespace: "default"
worker:
  max_concurrent_activities: 100
  stop_timeout_seconds: 30
```

### Step 2: Update the Go File
Modify the corresponding Go file in internal/cfg/ (e.g., internal/cfg/temporal.go).

#### 2.1 Define Constants and Secret Keys
Define the internal file path and the environment variable keys that Vault will populate.
const configYAMLPath = "internal/cfg/temporal.yaml"

```go
const (
    envTemporalTLSCert = "TEMPORAL_TLS_CERT"
    envTemporalTLSKey  = "TEMPORAL_TLS_KEY"
)
```

#### 2.2 Add Configuration Structs
Define structs for both the raw YAML mapping and the final application config.

```go
type TemporalYAMLConfig struct {
    Host      string `yaml:"host"`
    Port      int    `yaml:"port"`
    Namespace string `yaml:"namespace"`
    Worker    struct {
        MaxActivities int `yaml:"max_concurrent_activities"`
        StopTimeout   int `yaml:"stop_timeout_seconds"`
    } `yaml:"worker"`
}

type TemporalConfig struct {
    Host          string
    Port          int
    Namespace     string
    MaxActivities int
    StopTimeout   int
    TLSCert       string // Loaded from Vault/Env
    TLSKey        string // Loaded from Vault/Env
}
```

#### 2.3 Create the Load Function
The loader merges the YAML file data with the secrets injected into the environment.

```go
func (l *Loader) loadTemporalConfig() *TemporalConfig {
    yamlCfg, err := l.loadTemporalYAML()
    if err != nil {
        l.errs = append(l.errs, errors.New("failed to load temporal yaml: "+err.Error()))
        return nil
    }

    return &TemporalConfig{
        Host:          yamlCfg.Host,
        Port:          yamlCfg.Port,
        Namespace:     yamlCfg.Namespace,
        MaxActivities: yamlCfg.Worker.MaxActivities,
        StopTimeout:   yamlCfg.Worker.StopTimeout,
        TLSCert:       l.getEnv(envTemporalTLSCert),
        TLSKey:        l.getEnv(envTemporalTLSKey),
    }
}

func (l *Loader) loadTemporalYAML() (*TemporalYAMLConfig, error) {
    yamlData, err := os.ReadFile(configYAMLPath)
    if err != nil {
        return nil, err
    }
    var cfg TemporalYAMLConfig
    if err := yaml.Unmarshal(yamlData, &cfg); err != nil {
        return nil, err
    }
    return &cfg, nil
}
```

### Step 3: Update config.go
Register the new config in the root Config struct and the Load() function.

```go
type Config struct {
    // ...
    Temporal *TemporalConfig
}

// In Load()
cfg := &Config{
    // ...
    Temporal: l.loadTemporalConfig(),
}
```

## HashiCorp Vault Secret Management

### CLI Usage

To save secrets into Vault so the application can read them, use the kv put command.

```bash
# Postgres secrets
vault kv put secret/go-sdk/postgres password="secure-password"

# OAuth2/JWT secrets
vault kv put secret/go-sdk/oauth2 \
  client_id="your-google-client-id" \
  client_secret="your-google-client-secret" \
  jwt_secret="your-jwt-secret-min-32-chars"

# Redis secrets
vault kv put secret/go-sdk/redis password="redis-password"

# RabbitMQ secrets (URL contains credentials)
vault kv put secret/go-sdk/rabbitmq url="amqp://user:pass@host:5672/"

# Kafka secrets (TLS and Schema Registry)
vault kv put secret/go-sdk/kafka \
  tls_cert_file="/path/to/cert.pem" \
  tls_key_file="/path/to/key.pem" \
  schema_registry_username="user" \
  schema_registry_password="pass"

# Temporal secrets (TLS certificates)
vault kv put secret/go-sdk/temporal \
  tls_cert_file="/path/to/temporal-cert.pem" \
  tls_key_file="/path/to/temporal-key.pem"
```

### Environment Injection Reference

The application accesses these secrets through environment variable injection via Vault Agent. The Vault Agent runs as a sidecar and writes secrets to `/vault/secrets/*.env` files.

| Service | Secret Variables | Vault Path |
|---------|------------------|------------|
| Postgres | `POSTGRES_PASSWORD` | `secret/go-sdk/postgres` |
| OAuth2 | `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | `secret/go-sdk/oauth2` |
| JWT | `JWT_SECRET` | `secret/go-sdk/oauth2` |
| Redis | `REDIS_PASSWORD` | `secret/go-sdk/redis` |
| RabbitMQ | `RABBITMQ_URL` | `secret/go-sdk/rabbitmq` |
| Kafka | `tls_cert`, `tls_key`, `tls_ca` (content), `KAFKA_SCHEMA_REGISTRY_USERNAME`, `KAFKA_SCHEMA_REGISTRY_PASSWORD`, `KAFKA_TRANSACTION_ID` | `secret/go-sdk/kafka` |
| Temporal | `tls_cert`, `tls_key`, `tls_ca` (content) | `secret/go-sdk/temporal` |

**Note**: For TLS certificates, the actual certificate content is stored in Vault (not just file paths). The `vault-setup-secrets.sh` script loads certificate files from the `secrets/` directory into Vault.

### Helper Scripts

Use the provided scripts for easier Vault management:

```bash
# Initialize Vault and create policies
./scripts/vault-init.sh

# Setup all application secrets (interactive mode)
./scripts/vault-setup-secrets.sh -i

# Quick development mode (starts Vault + Agent + sets up secrets)
./scripts/vault-dev-mode.sh

# Verify secrets are set correctly
./scripts/vault-setup-secrets.sh --verify-only
```

### Docker Compose Integration

When running with Docker Compose, Vault and Vault Agent start automatically:

```bash
# Start all services including Vault
docker-compose up -d

# Vault UI available at http://localhost:8200 (token: root)
# Secrets are automatically injected into the application container
```

## Best Practices

**YAML for Structure**: Use YAML for timeouts, limits, hostnames, and logic-altering toggles.

**Vault for Identity**: Use Vault for passwords, private keys, and API tokens.

**Fail Early**: If a required secret is missing from the environment, the Loader must append an error to prevent the app from starting in an unstable state. See `internal/cfg/loader.go` for implementation.

**Local Dev**: Use the `vault-dev-mode.sh` script or a `.env.vault` file (git-ignored) to simulate Vault injection during local development. Never commit actual secrets.

**No Defaults for Secrets**: Never provide hardcoded fallback values for sensitive credentials in the code. The `requireEnv()` function in the Loader enforces this.

**Secret Rotation**: When using Vault, secrets can be rotated without application restart. Vault Agent will automatically re-render templates when secrets change.

**Production Hardening**:
- Use TLS for Vault connections (`https://vault.production.example.com:8200`)
- Replace token-based auth with AppRole, Kubernetes auth, or cloud IAM
- Enable Vault audit logging
- Use short-lived dynamic credentials where possible (database credentials, cloud IAM)
- Rotate root tokens regularly

**Security Checklist**:
- [ ] All secrets stored in Vault, none in code or YAML
- [ ] `.env.example` contains only non-sensitive configuration
- [ ] Vault Agent sidecar configured with minimal privileges
- [ ] Policies follow least-privilege principle (see `vault/gosdk-policy.hcl`)
- [ ] Local development uses dev mode only
- [ ] Production uses proper authentication (not root token)