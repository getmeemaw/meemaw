package server

import (
	"log"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/CAFxX/httpcompression"
	"github.com/getmeemaw/meemaw/server/vault"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/patrickmn/go-cache"
	"github.com/rs/cors"
)

type Server struct {
	_vault  *vault.Vault
	_cache  *cache.Cache
	_config *Config
	_router *chi.Mux
}

// NewServer creates a new server object used in the "cmd" package and in tests
func NewServer(vault *vault.Vault, config *Config, logging bool) *Server {
	server := Server{
		_vault:  vault,
		_cache:  cache.New(2*time.Minute, 3*time.Minute),
		_config: config,
	}

	_cors := cors.New(cors.Options{
		AllowedOrigins:   []string{server._config.ClientOrigin},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})

	r := chi.NewRouter()

	// global middlewares
	if logging {
		r.Use(middleware.Logger)
	}
	r.Use(_cors.Handler)
	r.Use(server.headerMiddleware)

	// debug rpc
	r.HandleFunc("/rpc", server.RpcHandler)

	// wasm
	compress, err := httpcompression.DefaultAdapter()
	if err != nil {
		panic(err)
	}
	r.With(compress).Get("/meemaw.wasm", server.ServeWasm)

	// tss endpoints
	r.With(server.identityMiddleware).Get("/identify", server.IdentifyHandler)
	r.With(server.identityMiddleware).Get("/authorize", server.AuthorizeHandler)
	r.With(server.authMiddleware).Get("/dkg", server.DkgHandler)
	r.With(server.authMiddleware).Get("/sign", server.SignHandler)
	r.With(server.authMiddleware).Post("/recover", server.RecoverHandler)

	server._router = r

	return &server
}

// Router returns the router of the server (useful for tests)
func (server *Server) Router() http.Handler {
	return server._router
}

// Vault returns the vault of the server (useful for tests)
func (server *Server) Vault() *vault.Vault {
	return server._vault
}

// Start starts the web server on given port
func (server *Server) Start() {
	log.Println("Starting server on port", server._config.Port)

	if !server._config.DevMode {

		// Check that all communications happen through https
		if !strings.Contains(server._config.AuthServerUrl, "https") || !strings.Contains(server._config.SupabaseUrl, "https") || !strings.Contains(server._config.ClientOrigin, "https") {
			log.Fatal("Server not in dev mode and not all targets are https")
		}

		// Check that auth config is complete
		if server._config.AuthType == "supabase" {
			if len(server._config.SupabaseApiKey) == 0 || len(server._config.SupabaseUrl) == 0 {
				log.Fatal("Missing Supabase config")
			}
		} else if server._config.AuthType == "custom" {
			if len(server._config.AuthServerUrl) == 0 {
				log.Fatal("Missing custom auth url")
			}
		} else {
			log.Fatal("Unknown auth")
		}
	}

	if runtime.GOOS == "darwin" {
		http.ListenAndServe("localhost:"+strconv.Itoa(server._config.Port), server._router)
	} else {
		http.ListenAndServe(":"+strconv.Itoa(server._config.Port), server._router)
	}
}

type Config struct {
	DevMode         bool
	Port            int
	DbConnectionUrl string
	ClientOrigin    string
	AuthType        string
	AuthServerUrl   string
	SupabaseUrl     string
	SupabaseApiKey  string
}
