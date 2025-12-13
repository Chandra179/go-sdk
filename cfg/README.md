# Config
.yaml, .env, config files 

## config.go
```go
package config

type Config struct {
	ServiceA         ServiceAConfig
	ServiceB      ServiceBConfig
}
type ServiceAConfig struct {
    env1 string
}
type ServiceBConfig struct {
    env2 string
}

env1 := mustEnv("ENV_1", &errs)
env2 := mustEnv("ENV_2", &errs)

return &Config{
    ServiceA: ServiceAConfig{},
    ServiceB: ServiceBConfig{}
}


func getEnvOrDefault(key, defaultValue string) string 
func mustEnv(key string, errs *[]error) string
```