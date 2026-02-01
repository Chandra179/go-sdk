ins:
	go mod tidy && go mod vendor

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