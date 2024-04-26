package server

import (
	"database/sql"
	"strings"

	_ "embed"
)

//go:embed sqlc/schema.sql
var schema string

func LoadSchema(_db *sql.DB) error {
	queries := strings.Split(schema, ";")

	for _, query := range queries {
		_, err := _db.Exec(query)
		if err != nil {
			return err
		}
	}

	return nil
}
