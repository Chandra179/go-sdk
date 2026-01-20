GOBIN := $(shell go env GOPATH)/bin

.PHONY: all gen swagger install

# Default
all: install swagger

# Usage: make swagger
swagger:
	$(GOBIN)/swag init -d .. -g cmd/myapp/main.go --output .

# Usage: make install
install:
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest
	go install github.com/swaggo/swag/cmd/swag@latest

# Catch-all to allow passing arguments to "gen"
%:
	@: