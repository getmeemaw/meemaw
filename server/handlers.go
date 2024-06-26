package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
	"github.com/google/uuid"
	"github.com/patrickmn/go-cache"
	"nhooyr.io/websocket"
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
		log.Println("Authorization info not found")
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
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	// Get metadata from context
	metadata, ok := r.Context().Value(types.ContextKey("metadata")).(string)
	if !ok {
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
				http.Error(w, "Secure connection required", http.StatusUnauthorized)
				return
			}
		}

		// Extract the token from the URL query
		params := r.URL.Query()
		tokenParam, ok := params["token"]
		if !ok || len(tokenParam) == 0 {
			http.Error(w, "You need to provide an access token", http.StatusUnauthorized)
			return
		}

		// Find the userId related to the token in cache
		paramsInterface, found := server._cache.Get(tokenParam[0])
		if !found {
			http.Error(w, "The access token does not exist", http.StatusUnauthorized)
			return
		}

		tokenParams, ok := paramsInterface.(tokenParameters)
		if !ok {
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

// DkgHandler performs the dkg process from the server side
// goes through the authMiddleware to confirm the access token and get the userId
// stores the result of dkg in DB (new wallet)
func (server *Server) DkgHandler(w http.ResponseWriter, r *http.Request) {
	// Get userId and access token from context
	userId, ok := r.Context().Value(types.ContextKey("userId")).(string)
	if !ok {
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	token, ok := r.Context().Value(types.ContextKey("token")).(string)
	if !ok {
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	// Check if no existing wallet for that user
	// Note : update when implementing multi-device
	err := server._vault.WalletExists(r.Context(), userId)
	if err == nil {
		log.Println("Wallet already exists for that user.")
		http.Error(w, "Conflict", http.StatusConflict)
		return
	} else if err != sql.ErrNoRows {
		log.Println("Error when getting user for dkg, but not sql.ErrNoRows although it should:", err)
		http.Error(w, "Conflict", http.StatusConflict)
		return
	}

	// Prepare DKG process
	dkg, err := tss.NewServerDkg()
	if err != nil {
		log.Println("Error when creating new server dkg:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Parse clientOrigin URL (to remove scheme from it)
	u, err := url.Parse(server._config.ClientOrigin)
	if err != nil {
		log.Println("ClientOrigin wrongly configured")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{u.Host + u.Path},
	})
	if err != nil {
		log.Println("Error accepting websocket:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer c.Close(websocket.StatusInternalError, "the sky is falling")

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	errs := make(chan error, 2)

	go tss.Send(dkg, ctx, errs, c)
	go tss.Receive(dkg, ctx, errs, c)

	// Start DKG process.
	dkgResult, err := dkg.Process()
	if err != nil {
		log.Println("Error whil dkg process:", err)
		c.Close(websocket.StatusInternalError, "dkg process failed")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Error management
	select {
	case processErr := <-errs:
		if websocket.CloseStatus(processErr) == websocket.StatusNormalClosure {
			log.Println("websocket closed normally") // Should not really happen on server side (server is closing)
		} else if ctx.Err() == context.Canceled {
			log.Println("websocket closed by context cancellation:", processErr)
			c.Close(websocket.StatusInternalError, "dkg process failed")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		} else {
			log.Println("error during websocket connection:", processErr)
			c.Close(websocket.StatusInternalError, "dkg process failed")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	default:
		log.Println("no error during TSS")
	}

	time.Sleep(time.Second) // let the dkg process finish cleanly on client side

	log.Println("storing dkg results")

	// Store dkgResult
	userAgent := r.UserAgent()
	metadata, err := server._vault.StoreWallet(r.Context(), userAgent, userId, dkgResult) // use context from request
	if err != nil {
		log.Println("Error while storing dkg result:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Println("dkg results stored")
	log.Println("metadata:", metadata)

	log.Println("closing websocket")
	c.Close(websocket.StatusNormalClosure, "dkg process finished successfully")
	cancel()

	// Delete token from cache to avoid re-use
	// NO : delete after the rest is queried (metadata, cleanup)
	// server._cache.Delete(token)

	// replace parameters stored for token
	params := tokenParameters{
		userId:   userId,
		metadata: metadata,
	}
	server._cache.Replace(token, params, cache.DefaultExpiration) // Add wallet identifier => if client has an issue storing, revert in DB

	// Note: DO NOT return the dkgResult as the client will have its own version with a different share!
}

// DkgTwoHandler returns the metadata for the Dkg process
// In the future: verifies that client has been able to store wallet; if not, remove in DB
// Idea: "validated" status for the wallet, which becomes True after calling DkgTwo; if False, can be overwritten
func (server *Server) DkgTwoHandler(w http.ResponseWriter, r *http.Request) {
	token, ok := r.Context().Value(types.ContextKey("token")).(string)
	if !ok {
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	// Find the token in cache
	paramsInterface, found := server._cache.Get(token)
	if !found {
		http.Error(w, "The access token does not exist", http.StatusUnauthorized)
		return
	}

	tokenParams, ok := paramsInterface.(tokenParameters)
	if !ok {
		http.Error(w, "could not unmarshal params", http.StatusBadRequest)
		return
	}

	server._cache.Delete(token)

	w.Write([]byte(tokenParams.metadata))
}

// SignHandler performs the signing process from the server side
// goes through the authMiddleware to confirm the access token and get the userId
// requires a hex-encoded message to be signed (provided in URL parameter)
func (server *Server) SignHandler(w http.ResponseWriter, r *http.Request) {
	// Get userId and access token from context
	userId, ok := r.Context().Value(types.ContextKey("userId")).(string)
	if !ok {
		// If there's no userID in the context, report an error and return.
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	token, ok := r.Context().Value(types.ContextKey("token")).(string)
	if !ok {
		// If there's no token in the context, report an error and return.
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	// Get message to be signed from URL parameters
	params := r.URL.Query()
	msg := params.Get("msg")

	if len(msg) == 0 {
		http.Error(w, "No message to be signed", http.StatusBadRequest)
		return
	}

	message, err := hex.DecodeString(msg)
	if err != nil {
		log.Println("Error decoding msg:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Retrieve wallet from DB for given userId
	dkgResult, err := server._vault.RetrieveWallet(r.Context(), userId) // RetrieveWallet can use metadata from context if required
	if err != nil {
		if errors.Is(err, &types.ErrNotFound{}) {
			http.Error(w, "Wallet does not exist.", http.StatusNotFound)
			return
		} else {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}

	// Prepare signing process
	signer, err := tss.NewServerSigner(dkgResult.Pubkey, dkgResult.Share, dkgResult.BKs, message)
	if err != nil {
		log.Println("Error initialising signer tss:", err)
		if strings.Contains(err.Error(), "invalid point") {
			http.Error(w, "Bad Request", http.StatusBadRequest)
		} else {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	// Parse clientOrigin URL (to remove scheme from it)
	u, err := url.Parse(server._config.ClientOrigin)
	if err != nil {
		log.Println("ClientOrigin wrongly configured")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{u.Host + u.Path},
	})
	if err != nil {
		log.Println("Error accepting websocket:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer c.Close(websocket.StatusInternalError, "the sky is falling")

	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()

	errs := make(chan error, 2)

	go tss.Send(signer, ctx, errs, c)
	go tss.Receive(signer, ctx, errs, c)

	// Start signing process
	_, err = signer.Process()
	if err != nil {
		log.Println("Error launching signer.Process:", err)
		c.Close(websocket.StatusInternalError, "signing process failed")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	time.Sleep(time.Second) // let the signing process finish cleanly on client side

	c.Close(websocket.StatusNormalClosure, "signing process finished successfully")

	// Delete token from cache to avoid re-use
	server._cache.Delete(token)

	// Note: no need to return the signature as the client will have it as well
}

// RecoverHandler recovers the private key from the server and client shares
// goes through the authMiddleware to confirm the access token and get the userId
// requires the client share (provided in URL parameter)
func (server *Server) RecoverHandler(w http.ResponseWriter, r *http.Request) {

	// Verify POST request
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Get userId and access token from context
	userId, ok := r.Context().Value(types.ContextKey("userId")).(string)
	if !ok {
		// If there's no userID in the context, report an error and return.
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	token, ok := r.Context().Value(types.ContextKey("token")).(string)
	if !ok {
		// If there's no token in the context, report an error and return.
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	// Get client share from POST parameters
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}
	clientShareStr := r.FormValue("share")

	if len(clientShareStr) == 0 {
		log.Println("No client share provided")
		http.Error(w, "No client share", http.StatusBadRequest)
		return
	}

	// Retrieve wallet from DB for given userId
	dkgResult, err := server._vault.RetrieveWallet(r.Context(), userId)
	if err != nil {
		if errors.Is(err, &types.ErrNotFound{}) {
			log.Println("Wallet does not exist.")
			http.Error(w, "Wallet does not exist.", http.StatusNotFound)
			return
		} else {
			log.Println("Error while retrieving wallet:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}

	// Recover private key
	privateKey, err := tss.RecoverPrivateKeyWrapper(dkgResult.Pubkey, dkgResult.Share, clientShareStr, dkgResult.BKs)
	if err != nil {
		log.Println("Error recovering private key:", err)
		if strings.Contains(err.Error(), "invalid point") {
			http.Error(w, "Bad Request", http.StatusBadRequest)
		} else {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	// Delete token from cache to avoid re-use
	server._cache.Delete(token)

	// Return private key
	privateKeyBytes := privateKey.D.Bytes()
	privateKeyStr := hex.EncodeToString(privateKeyBytes)

	w.Write([]byte(privateKeyStr))
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
