package server

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/getmeemaw/meemaw/utils/types"
	"github.com/google/uuid"
	"github.com/patrickmn/go-cache"
)

// identityMiddleware is a middleware used to get the userId from auth provider based on a generic bearer token provided by the client
// used by /identify and /authorize
func (server *Server) identityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Verify https (if not dev mode)
		if !server._config.DevMode {
			if r.URL.Scheme != "https" {
				log.Println("Unsecure connection in prod mode")
				http.Error(w, "Secure connection required", http.StatusUnauthorized)
				return
			}
		}

		// Get Bearer token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || getBearerTokenFromHeader(authHeader) == "" {
			log.Println("Empty auth header")
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		// Auth config
		authConfig, err := server._getAuthConfig(ctx, server)
		if err != nil {
			log.Println("Problem getting auth config, err:", err)
			http.Error(w, "Problem getting auth config", http.StatusBadRequest)
			return
		}

		// Get userId from auth provider, based on Bearer token
		userId, err := server.authProviders(authConfig, getBearerTokenFromHeader(authHeader))
		if err != nil {
			log.Println("Problem during the authorization, err:", err)
			http.Error(w, "Invalid auth token", http.StatusUnauthorized)
			// NOTE : we're loosing all error details (400 vs 401 vs 404). What do we really want?
			return
		}

		// Store userId in context for next request in the stack

		ctx = context.WithValue(ctx, types.ContextKey("userId"), userId)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ServeWasm is responsible for serving the wasm module
func (server *Server) ServeWasm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/wasm")
	w.Header().Set("Access-Control-Allow-Origin", server._config.ClientOrigin)
	w.Write(server._wasm)
}

// IdentifyHandler is responsible for getting a unique identifier of a user from the auth provider
// It uses identityMiddleware to get the userId from auth provider based on a generic bearer token provided by the client, then returns it
func (server *Server) IdentifyHandler(w http.ResponseWriter, r *http.Request) {
	// Get userId from context
	userId, ok := r.Context().Value(types.ContextKey("userId")).(string)
	if !ok {
		log.Println("IdentifyHandler - userId not found in context")
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	// Return encrypted userId
	w.Write([]byte(userId))
}

type tokenParameters struct {
	userId   string
	metadata string
}

// AuthorizeHandler is responsible for creating an access token allowing for a tss request to be performed
// It uses identityMiddleware to get the userId from auth provider based on a generic bearer token provided by the client
// It then creates an access token linked to that userId, stores it in cache and returns it
func (server *Server) AuthorizeHandler(w http.ResponseWriter, r *http.Request) {
	// Get userId from context
	userId, ok := r.Context().Value(types.ContextKey("userId")).(string)
	if !ok {
		log.Println("AuthorizeHandler - userId not found in context")
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	// Get metadata from context
	metadata, ok := r.Context().Value(types.ContextKey("metadata")).(string)
	if !ok {
		log.Println("AuthorizeHandler - metadata not found in context")
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	// Create access token and store parameters in cache
	accessToken := uuid.New().String()
	params := tokenParameters{
		userId:   userId,
		metadata: metadata,
	}

	server._cache.Set(accessToken, params, cache.DefaultExpiration)

	// Return access token
	w.Write([]byte(accessToken))
}

// authMiddleware returns the userId associated with the given access token
// blocks access if no token provided
func (server *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify wss (if not dev mode)
		if !server._config.DevMode {
			if r.URL.Scheme != "wss" {
				log.Println("authMiddleware - secure connection required")
				http.Error(w, "Secure connection required", http.StatusUnauthorized)
				return
			}
		}

		// Extract the token from the URL query
		params := r.URL.Query()
		tokenParam, ok := params["token"]
		if !ok || len(tokenParam) == 0 {
			log.Println("authMiddleware - you need to provide an access token")
			http.Error(w, "You need to provide an access token", http.StatusUnauthorized)
			return
		}

		// Find the userId related to the token in cache
		paramsInterface, found := server._cache.Get(tokenParam[0])
		if !found {
			log.Println("authMiddleware - access token does not exist")
			http.Error(w, "The access token does not exist", http.StatusUnauthorized)
			return
		}

		tokenParams, ok := paramsInterface.(tokenParameters)
		if !ok {
			log.Println("authMiddleware - could not infer tokenParameters type")
			http.Error(w, "Issue during authorization", http.StatusBadRequest)
			return
		}

		// Add the userId and token to the context
		ctx := r.Context()
		ctx = context.WithValue(ctx, types.ContextKey("userId"), tokenParams.userId)
		ctx = context.WithValue(ctx, types.ContextKey("metadata"), tokenParams.metadata)
		ctx = context.WithValue(ctx, types.ContextKey("token"), tokenParam[0])
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getBearerTokenFromHeader(header string) string {
	ret := strings.Replace(header, "Bearer", "", 1)
	ret = strings.Replace(ret, " ", "", 1)
	return ret
}

// RpcHandler is used for debug operations : it logs every RPC-JSON requests and the return value
func (server *Server) RpcHandler(w http.ResponseWriter, r *http.Request) {

	// Log the incoming request details
	log.Println("Received RPC request:", r.Method, r.URL.Path)

	// Proxy the request to Alchemy
	url := "https://eth-sepolia.g.alchemy.com/v2/6dMGxuEv2875AnJoXy2dy-5swIeK7WGG"
	client := &http.Client{}
	req, err := http.NewRequest(r.Method, url, r.Body)
	if err != nil {
		log.Println("error creating new request:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	response, err := client.Do(req)
	if err != nil {
		log.Println("error transmitting rpc call:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	defer response.Body.Close()

	// Read the response body
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		log.Println("error reading response body of rpc call:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Print the response body
	log.Println("Response body:", string(bodyBytes))

	// Create a new reader with the body bytes for io.Copy
	bodyReader := bytes.NewReader(bodyBytes)

	_, err = io.Copy(w, bodyReader)
	if err != nil {
		log.Println("error copying body:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

const headerPrefix = "M-"

// headerMiddleware is a middleware used to transfer Meemaw headers to context
func (server *Server) headerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create a new context from the request context
		ctx := r.Context()

		// Extract headers with the specific format
		for name, values := range r.Header {
			if strings.HasPrefix(name, headerPrefix) && len(values) > 0 {
				// Remove the prefix and use the remaining part as the context key
				key := strings.ToLower(strings.TrimPrefix(name, headerPrefix))
				// Add the first header value to the context
				ctx = context.WithValue(ctx, types.ContextKey(key), values[0])
			}
		}

		// Pass the context to the next handler in the chain
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
