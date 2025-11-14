ins:
	go mod tidy && go mod vendor

up:
	docker compose up -d

build:
	docker compose up --build -d

run:
	go run cmd/myapp/main.go