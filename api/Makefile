GOBIN := $(shell go env GOPATH)/bin

.PHONY: all gen swagger install

# Default
all: install swagger
 
# Usage: make gen proto/user/user.proto
gen:
	@if [ -z "$(filter-out $@,$(MAKECMDGOALS))" ]; then echo "Error: Please specify a proto file"; exit 1; fi
	@mkdir -p proto/gen
	protoc --go_out=./proto/gen --go_opt=paths=source_relative \
	       --go-grpc_out=./proto/gen --go-grpc_opt=paths=source_relative \
	       $(filter-out $@,$(MAKECMDGOALS))

# Usage: make swagger
swagger:
	$(GOBIN)/swag init -d .. -g cmd/myapp/main.go --output .

# Usage: make install
install:
	sudo apt install -y protobuf-compiler
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/swaggo/swag/cmd/swag@latest

# Catch-all to allow passing arguments to "gen"
%:
	@: