# Configuration Guide

This guide explains how to add new configuration sources to the `internal/cfg` package.

## Overview

The configuration system uses a hybrid approach:
- **Non-sensitive settings** are loaded from YAML files in `internal/cfg/`
- **Secrets** (passwords, API keys, certificates, credentials) are loaded from environment variables

## Adding a New YAML Config

### Step 1: Create the YAML File

Create a new YAML file in `internal/cfg/` (e.g., `internal/cfg/newconfig.yaml`):

```yaml
# Example configuration
setting_one: "value"
setting_two: 100
enabled: true
```

### Step 2: Update the Go File

Modify the corresponding Go file (e.g., `internal/cfg/newconfig.go`):

#### 2.1 Add Constants

Add these constants at the top of the file:

```go
// Config file path
const configYAMLPath = "internal/cfg/newconfig.yaml"

// Environment variable names for secrets
const (
    envSecretKey = "SECRET_KEY"
    envPassword  = "PASSWORD"
)
```

#### 2.2 Add YAML Config Structs

Add YAML struct types that mirror your YAML file:

```go
type NewConfigYAMLConfig struct {
    SettingOne string `yaml:"setting_one"`
    SettingTwo  int    `yaml:"setting_two"`
    Enabled    bool   `yaml:"enabled"`
}
```

#### 2.3 Add Main Config Struct

Keep your existing config struct for internal use:

```go
type NewConfig struct {
    SettingOne string
    SettingTwo int
    Enabled    bool
    Secret     string  // from env
}
```

#### 2.4 Create Load Function

```go
func (l *Loader) loadNewConfig() *NewConfig {
    yamlCfg, err := l.loadNewConfigYAML()
    if err != nil {
        l.errs = append(l.errs, errors.New("failed to load newconfig yaml config: "+err.Error()))
        return nil
    }

    // Load secrets from env
    secret := l.getEnvWithDefault(envSecretKey, "")

    return &NewConfig{
        SettingOne: yamlCfg.SettingOne,
        SettingTwo: yamlCfg.SettingTwo,
        Enabled:    yamlCfg.Enabled,
        Secret:     secret,
    }
}

func (l *Loader) loadNewConfigYAML() (*NewConfigYAMLConfig, error) {
    yamlData, err := os.ReadFile(configYAMLPath)
    if err != nil {
        return nil, errors.New("failed to read "+configYAMLPath+": " + err.Error())
    }

    var cfg NewConfigYAMLConfig
    if err := yaml.Unmarshal(yamlData, &cfg); err != nil {
        return nil, errors.New("failed to parse "+configYAMLPath+": " + err.Error())
    }

    return &cfg, nil
}
```

### Step 3: Update config.go

Add your config to the main `Config` struct in `config.go`:

```go
type Config struct {
    // ... existing fields
    NewConfig *NewConfig
}
```

And update the `Load()` function:

```go
cfg := &Config{
    // ... existing fields
    NewConfig: l.loadNewConfig(),
}
```

## Environment Variables for Secrets

Always use environment variables for sensitive data:

| Config      | Environment Variables                              |
|-------------|---------------------------------------------------|
| Kafka       | `KAFKA_TLS_CERT_FILE`, `KAFKA_TLS_KEY_FILE`, `KAFKA_TLS_CA_FILE`, `KAFKA_SCHEMA_REGISTRY_USERNAME`, `KAFKA_SCHEMA_REGISTRY_PASSWORD`, `KAFKA_TRANSACTION_ID` |
| RabbitMQ    | `RABBITMQ_URL`                                    |

## Best Practices

1. **YAML for non-secrets**: All non-sensitive configuration should go in YAML
2. **Env for secrets**: Passwords, API keys, tokens, certificates should always come from environment variables
3. **Use constants**: Define environment variable names as constants at the top of the file
4. **Validate required fields**: Check for required fields and add errors to the loader
5. **Provide defaults**: Set sensible defaults in the YAML file, not in code

## Example: Adding a Redis Config (if needed in the future)

```yaml
# internal/cfg/redis.yaml
host: "localhost"
port: 6379
db: 0
pool_size: 10
```

```go
// internal/cfg/redis.go
const redisYAMLPath = "internal/cfg/redis.yaml"

const (
    envREDISPassword = "REDIS_PASSWORD"
)
```

## Troubleshooting

If configuration fails to load:
1. Check that the YAML file exists in `internal/cfg/`
2. Verify environment variables are set for secrets
3. Check for YAML syntax errors
4. Ensure the config struct field names match YAML keys (use `yaml:` tags)
