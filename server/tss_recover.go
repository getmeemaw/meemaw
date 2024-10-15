package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/getmeemaw/meemaw/utils/types"
)

// ExportHandler exports the server share for the client to be able to generate the private key based on both client & server shares
// goes through the authMiddleware to confirm the access token and get the userId
// NOTE - potential improvement for the future: asymmetric encryption of the server shares based on a public encryption key shared by the client, to avoid MITM attack vectors. However, avoid making the wasm file heavier.
func (server *Server) ExportHandler(w http.ResponseWriter, r *http.Request) {

	// Get userId and access token from context
	userId, ok := r.Context().Value(types.ContextKey("userId")).(string)
	if !ok {
		// If there's no userID in the context, report an error and return.
		log.Println("ExportHandler - authorization info not found")
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	token, ok := r.Context().Value(types.ContextKey("token")).(string)
	if !ok {
		// If there's no token in the context, report an error and return.
		log.Println("ExportHandler - authorization info not found")
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	// Retrieve wallet from DB for given userId
	dkgResult, err := server._vault.RetrieveWallet(r.Context(), userId)
	if err != nil {
		if errors.Is(err, &types.ErrNotFound{}) {
			log.Println("ExportHandler - wallet does not exist")
			http.Error(w, "Wallet does not exist.", http.StatusNotFound)
			return
		} else {
			log.Println("ExportHandler - error while retrieving wallet:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}

	// Return server dkgResult
	ret, err := json.Marshal(dkgResult)
	if err != nil {
		log.Println("ExportHandler - could not marshal dkgResult")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Delete token from cache to avoid re-use
	server._cache.Delete(token)

	w.Write(ret)
}
