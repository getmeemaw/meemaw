package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/CAFxX/httpcompression"
	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/patrickmn/go-cache"
)

type Server struct {
	_vault         Vault
	_cache         *cache.Cache
	_config        *Config
	_wasm          []byte
	_router        *chi.Mux
	_getAuthConfig func(context.Context, *Server) (*AuthConfig, error)
}

type Vault interface {
	WalletExists(context.Context, string) error
	StoreWallet(context.Context, string, string, *tss.DkgResult) (string, error)
	RetrieveWallet(context.Context, string) (*tss.DkgResult, error)
}

// NewServer creates a new server object used in the "cmd" package and in tests
func NewServer(vault Vault, config *Config, wasmBinary []byte, logging bool) *Server {
	server := Server{
		_vault:  vault,
		_cache:  cache.New(2*time.Minute, 3*time.Minute),
		_config: config,
		_wasm:   wasmBinary,
	}

	// Auth Config
	server._getAuthConfig = func(ctx context.Context, server *Server) (*AuthConfig, error) {
		return &AuthConfig{
			AuthType:       server._config.AuthType,
			AuthServerUrl:  server._config.AuthServerUrl,
			SupabaseUrl:    server._config.SupabaseUrl,
			SupabaseApiKey: server._config.SupabaseApiKey,
		}, nil
	}

	// Router

	r := chi.NewRouter()

	// global middlewares
	if logging {
		r.Use(middleware.Logger)
	}
	r.Use(server.corsMiddleware)
	// r.Use(cors.Default().Handler)
	r.Use(server.headerMiddleware)

	// debug rpc
	r.HandleFunc("/rpc", server.RpcHandler)

	// wasm
	compress, err := httpcompression.DefaultAdapter()
	if err != nil {
		panic(err)
	}
	r.With(compress).Get("/meemaw.wasm", server.ServeWasm)

	// auth management
	r.With(server.identityMiddleware).Get("/identify", server.IdentifyHandler)
	r.With(server.identityMiddleware).Get("/authorize", server.AuthorizeHandler)

	// dkg
	r.With(server.authMiddleware).Get("/dkg", server.DkgHandler)
	r.With(server.authMiddleware).Get("/dkgtwo", server.DkgTwoHandler)

	// sign
	r.With(server.authMiddleware).Get("/sign", server.SignHandler)

	// export private key
	r.With(server.authMiddleware).Post("/recover", server.RecoverHandler)

	// multi-device
	r.With(server.authMiddleware).Get("/register", server.RegisterDeviceHandler)
	r.With(server.authMiddleware).Get("/accept", server.AcceptDeviceHandler)

	server._router = r

	return &server
}

// Router returns the router of the server (useful for tests)
func (server *Server) Router() http.Handler {
	return server._router
}

// Vault returns the vault of the server (useful for tests)
func (server *Server) Vault() Vault {
	return server._vault
}

// UpdateGetAuthConfig changes the auth config getter
func (server *Server) UpdateGetAuthConfig(getAuthConfig func(context.Context, *Server) (*AuthConfig, error)) {
	server._getAuthConfig = getAuthConfig
}

// AddRoute adds an endpoint to the server. Note that it will go through authMiddleware for security reasons.
func (server *Server) AddRoute(method string, pattern string, h http.HandlerFunc) error {
	if strings.ToLower(method) == "get" {
		server._router.With(server.authMiddleware).Get(pattern, h)
		return nil
	} else if strings.ToLower(method) == "post" {
		server._router.With(server.authMiddleware).Post(pattern, h)
		return nil
	} else {
		return errors.New("method not recognized")
	}
}

// Start starts the web server on given port
func (server *Server) Start() {
	log.Println("Starting server on port", server._config.Port)

	if !server._config.DevMode {

		// Check that all communications happen through https
		if !strings.Contains(server._config.AuthServerUrl, "https") || !strings.Contains(server._config.SupabaseUrl, "https") || !strings.Contains(server._config.ClientOrigin, "https") {
			log.Fatal("Server not in dev mode and not all targets are https")
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

func (server *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", server._config.ClientOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, M-METADATA")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
