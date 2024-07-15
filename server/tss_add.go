package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
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

type MessageType struct { // should be in utils/ws.go ? (with the rest below)
	MsgType  string `json:"msgType"`
	MsgStage uint32 `json:"msgStage"`
}

var (
	PeerIdBroadcastMessage        = MessageType{MsgType: "peer", MsgStage: 10}
	DeviceMessage                 = MessageType{MsgType: "device", MsgStage: 20}       // new device to server (=> respond with pubkey)
	PubkeyMessage                 = MessageType{MsgType: "pubkey", MsgStage: 20}       // server to new device (=> start TSS on new device)
	PubkeyAckMessage              = MessageType{MsgType: "pubkey-ack", MsgStage: 30}   // new device to server (=> start TSS msg management on server registerHandler)
	MetadataMessage               = MessageType{MsgType: "metadata", MsgStage: 20}     // old device to server (before TSS) ; server to new device (after TSS)
	MetadataAckMessage            = MessageType{MsgType: "metadata-ack", MsgStage: 30} // server to old device (=> start TSS)
	TssMessage                    = MessageType{MsgType: "tss", MsgStage: 40}
	TssDoneMessage                = MessageType{MsgType: "tss-done", MsgStage: 50}
	EverythingStoredClientMessage = MessageType{MsgType: "stored-client", MsgStage: 70}
	ExistingDeviceDoneMessage     = MessageType{MsgType: "existing-device-done", MsgStage: 80}
	NewDeviceDoneMessage          = MessageType{MsgType: "new-device-done", MsgStage: 80}
	ErrorMessage                  = MessageType{MsgType: "error", MsgStage: 0}
)

type Message struct {
	Type MessageType `json:"type"`
	Msg  string      `json:"payload"`
}

type PublicWallet struct {
	PublicKey tss.PubkeyStr
	BKs       map[string]tss.BK
}

// UPDATE DESCRIPTION
func (server *Server) RegisterDeviceHandler(w http.ResponseWriter, r *http.Request) {
	// Get userId and access token from context
	userId, ok := r.Context().Value(types.ContextKey("userId")).(string)
	if !ok {
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	log.Println("RegisterDeviceHandler userId:", userId)

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

	newClientPeerIdCh := make(chan string, 1)
	existingClientPeerIdCh := make(chan string, 1)
	metadataCh := make(chan string, 1)
	adderCh := make(chan *tss.ServerAdd, 1)
	existingDeviceTssDoneCh := make(chan struct{}, 1)
	newDeviceDoneCh := make(chan struct{}, 1)
	existingDeviceDoneCh := make(chan struct{}, 1)

	server._cache.Set(userId+"-newclientpeeridch", newClientPeerIdCh, 10*time.Minute)
	server._cache.Set(userId+"-existingclientpeeridch", existingClientPeerIdCh, 10*time.Minute)
	server._cache.Set(userId+"-metadatach", metadataCh, 10*time.Minute)
	server._cache.Set(userId+"-adderch", adderCh, 10*time.Minute)
	server._cache.Set(userId+"-existingdevicetssdonech", existingDeviceTssDoneCh, 10*time.Minute)
	server._cache.Set(userId+"-newdevicedonech", newDeviceDoneCh, 10*time.Minute)
	server._cache.Set(userId+"-existingdevicedonech", existingDeviceDoneCh, 10*time.Minute)

	var metadata string
	var newClientPeerID string
	var existingClientPeerID string

	var adder *tss.ServerAdd

	var stage uint32 = 0

	go func() {
		for {
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
				log.Println("RegisterDeviceHandler - error reading message from websocket:", err)
				log.Println("websocket.CloseStatus(err):", closeStatus)
				errs <- err
				return
			}

			// log.Println("received message in RegisterDeviceHandler:", msg)

			switch msg.Type {
			case PeerIdBroadcastMessage:
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("Device message but we're at later stage; stage:", stage)
					continue
				}

				newClientPeerID = string(msg.Msg)

				newClientPeerIdCh <- newClientPeerID
				existingClientPeerID = <-existingClientPeerIdCh

				PeerIdBroadcastMsg := Message{
					Type: PeerIdBroadcastMessage,
					Msg:  existingClientPeerID,
				}
				err = wsjson.Write(ctx, c, PeerIdBroadcastMsg)
				if err != nil {
					log.Println("RegisterDevice - deviceMsg - error writing json through websocket:", err)
					errs <- err
					return
				}

			case DeviceMessage:
				// verify stage : if device message but we're at tss stage or further, discard
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("Device message but we're at later stage; stage:", stage)
					continue
				}

				// READ DEVICE from message
				device := string(msg.Msg) // verify if sufficient

				log.Println("RegisterDeviceHandler - got device from client:", device)

				// WAIT FOR METADATA (channel between registerDeviceHandler & acceptDeviceHandler) AND START ADDER ??
				metadata = <-metadataCh

				log.Println("RegisterDeviceHandler - got metadata from channel:", metadata)

				// IMPORTANT : needs to be done here, as we don't have the metadata beforehand (=> add metadata to context)
				// Retrieve wallet from DB for given userId
				dkgResult, err := server._vault.RetrieveWallet(context.WithValue(r.Context(), types.ContextKey("metadata"), metadata), userId) // RetrieveWallet can use metadata from context if required
				if err != nil {
					log.Println("could not retrieve wallet:", err)
					if errors.Is(err, &types.ErrNotFound{}) {
						http.Error(w, "Wallet does not exist.", http.StatusNotFound)
						return
					} else {
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
						return
					}
				}

				log.Println("RegisterDeviceHandler - wallet retrieved:", dkgResult)
				log.Println("RegisterDeviceHandler - wallet retrieved share:", dkgResult.Share)

				// Prepare Adding process
				adder, err = tss.NewServerAdd(newClientPeerID, existingClientPeerID, dkgResult.Pubkey, dkgResult.Share, dkgResult.BKs)
				if err != nil {
					log.Println("Error when creating new server Add:", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}

				log.Println("RegisterDeviceHandler - adder created")

				// Adder & device identifier in Cache (to be used by AcceptDeviceHandler concurrently)
				// server._cache.Set(userId+"-adder", adder, 10*time.Minute)
				server._cache.Set(userId+"-device", device, 10*time.Minute)
				adderCh <- adder

				log.Println("RegisterDeviceHandler - adder sent through channel")

				// SEND PUBLIC KEY AND BKs
				wallet := PublicWallet{
					PublicKey: dkgResult.Pubkey,
					BKs:       dkgResult.BKs,
				}
				walletJSON, err := json.Marshal(wallet)
				if err != nil {
					log.Println("RegisterDeviceHandler - Error marshaling wallet:", err)
					errs <- err
					return
				}
				payload := hex.EncodeToString(walletJSON)
				pubkeyMsg := Message{
					Type: PubkeyMessage,
					Msg:  payload,
				}
				err = wsjson.Write(ctx, c, pubkeyMsg)
				if err != nil {
					log.Println("error writing json through websocket:", err)
					errs <- err
					return
				}

				// start message handling of tss process
				startTss <- struct{}{} // note : in acceptDeviceHandler, need to start the adder !

				// update stage
				stage = 30

			case TssMessage:
				// verify stage : if tss message but we're at storage stage or further, discard
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("TSS message but we're at later stage; stage:", stage)
					continue
				}

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
				err = adder.HandleMessage(tssMsg)
				if err != nil {
					log.Println("could not handle tss msg:", err)
					errs <- err
					return
				}

			case EverythingStoredClientMessage:
				// note : have a timer somewhere, if after X seconds we don't have this message, then it means the process failed.
				log.Println("RegisterDeviceHandler - received EverythingStoredClientMessage")

				// let AcceptDeviceHandler know that the new device is done
				newDeviceDoneCh <- struct{}{}

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
	log.Println("RegisterDeviceHandler - trying to GetDoneChan()")
	tssDone := adder.GetDoneChan()

	// TSS sending and listening for finish signal
	go func() {
		for {
			select {
			case <-serverDone: // for the server, don't stop communcation straight when TSS is done (communication can still happen between clients)
				return
			default:
				tssMsg, err := adder.GetNextMessageToSend(newClientPeerID)
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

				log.Println("RegisterDeviceHandler - got next message to send to", newClientPeerID, ":", tssMsg)

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

	// start finishing steps after tss process => sending metadata
	<-tssDone

	log.Println("RegisterDeviceHandler - tssDone")

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
		log.Println("RegisterDeviceHandler - no error during TSS")
	}

	stage = 40 // only move to next stage after tss process is done

	// wait for existing device tss done
	<-existingDeviceTssDoneCh

	log.Println("RegisterDeviceHandler - <-existingDeviceTssDoneCh => sending MetadataMessage")

	// Send metadata to new device
	ack := Message{
		Type: MetadataMessage,
		Msg:  metadata,
	}
	err = wsjson.Write(ctx, c, ack)
	if err != nil {
		log.Println("error writing json through websocket:", err)
		errs <- err // UPDATE !!
		return
	}

	// Wait for finish signal from accepting device (= existing device)
	<-existingDeviceDoneCh

	log.Println("RegisterDeviceHandler - sending ExistingDeviceDoneMessage")

	// send existingDeviceDoneMessage to new device
	existingDeviceDoneMsg := Message{
		Type: ExistingDeviceDoneMessage,
		Msg:  "",
	}
	err = wsjson.Write(ctx, c, existingDeviceDoneMsg)
	if err != nil {
		log.Println("error writing json through websocket:", err)
		errs <- err // UPDATE !!
		return
	}

	log.Println("RegisterDeviceHandler - ExistingDeviceDoneMessage sent")

	close(serverDone)
	cancel()

	// Timer to verify that we get what we need from client ? if not, remove stuff if we need to.

	// time.Sleep(200 * time.Millisecond)

	// CLOSE WEBSOCKET
	c.Close(websocket.StatusNormalClosure, "dkg process finished successfully")
}

///////////////////////////////////////////////
///////////////////////////////////////////////
///////////////////////////////////////////////

// UPDATE DESCRIPTION
func (server *Server) AcceptDeviceHandler(w http.ResponseWriter, r *http.Request) {

	// Get userId and access token from context
	userId, ok := r.Context().Value(types.ContextKey("userId")).(string)
	if !ok {
		log.Println("Could not find userId")
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	// Get inter-handlers channels
	newClientPeerIdCh, existingClientPeerIdCh, metadataCh, adderCh, existingDeviceTssDoneCh, newDeviceDoneCh, existingDeviceDoneCh, err := server.GetInterHandlersChannels(userId)
	if err != nil {
		http.Error(w, "Channel not found", http.StatusUnauthorized)
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

	serverDone := make(chan struct{})
	startTss := make(chan struct{}) // used to avoid polling for next messages until the tss process starts
	errs := make(chan error, 2)

	var newClientPeerID string
	var existingClientPeerID string
	var metadata string

	var adder *tss.ServerAdd

	var stage uint32 = 0

	go func() {
		for {
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
				log.Println("AcceptDeviceHandler - error reading message from websocket:", err)
				log.Println("websocket.CloseStatus(err):", closeStatus)
				errs <- err
				return
			}

			// log.Println("received message in AcceptDeviceHandler:", msg)

			switch msg.Type {
			case PeerIdBroadcastMessage:
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("Device message but we're at later stage; stage:", stage)
					continue
				}

				existingClientPeerID = string(msg.Msg)

				existingClientPeerIdCh <- existingClientPeerID
				newClientPeerID = <-newClientPeerIdCh

				PeerIdBroadcastMsg := Message{
					Type: PeerIdBroadcastMessage,
					Msg:  newClientPeerID,
				}
				err = wsjson.Write(ctx, c, PeerIdBroadcastMsg)
				if err != nil {
					log.Println("RegisterDevice - deviceMsg - error writing json through websocket:", err)
					errs <- err
					return
				}

			case MetadataMessage:
				// verify stage : if device message but we're at tss stage or further, discard
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("Metadata message but we're at later stage; stage:", stage)
					continue
				}

				metadata = msg.Msg

				log.Println("AcceptDeviceHandler - metadata from message:", metadata)

				// Metadata in cache (to be used by RegisterDeviceHandler)
				// server._cache.Set(userId+"-metadata", metadata, 1*time.Minute) // useless if metadataCh ?

				metadataCh <- metadata

				adder = <-adderCh

				log.Println("adder:", adder)

				// send MetadataAckMessage (so that client can start tss process on his side)
				ack := Message{
					Type: MetadataAckMessage,
					Msg:  "",
				}
				err := wsjson.Write(ctx, c, ack)
				if err != nil {
					log.Println("error writing json through websocket:", err)
					errs <- err
					return
				}

				// start message handling of tss process & adder.process
				startTss <- struct{}{}

				// update stage
				stage = 30

			case TssMessage:
				// verify stage : if tss message but we're at storage stage or further, discard
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("TSS message but we're at later stage; stage:", stage)
					continue
				}

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
				err = adder.HandleMessage(tssMsg)
				if err != nil {
					log.Println("could not handle tss msg:", err)
					errs <- err
					return
				}

			case TssDoneMessage:
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("TSS message but we're at later stage; stage:", stage)
					continue
				}

				log.Println("AcceptDeviceHandler - received TssDoneMessage")

				existingDeviceTssDoneCh <- struct{}{}

			case ExistingDeviceDoneMessage:
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("TSS message but we're at later stage; stage:", stage)
					continue
				}

				log.Println("AcceptDeviceHandler - received ExistingDeviceDoneMessage")

				existingDeviceDoneCh <- struct{}{}

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
	log.Println("AcceptDeviceHandler - trying to GetDoneChan()")
	tssDone := adder.GetDoneChan()

	// TSS sending and listening for finish signal
	go func() {
		for {
			select {
			case <-serverDone: // for the server, don't stop communcation straight when TSS is done (communication can still happen between clients)
				return
			default:
				tssMsg, err := adder.GetNextMessageToSend(existingClientPeerID)
				if err != nil {
					if strings.Contains(err.Error(), "no message to be sent") {
						continue
					}
					log.Println("AcceptDeviceHandler - error getting next message:", err)
					errs <- err
					return
				}

				if len(tssMsg.PeerID) == 0 {
					continue
				}

				log.Println("AcceptDeviceHandler - got next message to send to", existingClientPeerID, ":", tssMsg)

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

	originalDkgResult := adder.GetOriginalWallet()

	log.Println("AcceptDeviceHandler - originalDkgResult before process:", originalDkgResult)

	// Start Adder process.
	updatedDkgResult, err := adder.Process()
	if err != nil {
		log.Println("AcceptDeviceHandler - Error while adder process:", err)
		c.Close(websocket.StatusInternalError, "adder process failed")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Println("AcceptDeviceHandler - originalDkgResult after process:", originalDkgResult)

	log.Println("AcceptDeviceHandler - process done, updatedDkgResult:", updatedDkgResult)

	mergedDkgResult, ok := tss.MergeDkgResults(originalDkgResult, updatedDkgResult)
	if !ok {
		log.Println("AcceptDeviceHandler - Error while merging dkg results:", err)
		c.Close(websocket.StatusInternalError, "adder process failed")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Update wallet in DB
	// note : verify that everything same except for BKs ??
	userAgent := r.UserAgent()
	err = server._vault.AddPeer(context.WithValue(r.Context(), types.ContextKey("metadata"), metadata), userId, newClientPeerID, userAgent, mergedDkgResult) // add metadata to context
	if err != nil {
		log.Println("Error while storing adding peer in DB:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Println("AcceptDeviceHandler - mergedDkgResult:", mergedDkgResult)

	// start finishing steps after tss process => sending metadata
	<-tssDone // kind of a duplicate from Process() in terms of timing?

	log.Println("AcceptDeviceHandler - tssDone")

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
		log.Println("AcceptDeviceHandler - no error during TSS")
	}

	stage = 40 // only move to next stage after tss process is done

	<-newDeviceDoneCh

	log.Println("AcceptDeviceHandler - sending NewDeviceDoneMessage")

	// send newDeviceDoneMessage to existing device
	newDeviceDoneMsg := Message{
		Type: NewDeviceDoneMessage,
		Msg:  "",
	}
	err = wsjson.Write(ctx, c, newDeviceDoneMsg)
	if err != nil {
		log.Println("error writing json through websocket:", err)
		errs <- err // UPDATE !!
		return
	}

	log.Println("AcceptDeviceHandler - NewDeviceDoneMessage sent")

	<-serverDone
	cancel()

	// COMPARE DKG RESULTS AFTER (same ?)

	// Timer to verify that we get what we need from client ? if not, remove stuff if we need to.

	// CLOSE WEBSOCKET
	c.Close(websocket.StatusNormalClosure, "dkg process finished successfully")
}

// Returns channels : metadata, adder, new device done, existing device done
func (server *Server) GetInterHandlersChannels(userId string) (chan string, chan string, chan string, chan *tss.ServerAdd, chan struct{}, chan struct{}, chan struct{}, error) {

	// NewClientPeerID
	newClientPeerIdChInterface, ok := server._cache.Get(userId + "-newclientpeeridch")
	if !ok {
		log.Println("could not find newClientPeerIdCh in cache")
		return nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	newClientPeerIdCh, ok := newClientPeerIdChInterface.(chan string)
	if !ok {
		log.Println("could not assert newClientPeerIdCh")
		return nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	// ExistingClientPeerID
	existingClientPeerIdChInterface, ok := server._cache.Get(userId + "-existingclientpeeridch")
	if !ok {
		log.Println("could not find existingClientPeerIdCh in cache")
		return nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	existingClientPeerIdCh, ok := existingClientPeerIdChInterface.(chan string)
	if !ok {
		log.Println("could not assert existingClientPeerIdCh")
		return nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	// Metadata
	metadataChInterface, ok := server._cache.Get(userId + "-metadatach")
	if !ok {
		log.Println("could not find metadataCh in cache")
		return nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	metadataCh, ok := metadataChInterface.(chan string)
	if !ok {
		log.Println("could not assert metadataCh")
		return nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	// Adder
	adderChInterface, ok := server._cache.Get(userId + "-adderch")
	if !ok {
		log.Println("could not find adderReady in cache")
		return nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	adderCh, ok := adderChInterface.(chan *tss.ServerAdd)
	if !ok {
		log.Println("could not assert adderReady")
		return nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	// Existing Device TSS Done
	existingDeviceTssDoneChInterface, ok := server._cache.Get(userId + "-existingdevicetssdonech")
	if !ok {
		log.Println("could not find metadataCh in cache")
		return nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	existingDeviceTssDoneCh, ok := existingDeviceTssDoneChInterface.(chan struct{})
	if !ok {
		log.Println("could not assert metadataCh")
		return nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	// New Device Done
	newDeivceDoneChInterface, ok := server._cache.Get(userId + "-newdevicedonech")
	if !ok {
		log.Println("could not find metadataCh in cache")
		return nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	newDeviceDoneCh, ok := newDeivceDoneChInterface.(chan struct{})
	if !ok {
		log.Println("could not assert metadataCh")
		return nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	// Existing Device Done
	existingDeviceDoneChInterface, ok := server._cache.Get(userId + "-existingdevicedonech")
	if !ok {
		log.Println("could not find metadataCh in cache")
		return nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	existingDeviceDoneCh, ok := existingDeviceDoneChInterface.(chan struct{})
	if !ok {
		log.Println("could not assert metadataCh")
		return nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	return newClientPeerIdCh, existingClientPeerIdCh, metadataCh, adderCh, existingDeviceTssDoneCh, newDeviceDoneCh, existingDeviceDoneCh, nil
}