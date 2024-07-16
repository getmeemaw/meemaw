package server

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// DkgHandler performs the dkg process from the server side
// goes through the authMiddleware to confirm the access token and get the userId
// stores the result of dkg in DB (new wallet)
// func (server *Server) DkgHandler(w http.ResponseWriter, r *http.Request) {
// 	// Get userId and access token from context
// 	userId, ok := r.Context().Value(types.ContextKey("userId")).(string)
// 	if !ok {
// 		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
// 		return
// 	}

// 	token, ok := r.Context().Value(types.ContextKey("token")).(string)
// 	if !ok {
// 		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
// 		return
// 	}

// 	// Check if no existing wallet for that user
// 	// Note : update when implementing multi-device
// 	err := server._vault.WalletExists(r.Context(), userId)
// 	if err == nil {
// 		log.Println("Wallet already exists for that user.")
// 		http.Error(w, "Conflict", http.StatusConflict)
// 		return
// 	} else if err != sql.ErrNoRows {
// 		log.Println("Error when getting user for dkg, but not sql.ErrNoRows although it should:", err)
// 		http.Error(w, "Conflict", http.StatusConflict)
// 		return
// 	}

// 	clientPeerID := "client" // UPDATE : get it from client

// 	// Prepare DKG process
// 	dkg, err := tss.NewServerDkg(clientPeerID)
// 	if err != nil {
// 		log.Println("Error when creating new server dkg:", err)
// 		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
// 		return
// 	}

// 	// Parse clientOrigin URL (to remove scheme from it)
// 	u, err := url.Parse(server._config.ClientOrigin)
// 	if err != nil {
// 		log.Println("ClientOrigin wrongly configured")
// 		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
// 		return
// 	}

// 	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
// 		OriginPatterns: []string{u.Host + u.Path},
// 	})
// 	if err != nil {
// 		log.Println("Error accepting websocket:", err)
// 		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
// 		return
// 	}
// 	defer c.Close(websocket.StatusInternalError, "the sky is falling")

// 	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
// 	defer cancel()

// 	errs := make(chan error, 2)

// 	go tss.Send(dkg, ctx, errs, c)
// 	go tss.Receive(dkg, ctx, errs, c)

// 	// Start DKG process.
// 	dkgResult, err := dkg.Process()
// 	if err != nil {
// 		log.Println("Error whil dkg process:", err)
// 		c.Close(websocket.StatusInternalError, "dkg process failed")
// 		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
// 		return
// 	}

// 	// Error management
// 	select {
// 	case processErr := <-errs:
// 		if websocket.CloseStatus(processErr) == websocket.StatusNormalClosure {
// 			log.Println("websocket closed normally") // Should not really happen on server side (server is closing)
// 		} else if ctx.Err() == context.Canceled {
// 			log.Println("websocket closed by context cancellation:", processErr)
// 			c.Close(websocket.StatusInternalError, "dkg process failed")
// 			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
// 			return
// 		} else {
// 			log.Println("error during websocket connection:", processErr)
// 			c.Close(websocket.StatusInternalError, "dkg process failed")
// 			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
// 			return
// 		}
// 	default:
// 		log.Println("no error during TSS")
// 	}

// 	time.Sleep(time.Second) // let the dkg process finish cleanly on client side

// 	log.Println("dkgHandler - storing dkg results")

// 	// Store dkgResult
// 	userAgent := r.UserAgent()
// 	metadata, err := server._vault.StoreWallet(r.Context(), userAgent, userId, dkgResult) // use context from request
// 	if err != nil {
// 		log.Println("Error while storing dkg result:", err)
// 		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
// 		return
// 	}

// 	log.Println("dkgHandler - dkg results stored")
// 	log.Println("dkgHandler - metadata:", metadata)

// 	log.Println("closing websocket")
// 	c.Close(websocket.StatusNormalClosure, "dkg process finished successfully")
// 	cancel()

// 	// Delete token from cache to avoid re-use
// 	// NO : delete after the rest is queried (metadata, cleanup)
// 	// server._cache.Delete(token)

// 	// replace parameters stored for token
// 	params := tokenParameters{
// 		userId:   userId,
// 		metadata: metadata,
// 	}
// 	err = server._cache.Replace(token, params, cache.DefaultExpiration) // Add wallet identifier => if client has an issue storing, revert in DB
// 	if err != nil {
// 		log.Println("Error while setting params:", err) // have _cache.set instead? Do we care about upserts?
// 		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
// 		return
// 	}

// 	// Note: DO NOT return the dkgResult as the client will have its own version with a different share!
// }

// // DkgTwoHandler returns the metadata for the Dkg process
// // In the future: verifies that client has been able to store wallet; if not, remove in DB
// // Idea: "validated" status for the wallet, which becomes True after calling DkgTwo; if False, can be overwritten
// func (server *Server) DkgTwoHandler(w http.ResponseWriter, r *http.Request) {
// 	token, ok := r.Context().Value(types.ContextKey("token")).(string)
// 	if !ok {
// 		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
// 		return
// 	}

// 	// Find the token in cache
// 	paramsInterface, found := server._cache.Get(token)
// 	if !found {
// 		http.Error(w, "The access token does not exist", http.StatusUnauthorized)
// 		return
// 	}

// 	tokenParams, ok := paramsInterface.(tokenParameters)
// 	if !ok {
// 		http.Error(w, "could not unmarshal params", http.StatusBadRequest)
// 		return
// 	}

// 	server._cache.Delete(token)

// 	log.Println("dkgtwo - metadata:", tokenParams.metadata)

// 	w.Write([]byte(tokenParams.metadata))
// }

func (server *Server) DkgHandler(w http.ResponseWriter, r *http.Request) {
	// Get userId and access token from context
	userId, ok := r.Context().Value(types.ContextKey("userId")).(string)
	if !ok {
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	log.Println("DkgHandler userId:", userId)

	// Check if no existing wallet for that user
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

	// WS connection

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

	serverDone := make(chan struct{})
	startTss := make(chan struct{}) // used to avoid polling for next messages until the tss process starts
	errs := make(chan error, 2)

	var clientPeerID string

	var dkg *tss.ServerDkg

	var stage uint32 = 0

	go func() {
		for {
			log.Println("DkgHandler - wsjson.Read")
			var msg Message
			err := wsjson.Read(ctx, c, &msg)
			if err != nil {
				// Check if the context was canceled
				if ctx.Err() == context.Canceled {
					log.Println("read operation canceled")
					return
				}

				// Check if the WebSocket was closed normally
				closeStatus := websocket.CloseStatus(err)
				if closeStatus == websocket.StatusNormalClosure || closeStatus == websocket.StatusGoingAway {
					log.Println("WebSocket closed normally")
					return
				}

				// Handle other errors
				log.Println("DkgHandler - error reading message from websocket:", err)
				log.Println("websocket.CloseStatus(err):", closeStatus)
				errs <- err
				return
			}

			// log.Println("received message in DkgHandler:", msg)

			switch msg.Type {
			case PeerIdBroadcastMessage:
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("PeerIdBroadcastMessage message but we're at later stage; stage:", stage)
					continue
				}

				clientPeerID = string(msg.Msg)

				log.Println("DkgHandler - received PeerIdBroadcastMessage:", clientPeerID)

				// Prepare DKG process
				dkg, err = tss.NewServerDkg(clientPeerID)
				if err != nil {
					log.Println("Error when creating new server dkg:", err)
					errs <- err
					return
				}

				startTss <- struct{}{}

				stage = 30

			case TssMessage:
				// verify stage : if tss message but we're at storage stage or further, discard
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("TSS message but we're at later stage; stage:", stage)
					continue
				}

				log.Println("DkgHandler - received TssMessage:", msg)

				// Decode TSS msg
				byteString, err := hex.DecodeString(msg.Msg)
				if err != nil {
					log.Println("error decoding hex:", err)
					errs <- err
					return
				}

				tssMsg := &tss.Message{}
				err = json.Unmarshal(byteString, &tssMsg)
				if err != nil {
					log.Println("could not unmarshal tss msg:", err)
					errs <- err
					return
				}

				// Handle tss message (NOTE : will automatically, in ServerAdd.HandleMessage, redirect to other client if needs be)
				err = dkg.HandleMessage(tssMsg)
				if err != nil {
					log.Println("could not handle tss msg:", err)
					errs <- err
					return
				}

			case MetadataAckMessage:
				// note : have a timer somewhere, if after X seconds we don't have this message, then it means the process failed.
				log.Println("DkgHandler - received MetadataAckMessage")

				close(serverDone)

			default:
				log.Println("Unexpected message type:", msg.Type)
				errorMsg := Message{Type: ErrorMessage, Msg: "error: Unexpected message type"}
				err := wsjson.Write(ctx, c, errorMsg)
				if err != nil {
					log.Println("error writing json through websocket:", err)
					errs <- err
					return
				}
			}
		}
	}()

	// Wait for tss start
	<-startTss

	// Get channel from adder /!\ needs to be initialised first => this line needs to be after <-startTss
	log.Println("DkgHandler - trying to GetDoneChan()")

	// TSS sending and listening for finish signal
	go func() {
		for {
			select {
			case <-serverDone: // for the server, don't stop communcation straight when TSS is done (communication can still happen between clients)
				return
			default:
				tssMsg, err := dkg.GetNextMessageToSend()
				if err != nil {
					if strings.Contains(err.Error(), "no message to be sent") {
						continue
					}
					log.Println("RegisterDeviceHandler - error getting next message:", err)
					errs <- err
					return
				}

				if len(tssMsg.PeerID) == 0 {
					continue
				}

				log.Println("DkgHandler - got next message to send to", clientPeerID, ":", tssMsg)

				// format message for communication
				jsonEncodedMsg, err := json.Marshal(tssMsg)
				if err != nil {
					log.Println("could not marshal tss msg:", err)
					errs <- err
					return
				}

				payload := hex.EncodeToString(jsonEncodedMsg)

				msg := Message{
					Type: TssMessage,
					Msg:  payload,
				}

				// log.Println("trying send, next encoded message to send:", encodedMsg)

				if tssMsg.Message != nil {
					// log.Println("trying to send message:", encodedMsg)
					err := wsjson.Write(ctx, c, msg)
					if err != nil {
						log.Println("error writing json through websocket:", err)
						errs <- err
						return
					}
				}

				time.Sleep(10 * time.Millisecond) // UPDATE : remove polling, use channels to trigger send when next TSS message ready
			}
		}
	}()

	// Start Adder process.
	dkgResult, err := dkg.Process()
	if err != nil {
		log.Println("Error while adder process:", err)
		c.Close(websocket.StatusInternalError, "adder process failed")
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
		log.Println("DkgHandler - no error during TSS")
	}

	stage = 40 // only move to next stage after tss process is done

	log.Println("DkgHandler - storing wallet")

	// Store dkgResult
	userAgent := r.UserAgent()
	metadata, err := server._vault.StoreWallet(r.Context(), userId, clientPeerID, userAgent, dkgResult) // use context from request
	if err != nil {
		log.Println("Error while storing dkg result:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Println("DkgHandler - sending metadata")

	// Send metadata to client
	ack := Message{
		Type: MetadataMessage,
		Msg:  metadata,
	}
	err = wsjson.Write(ctx, c, ack)
	if err != nil {
		log.Println("error writing json through websocket:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Println("DkgHandler - metadata sent")

	// Wait for the client to respond with MetadataAckMessage (note: timer so that we remove wallet from DB if never get ack ?)
	<-serverDone
	cancel()

	log.Println("DkgHandler - serverDone, closing")

	// Timer to verify that we get what we need from client ? if not, remove stuff if we need to.

	// CLOSE WEBSOCKET
	c.Close(websocket.StatusNormalClosure, "dkg process finished successfully")
}
