package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
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

type PublicWallet struct {
	PublicKey tss.PubkeyStr
	BKs       map[string]tss.BK
}

// RegisterDeviceHandler is called by a new device wanting to "join" the wallet by creating a new share for itself, in collaboration with existing peers
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
		log.Println("RegisterDeviceHandler - Error accepting websocket:", err)
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
	useragentCh := make(chan string, 1)
	metadataCh := make(chan string, 1)
	adderCh := make(chan *tss.ServerAdd, 1)
	existingDeviceTssDoneCh := make(chan struct{}, 1)
	newDeviceDoneCh := make(chan struct{}, 1)
	existingDeviceDoneCh := make(chan struct{}, 1)

	server._cache.Set(userId+"-newclientpeeridch", newClientPeerIdCh, 10*time.Minute)
	server._cache.Set(userId+"-existingclientpeeridch", existingClientPeerIdCh, 10*time.Minute)
	server._cache.Set(userId+"-useragentch", useragentCh, 10*time.Minute)
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
				log.Println("RegisterDeviceHandler - error reading message from websocket:", err)
				log.Println("websocket.CloseStatus(err):", closeStatus)
				errs <- err
				return
			}

			// log.Println("received message in RegisterDeviceHandler:", msg)

			switch msg.Type {
			case ws.PeerIdBroadcastMessage:
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("Device message but we're at later stage; stage:", stage)
					continue
				}

				newClientPeerID = string(msg.Msg)

				newClientPeerIdCh <- newClientPeerID
				existingClientPeerID = <-existingClientPeerIdCh

				useragentCh <- r.UserAgent()

				PeerIdBroadcastMsg := ws.Message{
					Type: ws.PeerIdBroadcastMessage,
					Msg:  existingClientPeerID,
				}
				err = wsjson.Write(ctx, c, PeerIdBroadcastMsg)
				if err != nil {
					log.Println("RegisterDevice - deviceMsg - error writing json through websocket:", err)
					errs <- err
					return
				}

			case ws.DeviceMessage:
				// verify stage : if device message but we're at tss stage or further, discard
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("Device message but we're at later stage; stage:", stage)
					continue
				}

				// Read device from message
				device := string(msg.Msg)

				log.Println("RegisterDeviceHandler - got device from client:", device)

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
				pubkeyMsg := ws.Message{
					Type: ws.PubkeyMessage,
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

			case ws.TssMessage:
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

			case ws.EverythingStoredClientMessage:
				// note : have a timer somewhere, if after X seconds we don't have this message, then it means the process failed.
				log.Println("RegisterDeviceHandler - received EverythingStoredClientMessage")

				// let AcceptDeviceHandler know that the new device is done
				newDeviceDoneCh <- struct{}{}

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
	log.Println("RegisterDeviceHandler - trying to GetDoneChan()")
	tssDone := adder.GetDoneChan()

	// TSS sending and listening for finish signal
	go ws.TssSend(func() (tss.Message, error) { return adder.GetNextMessageToSend(newClientPeerID) }, serverDone, errs, ctx, c, "RegisterDeviceHandler")

	// start finishing steps after tss process => sending metadata
	<-tssDone

	log.Println("RegisterDeviceHandler - tssDone")

	// Error management
	err = ws.ProcessErrors(errs, ctx, c, "RegisterDeviceHandler")
	if err != nil {
		c.Close(websocket.StatusInternalError, "RegisterDeviceHandler process failed")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	stage = 40 // only move to next stage after tss process is done

	// wait for existing device tss done
	<-existingDeviceTssDoneCh

	log.Println("RegisterDeviceHandler - <-existingDeviceTssDoneCh => sending MetadataMessage")

	// Send metadata to new device
	ack := ws.Message{
		Type: ws.MetadataMessage,
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
	existingDeviceDoneMsg := ws.Message{
		Type: ws.ExistingDeviceDoneMessage,
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

	// CLOSE WEBSOCKET
	c.Close(websocket.StatusNormalClosure, "dkg process finished successfully")
}

///////////////////////////////////////////////
///////////////////////////////////////////////
///////////////////////////////////////////////

// AcceptDeviceHandler is called by a device already part of the TSS wallet. In collaboration with the server and the new device, a new share is created for the new device
func (server *Server) AcceptDeviceHandler(w http.ResponseWriter, r *http.Request) {

	// Get userId and access token from context
	userId, ok := r.Context().Value(types.ContextKey("userId")).(string)
	if !ok {
		log.Println("Could not find userId")
		http.Error(w, "Authorization info not found", http.StatusUnauthorized)
		return
	}

	// Get inter-handlers channels
	newClientPeerIdCh, existingClientPeerIdCh, useragentCh, metadataCh, adderCh, existingDeviceTssDoneCh, newDeviceDoneCh, existingDeviceDoneCh, err := server.GetInterHandlersChannels(userId)
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
		log.Println("AcceptDeviceHandler - Error accepting websocket:", err)
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
				log.Println("AcceptDeviceHandler - error reading message from websocket:", err)
				log.Println("websocket.CloseStatus(err):", closeStatus)
				errs <- err
				return
			}

			// log.Println("received message in AcceptDeviceHandler:", msg)

			switch msg.Type {
			case ws.PeerIdBroadcastMessage:
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("Device message but we're at later stage; stage:", stage)
					continue
				}

				existingClientPeerID = string(msg.Msg)

				existingClientPeerIdCh <- existingClientPeerID
				newClientPeerID = <-newClientPeerIdCh

				PeerIdBroadcastMsg := ws.Message{
					Type: ws.PeerIdBroadcastMessage,
					Msg:  newClientPeerID,
				}
				err = wsjson.Write(ctx, c, PeerIdBroadcastMsg)
				if err != nil {
					log.Println("RegisterDevice - deviceMsg - error writing json through websocket:", err)
					errs <- err
					return
				}

			case ws.MetadataMessage:
				// verify stage : if device message but we're at tss stage or further, discard
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("Metadata message but we're at later stage; stage:", stage)
					continue
				}

				metadata = msg.Msg

				log.Println("AcceptDeviceHandler - metadata from message:", metadata)

				metadataCh <- metadata
				adder = <-adderCh

				log.Println("adder:", adder)

				// send MetadataAckMessage (so that client can start tss process on his side)
				ack := ws.Message{
					Type: ws.MetadataAckMessage,
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

			case ws.TssMessage:
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

			case ws.TssDoneMessage:
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("TSS message but we're at later stage; stage:", stage)
					continue
				}

				log.Println("AcceptDeviceHandler - received TssDoneMessage")

				existingDeviceTssDoneCh <- struct{}{}

			case ws.ExistingDeviceDoneMessage:
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
	log.Println("AcceptDeviceHandler - trying to GetDoneChan()")
	tssDone := adder.GetDoneChan()

	// TSS sending and listening for finish signal
	go ws.TssSend(func() (tss.Message, error) { return adder.GetNextMessageToSend(existingClientPeerID) }, serverDone, errs, ctx, c, "AcceptDeviceHandler")

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
	userAgent := <-useragentCh
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
	err = ws.ProcessErrors(errs, ctx, c, "AcceptDeviceHandler")
	if err != nil {
		c.Close(websocket.StatusInternalError, "AcceptDeviceHandler process failed")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	stage = 40 // only move to next stage after tss process is done

	<-newDeviceDoneCh

	log.Println("AcceptDeviceHandler - sending NewDeviceDoneMessage")

	// send newDeviceDoneMessage to existing device
	newDeviceDoneMsg := ws.Message{
		Type: ws.NewDeviceDoneMessage,
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

	// CLOSE WEBSOCKET
	c.Close(websocket.StatusNormalClosure, "dkg process finished successfully")
}

// Returns channels : metadata, adder, new device done, existing device done
func (server *Server) GetInterHandlersChannels(userId string) (chan string, chan string, chan string, chan string, chan *tss.ServerAdd, chan struct{}, chan struct{}, chan struct{}, error) {

	// NewClientPeerID
	newClientPeerIdChInterface, ok := server._cache.Get(userId + "-newclientpeeridch")
	if !ok {
		log.Println("could not find newClientPeerIdCh in cache")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	newClientPeerIdCh, ok := newClientPeerIdChInterface.(chan string)
	if !ok {
		log.Println("could not assert newClientPeerIdCh")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	// ExistingClientPeerID
	existingClientPeerIdChInterface, ok := server._cache.Get(userId + "-existingclientpeeridch")
	if !ok {
		log.Println("could not find existingClientPeerIdCh in cache")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	existingClientPeerIdCh, ok := existingClientPeerIdChInterface.(chan string)
	if !ok {
		log.Println("could not assert existingClientPeerIdCh")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	// User Agent
	useragentChInterface, ok := server._cache.Get(userId + "-useragentch")
	if !ok {
		log.Println("could not find useragentCh in cache")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	useragentCh, ok := useragentChInterface.(chan string)
	if !ok {
		log.Println("could not assert useragentCh")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	// Metadata
	metadataChInterface, ok := server._cache.Get(userId + "-metadatach")
	if !ok {
		log.Println("could not find metadataCh in cache")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	metadataCh, ok := metadataChInterface.(chan string)
	if !ok {
		log.Println("could not assert metadataCh")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	// Adder
	adderChInterface, ok := server._cache.Get(userId + "-adderch")
	if !ok {
		log.Println("could not find adderReady in cache")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	adderCh, ok := adderChInterface.(chan *tss.ServerAdd)
	if !ok {
		log.Println("could not assert adderReady")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	// Existing Device TSS Done
	existingDeviceTssDoneChInterface, ok := server._cache.Get(userId + "-existingdevicetssdonech")
	if !ok {
		log.Println("could not find metadataCh in cache")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	existingDeviceTssDoneCh, ok := existingDeviceTssDoneChInterface.(chan struct{})
	if !ok {
		log.Println("could not assert metadataCh")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	// New Device Done
	newDeivceDoneChInterface, ok := server._cache.Get(userId + "-newdevicedonech")
	if !ok {
		log.Println("could not find metadataCh in cache")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	newDeviceDoneCh, ok := newDeivceDoneChInterface.(chan struct{})
	if !ok {
		log.Println("could not assert metadataCh")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	// Existing Device Done
	existingDeviceDoneChInterface, ok := server._cache.Get(userId + "-existingdevicedonech")
	if !ok {
		log.Println("could not find metadataCh in cache")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("channel not found")
	}

	existingDeviceDoneCh, ok := existingDeviceDoneChInterface.(chan struct{})
	if !ok {
		log.Println("could not assert metadataCh")
		return nil, nil, nil, nil, nil, nil, nil, nil, errors.New("wrong channel")
	}

	return newClientPeerIdCh, existingClientPeerIdCh, useragentCh, metadataCh, adderCh, existingDeviceTssDoneCh, newDeviceDoneCh, existingDeviceDoneCh, nil
}
