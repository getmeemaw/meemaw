package server

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
	"github.com/patrickmn/go-cache"
	"nhooyr.io/websocket"
)

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

	clientPeerID := "client" // UPDATE : get it from client

	// Prepare DKG process
	dkg, err := tss.NewServerDkg(clientPeerID)
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

	log.Println("dkgHandler - storing dkg results")

	// Store dkgResult
	userAgent := r.UserAgent()
	metadata, err := server._vault.StoreWallet(r.Context(), userAgent, userId, dkgResult) // use context from request
	if err != nil {
		log.Println("Error while storing dkg result:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Println("dkgHandler - dkg results stored")
	log.Println("dkgHandler - metadata:", metadata)

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
	err = server._cache.Replace(token, params, cache.DefaultExpiration) // Add wallet identifier => if client has an issue storing, revert in DB
	if err != nil {
		log.Println("Error while setting params:", err) // have _cache.set instead? Do we care about upserts?
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

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

	log.Println("dkgtwo - metadata:", tokenParams.metadata)

	w.Write([]byte(tokenParams.metadata))
}
