package main

import (
	"context"
	"database/sql"
	"io"
	"log"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/getmeemaw/meemaw/server"
	"github.com/getmeemaw/meemaw/server/database"
	"github.com/getmeemaw/meemaw/server/vault"
	"github.com/pkg/errors"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	// check if logging should be enabled or disabled
	var logging bool
	if len(os.Args[1:]) > 0 && os.Args[1] == "-s" {
		log.Println("Silence mode: logging discarded")
		log.SetOutput(io.Discard)
	} else {
		log.Println("Logging enabled")
		logging = true
	}

	// load config
	config, err := loadConfigFromFile("config.toml") // Update for args ?
	if err != nil {
		log.Fatalf("Unable to load config: %v\n", err)
	}

	// connect to DB
	db, err := sql.Open("pgx", config.DbConnectionUrl)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer db.Close()

	// load sqlc queries
	queries := database.New(db)

	// load vault
	vault := vault.New(queries)

	// verify db connexion for good measure
	_, err = queries.Status(context.Background())
	if err != nil {
		panic(errors.Wrap(err, "Error during PostgreSQL status"))
	}
	log.Println("Connected to DB")

	// verify db schema exists
	_, err = queries.GetFirstUser(context.Background())
	if err != nil && err != sql.ErrNoRows {
		log.Println("Schema does not exist, creating...")
		err = server.LoadSchema(db)
		if err != nil {
			log.Fatalf("Could not load schema: %s", err)
		} else {
			log.Println("Schema loaded")
		}
	} else {
		log.Println("Schema exists")
	}

	// create server based on queries and config
	server := server.NewServer(vault, config, nil, logging)

	// start server
	server.Start()
}

func loadConfigFromFile(path string) (*server.Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		log.Println("Could not read config file:", err)
		return nil, err
	}

	var config server.Config
	if err := toml.Unmarshal(bytes, &config); err != nil {
		log.Fatalf("Failed to unmarshal configuration: %v", err)
	}

	return &config, nil
}
