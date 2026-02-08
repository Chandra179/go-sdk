ins:
	go mod tidy && go mod vendor
	@$(MAKE) install-lint

it:
	go install github.com/swaggo/swag/cmd/swag@latest
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

up:
	docker compose up -d

ba:
	docker compose up --build -d

bg:
	docker compose up gosdk-app --build -d

swag:
	swag init -g cmd/myapp/main.go -o api

kt:
	go test ./pkg/kafka/... -v -timeout 300s

# Generate type-safe SQL code using sqlc
.PHONY: sqlc
sqlc:
	sqlc generate

# Verify sqlc generated code is up to date
.PHONY: sqlc-verify
sqlc-verify:
	sqlc generate
	@git diff --exit-code internal/storage/db/generated/ || (echo "Generated code is out of date. Run 'make sqlc' to update." && exit 1)

IMAGE ?= my-app
VERSION ?= latest
DOCKER_USER ?= c1789

.PHONY: docker-push
docker-push:
	docker build -t $(IMAGE):$(VERSION) .
	docker tag $(IMAGE):$(VERSION) $(DOCKER_USER)/$(IMAGE):$(VERSION)
	docker push $(DOCKER_USER)/$(IMAGE):$(VERSION)

# Golangci-lint configuration
GOLANGCI_VERSION := v2.1.5

# Install golangci-lint using official binary (recommended method)
.PHONY: install-lint
install-lint:
	@which golangci-lint >/dev/null 2>&1 || (curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(shell go env GOPATH)/bin $(GOLANGCI_VERSION))

# Run golangci-lint
.PHONY: lint
lint: install-lint
	golangci-lint run

# Run golangci-lint with auto-fix
.PHONY: lint-fix
lint-fix: install-lint
	golangci-lint run --fix
