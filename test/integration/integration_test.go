package integration

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/getmeemaw/meemaw/server"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// This file is the entry point for all tests in this directory.
// All integration tests requiring the interaction of multiple components should live here, all unit tests should live inside each component's directory.

// For Github Actions : check https://github.com/ory/dockertest "Running Dockertest Using GitHub Actions"

var db *sql.DB
var logging bool

// TestMain is called before any other tests are run. It is reponsible for launching the other tests with m.Run()
// TestMain creates a DB using Docker & adds the server Schema to that db before running the tests. It then deletes that db and stops the container.
func TestMain(m *testing.M) {
	// Verbose ? (check from args)
	for _, arg := range os.Args {
		if strings.Contains(arg, "-test.v=true") {
			log.Println("Logging enabled")
			logging = true
			break
		}
	}

	if !logging {
		log.Println("Logging disabled")
		log.SetOutput(io.Discard) // Discard all logs
	}

	////////
	/// Spin up test replica DB in container
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not construct pool: %s", err)
	}

	err = pool.Client.Ping()
	if err != nil {
		log.Fatalf("Could not connect to Docker: %s", err)
	}

	// pulls an image, creates a container based on it and runs it
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "11",
		Env: []string{
			"POSTGRES_PASSWORD=secret",
			"POSTGRES_USER=user_name",
			"POSTGRES_DB=dbname",
			"listen_addresses = '*'",
		},
	}, func(config *docker.HostConfig) {
		// set AutoRemove to true so that stopped container goes away by itself
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		log.Fatalf("Could not start resource: %s", err)
	}

	hostAndPort := resource.GetHostPort("5432/tcp")
	databaseUrl := fmt.Sprintf("postgres://user_name:secret@%s/dbname?sslmode=disable", hostAndPort)

	log.Println("Connecting to database on url: ", databaseUrl)

	resource.Expire(120) // Tell docker to hard kill the container in 120 seconds

	// exponential backoff-retry, because the application in the container might not be ready to accept connections yet
	pool.MaxWait = 120 * time.Second
	if err = pool.Retry(func() error {
		log.Println("trying connecting to db")
		db, err = sql.Open("pgx", databaseUrl)
		if err != nil {
			return err
		}
		return db.Ping()
	}); err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	// Loading schema
	log.Println("Loading schema")
	err = server.LoadSchema(db)
	if err != nil {
		log.Fatalf("Could not load schema: %s", err)
	}

	////////
	/// Run tests
	log.Println("Running tests")
	code := m.Run()

	////////
	/// Clean after tests (DB)

	// You can't defer this because os.Exit doesn't care for defer
	log.Println("Cleaning docker containers")
	if err := pool.Purge(resource); err != nil {
		log.Fatalf("Could not purge resource: %s", err)
	}

	os.Exit(code)
}
