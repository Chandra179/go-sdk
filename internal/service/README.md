# Internal Service
These 3 files mandatory for service, if service.go to big seperated into new files.

```
├── service/                           # service root directory
│   ├── auth/                          # example: package name auth
│	│   ├── handler.go                 # endpoint handler using gin
│	│   ├── service.go                 # service logic (business logic, query, etc..)
│	│   ├── types.go                   # struct, const, etc..

```

## handler.go
```go
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}
```

## service.go
```go
type Service struct {
}

func NewService() *Service {
	return &Service{
	}
}
```

## types.go
```go
type A struct {
}

type B struct {
}

const (
)
```