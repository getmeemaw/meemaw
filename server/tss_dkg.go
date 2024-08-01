package server

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
	"github.com/getmeemaw/meemaw/utils/ws"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// DkgHandler performs the dkg process from the server side
// goes through the authMiddleware to confirm the access token and get the userId
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
	var origin string
	if server._config.ClientOrigin != "*" {
		u, err := url.Parse(server._config.ClientOrigin)
		if err != nil {
			log.Println("ClientOrigin wrongly configured")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		origin = u.Host + u.Path
	} else {
		origin = "*"
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{origin},
	})
	if err != nil {
		log.Println("DkgHandler - Error accepting websocket:", err)
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
			var msg ws.Message
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
			case ws.PeerIdBroadcastMessage:
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

			case ws.TssMessage:
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

			case ws.MetadataAckMessage:
				// note : have a timer somewhere, if after X seconds we don't have this message, then it means the process failed.
				log.Println("DkgHandler - received MetadataAckMessage")

				close(serverDone)

			default:
				log.Println("Unexpected message type:", msg.Type)
				errorMsg := ws.Message{Type: ws.ErrorMessage, Msg: "error: Unexpected message type"}
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
	go ws.TssSend(dkg.GetNextMessageToSend, serverDone, errs, ctx, c, "DkgHandler")

	// Start Adder process.
	dkgResult, err := dkg.Process()
	if err != nil {
		log.Println("Error while adder process:", err)
		c.Close(websocket.StatusInternalError, "adder process failed")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Error management
	err = ws.ProcessErrors(errs, ctx, c, "DkgHandler")
	if err != nil {
		c.Close(websocket.StatusInternalError, "DkgHandler process failed")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
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
	ack := ws.Message{
		Type: ws.MetadataMessage,
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

	// CLOSE WEBSOCKET
	c.Close(websocket.StatusNormalClosure, "dkg process finished successfully")
}
