package server

import (
	"database/sql"
	"os"
	"strings"

	_ "embed"
)

//go:embed sqlc/schema.sql
var schema string

func LoadSchema(_db *sql.DB, path string) error {
	var queries []string

	if path == "" {
		queries = strings.Split(schema, ";")
	} else {
		schemaFile, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		queries = strings.Split(string(schemaFile), ";")
	}

	for _, query := range queries {
		_, err := _db.Exec(query)
		if err != nil {
			return err
		}
	}

	return nil
}
