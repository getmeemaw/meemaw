package main

import (
	"context"
	"database/sql"
	"io"
	"log"
	"os"

	"github.com/getmeemaw/meemaw/server"
	"github.com/getmeemaw/meemaw/server/database"
	"github.com/getmeemaw/meemaw/server/vault"
	"github.com/getmeemaw/meemaw/utils/config"
	"github.com/joho/godotenv"

	_ "github.com/jackc/pgx/v5/stdlib"

	_ "embed"
)

/////////
//
// cmd is the entrypoint of Meemaw. It loads the config, the db, the vault, and starts the server.
//
/////////

//go:generate bash -c "GOOS=js GOARCH=wasm go build -o meemaw.wasm ../../client/web/wasm/main.go"

//go:embed meemaw.wasm
var wasmBinary []byte

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

	// config, err := loadConfigFromFile("config.toml")
	// if err != nil {
	// 	log.Fatalf("Unable to load config: %v\n", err)
	// }

	config, err := loadConfigFromEnvs()
	if err != nil {
		log.Fatalf("Unable to load config: %v\n", err)
		os.Exit(1)
	}

	// connect to DB
	db, err := sql.Open("pgx", config.DbConnectionUrl)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// load sqlc queries
	queries := database.New(db)

	// load vault
	vault := vault.NewVault(queries)

	// verify db connexion for good measure
	_, err = queries.Status(context.Background())
	if err != nil {
		log.Fatalf("Error during db sttuse: %v\n", err)
		os.Exit(1)
	}
	log.Println("Connected to DB")

	// verify db schema exists
	_, err = queries.GetFirstUser(context.Background())
	if err != nil && err != sql.ErrNoRows {
		log.Println("Schema does not exist, creating...")
		err = server.LoadSchema(db, "")
		if err != nil {
			log.Fatalf("Could not load schema: %s", err)
			os.Exit(1)
		} else {
			log.Println("Schema loaded")
		}
	} else {
		log.Println("Schema exists")
	}

	// create server based on queries and config
	server := server.NewServer(vault, config, wasmBinary, logging)

	// start server
	server.Start()
}

func loadConfigFromEnvs() (*server.Config, error) {
	// Try to load from .env, if exists
	err := godotenv.Load()
	if err != nil {
		log.Printf("No .env file found or error loading .env file: %v", err)
	}

	requiredVars := []string{
		"DEV_MODE",
		"PORT",
		"DB_CONNECTION_URL",
		"CLIENT_ORIGIN",
		"AUTH_TYPE",
		"AUTH_SERVER_URL",
		"SUPABASE_URL",
		"SUPABASE_API_KEY",
	}

	err = config.CheckRequiredEnvVars(requiredVars)
	if err != nil {
		return nil, err
	}

	return &server.Config{
		DevMode:         config.GetEnvAsBool("DEV_MODE", false),
		Export:          config.GetEnvAsBool("EXPORT", true),
		MultiDevice:     config.GetEnvAsBool("MULTI_DEVICE", true),
		Port:            config.GetEnvAsInt("PORT", 9421),
		DbConnectionUrl: os.Getenv("DB_CONNECTION_URL"), // Required, checked previously
		ClientOrigin:    os.Getenv("CLIENT_ORIGIN"),
		AuthType:        os.Getenv("AUTH_TYPE"),
		AuthServerUrl:   os.Getenv("AUTH_SERVER_URL"),
		SupabaseUrl:     os.Getenv("SUPABASE_URL"),
		SupabaseApiKey:  os.Getenv("SUPABASE_API_KEY"),
	}, nil
}
