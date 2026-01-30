ins:
	go mod tidy && go mod vendor

up:
	docker compose up -d

ba:
	docker compose up --build -d

bg:
	docker compose up gosdk-app --build -d

swag:
	swag init -g /cmd/myapp/main.go -o api

IMAGE ?= my-app
VERSION ?= latest
DOCKER_USER ?= c1789

.PHONY: docker-push
docker-push:
	docker build -t $(IMAGE):$(VERSION) .
	docker tag $(IMAGE):$(VERSION) $(DOCKER_USER)/$(IMAGE):$(VERSION)
	docker push $(DOCKER_USER)/$(IMAGE):$(VERSION)

test:
	go test ./... -race -cover

test-coverage:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# Example applications
.PHONY: build-examples
build-examples:
	go build -o bin/producer ./cmd/examples/producer
	go build -o bin/consumer ./cmd/examples/consumer

.PHONY: run-producer
run-producer:
	go run ./cmd/examples/producer/main.go

.PHONY: run-consumer
run-consumer:
	go run ./cmd/examples/consumer/main.go

.PHONY: docker-build-examples
docker-build-examples:
	docker build -f Dockerfile.examples --target producer -t kafka-example-producer:latest .
	docker build -f Dockerfile.examples --target consumer -t kafka-example-consumer:latest .
