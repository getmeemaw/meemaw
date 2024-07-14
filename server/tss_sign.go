package server

import (
	"context"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
	"nhooyr.io/websocket"
)

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
	clientPeerID := params.Get("peer")
	// clientPeerID := "client"

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
	signer, err := tss.NewServerSigner(clientPeerID, dkgResult.Pubkey, dkgResult.Share, dkgResult.BKs, message)
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
