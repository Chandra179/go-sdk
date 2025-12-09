# DB migrations
```
curl -L https://github.com/golang-migrate/migrate/releases/latest/download/migrate.linux-amd64.tar.gz | tar xvz
sudo mv migrate /usr/local/bin/

migrate create -ext sql -dir db/migrations -seq initial

migrate -database "postgres://user:pass@localhost:5432/mydb?sslmode=disable" \
        -path db/migrations up
migrate -database "postgres://user:pass@localhost:5432/mydb?sslmode=disable" \
        -path db/migrations down 1

```