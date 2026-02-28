.PHONY: vendor
vendor:
	go mod tidy && go mod vendor

.PHONY: ins
ins:
	@chmod +x scripts/install.sh && ./scripts/install.sh

.PHONY: up
up:
	docker compose up -d

.PHONY: bu
bu:
	docker compose up --build -d

.PHONY: bgo
bgo:
	docker compose up gosdk-app --build -d

.PHONY: swag
swag:
	swag init -g cmd/myapp/main.go -o api

.PHONY: kt
kt:
	go test ./pkg/kafka/... -v -timeout 5m

.PHONY: rt
rt:
	go test ./pkg/rabbitmq/... -v -timeout 300s

.PHONY: sqlc
sqlc:
	sqlc generate


# Database migration settings
POSTGRES_HOST ?= localhost
POSTGRES_PORT ?= 5432
POSTGRES_USER ?= myuser
POSTGRES_PASSWORD ?= mypassword
POSTGRES_DB ?= mydb
POSTGRES_SSLMODE ?= disable
MIGRATION_PATH ?= db/migrations
DATABASE_URL := postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=$(POSTGRES_SSLMODE)


.PHONY: migrate-create
migrate-create:
	@if [ -z "$(NAME)" ]; then \
		echo "Usage: make migrate-create NAME=<migration_name>"; \
		exit 1; \
	fi
	migrate create -ext sql -dir $(MIGRATION_PATH) -seq $(NAME)


.PHONY: migrate-up
migrate-up:
	migrate -database "$(DATABASE_URL)" -path $(MIGRATION_PATH) up


IMAGE ?= my-app
VERSION ?= latest
DOCKER_USER ?= c1789

.PHONY: docker-push
docker-push:
	docker build -t $(IMAGE):$(VERSION) .
	docker tag $(IMAGE):$(VERSION) $(DOCKER_USER)/$(IMAGE):$(VERSION)
	docker push $(DOCKER_USER)/$(IMAGE):$(VERSION)
