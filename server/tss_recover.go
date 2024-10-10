package server

import (
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
)

// ExportHandler exports the private key given the server and client shares
// goes through the authMiddleware to confirm the access token and get the userId
// requires the client share (provided as URL parameter)
func (server *Server) ExportHandler(w http.ResponseWriter, r *http.Request) {

	// Verify POST request
	if r.Method != http.MethodPost {
		log.Println("ExportHandler - invalid request method")
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

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

	// Get client share and clientPeerID from POST parameters
	if err := r.ParseForm(); err != nil {
		log.Println("ExportHandler - unable to parse form")
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}
	clientShareStr := r.FormValue("share")
	clientPeerID := r.FormValue("clientPeerID")

	if len(clientShareStr) == 0 || len(clientPeerID) == 0 {
		log.Println("ExportHandler - missing information")
		http.Error(w, "Missing information", http.StatusBadRequest)
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

	// Export private key
	privateKey, err := tss.RecoverPrivateKeyWrapper(clientPeerID, dkgResult.Pubkey, dkgResult.Share, clientShareStr, dkgResult.BKs)
	if err != nil {
		log.Println("ExportHandler - error recovering private key:", err)
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
