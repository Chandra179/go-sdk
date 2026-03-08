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

IMAGE ?= my-app
VERSION ?= latest
DOCKER_USER ?= c1789

.PHONY: docker-push
docker-push:
	docker build -t $(IMAGE):$(VERSION) .
	docker tag $(IMAGE):$(VERSION) $(DOCKER_USER)/$(IMAGE):$(VERSION)
	docker push $(DOCKER_USER)/$(IMAGE):$(VERSION)
