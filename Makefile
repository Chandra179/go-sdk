ins:
	go mod tidy && go mod vendor

up:
	docker compose up -d

build:
	docker compose up --build -d

run:
	go run cmd/myapp/main.go

swag:
	swag init -g /cmd/myapp/main.go -o api

########################################
# DOCKER IMAGE WORKFLOW
########################################

IMAGE ?= my-app
VERSION ?= latest
DOCKER_USER ?= c1789


.PHONY: docker-build
docker-build:
	docker build -t $(IMAGE):$(VERSION) .

.PHONY: docker-tag
docker-tag:
	docker tag $(IMAGE):$(VERSION) $(DOCKER_USER)/$(IMAGE):$(VERSION)

.PHONY: docker-push
docker-push:
	docker push $(DOCKER_USER)/$(IMAGE):$(VERSION)