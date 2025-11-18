package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"gosdk/pkg/db"
)

func main() {
	pgDSN := "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
	client, err := db.NewSQLClient("postgres", pgDSN)
	if err != nil {
		log.Fatal(err)
	}

	err = client.WithTransaction(context.Background(), sql.LevelSerializable,
		func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, "INSERT INTO users(id, name) VALUES($1, $2)", 1, "Alice")
			if err != nil {
				return err
			}
			return nil
		})
	if err != nil {
		fmt.Println("Transaction failed:", err)
	} else {
		fmt.Println("Transaction committed successfully")
	}

	mariaDSN := "user:pass@tcp(localhost:3306)/mydb?charset=utf8mb4&parseTime=True"
	mariaClient, err := db.NewSQLClient("mysql", mariaDSN)
	if err != nil {
		log.Fatal(err)
	}

	mariaClient.WithTransaction(context.Background(), sql.LevelSerializable, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO users(id, name) VALUES(?, ?)", 1, "Alice")
		return err
	})
}
