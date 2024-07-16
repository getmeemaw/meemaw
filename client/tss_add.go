package client

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/getmeemaw/meemaw/server"
	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
	"github.com/google/uuid"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// UPDATE DESCRIPTION
func RegisterDevice(host, authData, device string) (*tss.DkgResult, string, error) {
	// Get temporary access token from server based on auth data
	token, err := getAccessToken(host, "", authData)
	if err != nil {
		log.Println("error getting access token:", err)
		return nil, "", err
	}

	// Prepare DKG process
	path := "/register?token=" + token

	_host, err := urlToWs(host)
	if err != nil {
		log.Println("error getting ws host:", err)
		return nil, "", err
	}

	var adder *tss.ClientAdd

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	c, resp, err := websocket.Dial(ctx, _host+path, nil)
	if err != nil {
		if resp.StatusCode == 401 {
			return nil, "", &types.ErrUnauthorized{}
		} else if resp.StatusCode == 400 {
			return nil, "", &types.ErrBadRequest{}
		} else if resp.StatusCode == 404 {
			return nil, "", &types.ErrNotFound{}
		} else if resp.StatusCode == 409 {
			return nil, "", &types.ErrConflict{}
		} else {
			log.Println("error dialing websocket:", err)
			return nil, "", err
		}
	}
	defer c.Close(websocket.StatusInternalError, "the sky is falling")

	serverDone := make(chan struct{})
	startTss := make(chan struct{}) // used to avoid polling for next messages until the tss process starts
	errs := make(chan error, 2)

	var stage uint32 = 0

	var metadata string

	peerID := uuid.New().String()
	var acceptingDevicePeerID string

	// send peerID
	peerIdMsg := server.Message{
		Type: server.PeerIdBroadcastMessage,
		Msg:  peerID,
	}
	err = wsjson.Write(ctx, c, peerIdMsg)
	if err != nil {
		log.Println("RegisterDevice - peerIdMsg - error writing json through websocket:", err)
		errs <- err
		return nil, "", err
	}

	go func() {
		for {
			log.Println("RegisterDevice - wsjson.Read")
			var msg server.Message
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
				log.Println("RegisterDevice - error reading message from websocket:", err)
				log.Println("websocket.CloseStatus(err):", closeStatus)
				errs <- err
				return
			}

			log.Println("RegisterDevice - received message:", msg)

			switch msg.Type {
			case server.PeerIdBroadcastMessage:
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("Device message but we're at later stage; stage:", stage)
					continue
				}

				acceptingDevicePeerID = string(msg.Msg)

				// send DeviceMessage
				deviceMsg := server.Message{
					Type: server.DeviceMessage,
					Msg:  device,
				}
				err = wsjson.Write(ctx, c, deviceMsg)
				if err != nil {
					log.Println("RegisterDevice - deviceMsg - error writing json through websocket:", err)
					errs <- err
					return
				}

			case server.PubkeyMessage:
				// Recover pubkey & BKs from message

				log.Println("RegisterDevice - received pubkey message.")

				data, err := hex.DecodeString(msg.Msg)
				if err != nil {
					log.Println("error decoding publicWallet:", err)
					errs <- err
					return
				}

				var publicWallet server.PublicWallet
				err = json.Unmarshal(data, &publicWallet)
				if err != nil {
					log.Println("error unmarshaling publicWallet:", err)
					errs <- err
					return
				}

				log.Println("RegisterDevice - received pubkey public wallet:", publicWallet)
				log.Println("RegisterDevice - creating adder")

				// Create adder
				adder, err = tss.NewClientAdd(peerID, acceptingDevicePeerID, publicWallet.PublicKey, publicWallet.BKs)
				if err != nil {
					log.Println("RegisterDevice - error creating newClientAdd():", err)
					errs <- err
					return
				}

				log.Println("RegisterDevice - startTss<-")

				// start message handling of tss process & adder.process
				startTss <- struct{}{}

				// // SEND PUBLIC ACK
				// ack := server.Message{
				// 	Type: server.PubkeyAckMessage,
				// 	Msg:  nil,
				// }
				// err = wsjson.Write(ctx, c, ack)
				// if err != nil {
				// 	log.Println("RegisterDevice - PubkeyAckMessage - error writing json through websocket:", err)
				// 	errs <- err
				// 	return
				// }
			case server.TssMessage:
				// verify stage : if tss message but we're at storage stage or further, discard
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("TSS message but we're at later stage; stage:", stage)
					continue
				}

				log.Println("RegisterDevice - received tss message")

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

				log.Println("RegisterDevice - trying to handle tssMsg:", tssMsg)

				// Handle tss message (NOTE : will automatically, in ServerAdd.HandleMessage, redirect to other client if needs be)
				err = adder.HandleMessage(tssMsg)
				if err != nil {
					log.Println("could not handle tss msg:", err)
					errs <- err
					return
				}

			case server.MetadataMessage:
				// note : have a timer somewhere, if after X seconds we don't have this message, then it means the process failed.

				// update metadata to return it at the end
				metadata = string(msg.Msg)

				log.Println("RegisterDevice - received metadata (=> sending metadataAck):", metadata)

				// SEND EverythingStoredClientMessage
				ack := server.Message{
					Type: server.EverythingStoredClientMessage,
					Msg:  "",
				}
				err = wsjson.Write(ctx, c, ack)
				if err != nil {
					log.Println("RegisterDevice - EverythingStoredClientMessage - error writing json through websocket:", err)
					errs <- err
					return
				}

			case server.ExistingDeviceDoneMessage:
				// verify stage : if tss message but we're at storage stage or further, discard
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("TSS message but we're at later stage; stage:", stage)
					continue
				}

				log.Println("RegisterDevice - received ExistingDeviceDoneMessage")

				// Stop process
				close(serverDone)

			default:
				log.Println("Unexpected message type:", msg.Type)
				errorMsg := server.Message{Type: server.ErrorMessage, Msg: "error: Unexpected message type"}
				err := wsjson.Write(ctx, c, errorMsg)
				if err != nil {
					log.Println("RegisterDevice - errorMsg - error writing json through websocket:", err)
					errs <- err
					return
				}
			}
		}
	}()

	// Wait for tss start
	<-startTss

	// Get channel from adder /!\ needs to be initialised first => this line needs to be after <-startTss
	log.Println("RegisterDevice - trying to GetDoneChan()")
	tssDone := adder.GetDoneChan()

	// TSS sending and listening for finish signal
	go func() {
		var counter int

		for {
			select {
			case <-serverDone:
				return
			default:
				log.Println("RegisterDevice - adder.GetNextMessageToSendAll()")
				// Get next message to send (either server or other devices)
				tssMsg, err := adder.GetNextMessageToSendAll()
				if err != nil {
					if strings.Contains(err.Error(), "no message to be sent") {
						time.Sleep(10 * time.Millisecond)
						continue
					}
					log.Println("error getting next message:", err)
					errs <- err
					return
				}

				if len(tssMsg.PeerID) == 0 {
					if counter > 100 {
						log.Println("no more messages it seems like")
						return
					}
					counter++
					time.Sleep(10 * time.Millisecond)
					continue
				}

				log.Println("got next message to send:", tssMsg)

				// format message for communication
				jsonEncodedMsg, err := json.Marshal(tssMsg)
				if err != nil {
					log.Println("could not marshal tss msg:", err)
					errs <- err
					return
				}

				payload := hex.EncodeToString(jsonEncodedMsg)

				msg := server.Message{
					Type: server.TssMessage,
					Msg:  payload,
				}

				// log.Println("trying send, next encoded message to send:", encodedMsg)

				if tssMsg.Message != nil {
					// log.Println("trying to send message:", encodedMsg)
					err := wsjson.Write(ctx, c, msg)
					if err != nil {
						log.Println("RegisterDevice - tss message - error writing json through websocket:", err)
						errs <- err
						return
					}
				}

				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	log.Println("RegisterDevice - start process")

	// Start adder
	dkgResult, err := adder.Process()
	if err != nil {
		log.Println("error processing adder:", err)
		errs <- err
		return nil, "", nil
	}

	log.Println("RegisterDevice process finished")

	// start finishing steps after tss process => sending metadata
	<-tssDone

	log.Println("RegisterDevice tssDone")

	// Error management
	select {
	case processErr := <-errs:
		if websocket.CloseStatus(processErr) == websocket.StatusNormalClosure {
			log.Println("websocket closed normally") // Should not really happen on server side (server is closing)
		} else if ctx.Err() == context.Canceled {
			log.Println("websocket closed by context cancellation:", processErr)
			c.Close(websocket.StatusInternalError, "dkg process failed")
			// http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return nil, "", nil
		} else {
			log.Println("error during websocket connection:", processErr)
			c.Close(websocket.StatusInternalError, "dkg process failed")
			// http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return nil, "", nil
		}
	default:
		log.Println("RegisterDevice - no error during TSS")
	}

	stage = 40 // only move to next stage after tss process is done

	// Timer to verify that we get what we need from client ? if not, remove stuff if we need to.

	// Avoid closing connection until we received everything we needed from client
	<-serverDone

	// time.Sleep(2000 * time.Millisecond)
	cancel()

	log.Println("RegisterDevice serverDone")

	// CLOSE WEBSOCKET
	c.Close(websocket.StatusNormalClosure, "dkg process finished successfully")

	return dkgResult, metadata, nil // UPDATE
}

///////////////////////////////////////////////
///////////////////////////////////////////////
///////////////////////////////////////////////

// UPDATE DESCRIPTION
func AcceptDevice(host string, dkgResultStr string, metadata string, authData string) error {

	// send metadata, use dkgResults to create adder, etc

	// messages received:
	// - metadataAck
	// - tss
	// - some form of finishing message?

	//////////

	// Get temporary access token from server based on auth data
	token, err := getAccessToken(host, metadata, authData)
	if err != nil {
		log.Println("error getting access token:", err)
		return err
	}

	// Unmarshal dkg results
	var dkgResult tss.DkgResult
	err = json.Unmarshal([]byte(dkgResultStr), &dkgResult)
	if err != nil {
		log.Println("error unmarshaling dkgResult:", err)
		return err
	}

	// Prepare DKG process
	path := "/accept?token=" + token

	_host, err := urlToWs(host)
	if err != nil {
		log.Println("error getting ws host:", err)
		return err
	}

	var adder *tss.ExistingClientAdd

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	c, resp, err := websocket.Dial(ctx, _host+path, nil)
	if err != nil {
		log.Println("error dialing websocket:", err)
		if resp.StatusCode == 401 {
			return &types.ErrUnauthorized{}
		} else if resp.StatusCode == 400 {
			return &types.ErrBadRequest{}
		} else if resp.StatusCode == 404 {
			return &types.ErrNotFound{}
		} else if resp.StatusCode == 409 {
			return &types.ErrConflict{}
		} else {
			return err
		}
	}
	defer c.Close(websocket.StatusInternalError, "the sky is falling")

	serverDone := make(chan struct{})
	startTss := make(chan struct{}) // used to avoid polling for next messages until the tss process starts
	errs := make(chan error, 2)

	var stage uint32 = 0

	log.Println("sending metadata from acceptDevice:", metadata)

	peerID := dkgResult.PeerID
	var newClientPeerID string

	// send peerID
	peerIdMsg := server.Message{
		Type: server.PeerIdBroadcastMessage,
		Msg:  peerID,
	}
	err = wsjson.Write(ctx, c, peerIdMsg)
	if err != nil {
		log.Println("AcceptDevice - peerIdMsg - error writing json through websocket:", err)
		return err
	}

	log.Println("metadata sent from acceptDevice")

	go func() {
		for {
			log.Println("AcceptDevice - wsjson.Read")
			var msg server.Message
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
				log.Println("AcceptDevice - error reading message from websocket:", err)
				log.Println("websocket.CloseStatus(err):", closeStatus)
				errs <- err
				return
			}

			log.Println("AcceptDevice - received message:", msg)

			switch msg.Type {
			case server.PeerIdBroadcastMessage:
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("Device message but we're at later stage; stage:", stage)
					continue
				}

				newClientPeerID = string(msg.Msg)

				// send metadata
				metadataMsg := server.Message{
					Type: server.MetadataMessage,
					Msg:  metadata,
				}
				err = wsjson.Write(ctx, c, metadataMsg)
				if err != nil {
					log.Println("AcceptDevice - MetadataMessage - error writing json through websocket:", err)
					errs <- err
					return
				}

			case server.MetadataAckMessage:
				// Create adder

				log.Println("AcceptDevice - creating adder")

				adder, err = tss.NewExistingClientAdd(newClientPeerID, peerID, dkgResult.Pubkey, dkgResult.Share, dkgResult.BKs)
				if err != nil {
					log.Println("AcceptDevice - error creating newClientAdd():", err)
					errs <- err
					return
				}

				log.Println("AcceptDevice - startTss<-")

				// start message handling of tss process & adder.process
				startTss <- struct{}{}
			case server.TssMessage:
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

				log.Println("AcceptDevice - trying to handle tssMsg:", tssMsg)

				// Handle tss message
				err = adder.HandleMessage(tssMsg)
				if err != nil {
					log.Println("could not handle tss msg:", err)
					errs <- err
					return
				}

			case server.NewDeviceDoneMessage:
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("TSS message but we're at later stage; stage:", stage)
					continue
				}

				log.Println("AcceptDevice - received NewDeviceDoneMessage")

				existingDeviceDoneMsg := server.Message{
					Type: server.ExistingDeviceDoneMessage,
					Msg:  "",
				}
				err = wsjson.Write(ctx, c, existingDeviceDoneMsg)
				if err != nil {
					log.Println("error writing json through websocket:", err)
					errs <- err // UPDATE !!
					return
				}

				log.Println("AcceptDevice - closing serverDone")

				close(serverDone)

			default:
				log.Println("Unexpected message type:", msg.Type)
				errorMsg := server.Message{Type: server.ErrorMessage, Msg: "error: Unexpected message type"}
				err := wsjson.Write(ctx, c, errorMsg)
				if err != nil {
					log.Println("AcceptDevice - errorMessage - error writing json through websocket:", err)
					errs <- err
					return
				}
			}
		}
	}()

	// Wait for tss start
	<-startTss

	// Get channel from adder /!\ needs to be initialised first => this line needs to be after <-startTss
	log.Println("AcceptDevice - trying to GetDoneChan()")
	tssDone := adder.GetDoneChan()

	// TSS sending and listening for finish signal
	go func() {
		var counter int

		for {
			select {
			case <-serverDone:
				return
			default:
				// Get next message to send (either server or other devices)
				tssMsg, err := adder.GetNextMessageToSendAll()
				if err != nil {
					if strings.Contains(err.Error(), "no message to be sent") {
						time.Sleep(10 * time.Millisecond)
						continue
					}
					log.Println("error getting next message:", err)
					errs <- err
					return
				}

				if len(tssMsg.PeerID) == 0 {
					if counter > 100 {
						log.Println("no more messages it seems like")
						return
					}
					counter++
					time.Sleep(10 * time.Millisecond)
					continue
				}

				log.Println("got next message to send:", tssMsg)

				// format message for communication
				jsonEncodedMsg, err := json.Marshal(tssMsg)
				if err != nil {
					log.Println("could not marshal tss msg:", err)
					errs <- err
					return
				}

				payload := hex.EncodeToString(jsonEncodedMsg)

				msg := server.Message{
					Type: server.TssMessage,
					Msg:  payload,
				}

				// log.Println("trying send, next encoded message to send:", encodedMsg)

				if tssMsg.Message != nil {
					// log.Println("trying to send message:", encodedMsg)
					err := wsjson.Write(ctx, c, msg)
					if err != nil {
						log.Println("AcceptDevice - tss message - error writing json through websocket:", err)
						errs <- err
						return
					}
				}

				time.Sleep(10 * time.Millisecond) // UPDATE : remove polling, use channels to trigger send when next TSS message ready
			}
		}
	}()

	log.Println("AcceptDevice - start process")

	// Start adder
	newDkgResult, err := adder.Process() // UPDATE RETURN
	if err != nil {
		log.Println("error processing adder:", err)
		errs <- err
		return nil
	}

	log.Println("AcceptDevice - process done")

	// start finishing steps after tss process => sending metadata
	<-tssDone

	log.Println("AcceptDevice - tssDone")

	// Error management
	select {
	case processErr := <-errs:
		if websocket.CloseStatus(processErr) == websocket.StatusNormalClosure {
			log.Println("websocket closed normally") // Should not really happen on server side (server is closing)
		} else if ctx.Err() == context.Canceled {
			log.Println("websocket closed by context cancellation:", processErr)
			c.Close(websocket.StatusInternalError, "dkg process failed")
			// http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return nil
		} else {
			log.Println("error during websocket connection:", processErr)
			c.Close(websocket.StatusInternalError, "dkg process failed")
			// http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return nil
		}
	default:
		log.Println("AcceptDevice - no error during TSS")
	}

	stage = 40 // only move to next stage after tss process is done

	log.Println("AcceptDevice - sending TssDoneMessage")

	// Send tss done message
	existingDeviceDoneMsg := server.Message{
		Type: server.TssDoneMessage,
		Msg:  "",
	}
	err = wsjson.Write(ctx, c, existingDeviceDoneMsg)
	if err != nil {
		log.Println("error writing json through websocket:", err)
		return err
	}

	<-serverDone

	cancel()

	log.Println("AcceptDevice serverDone")

	log.Println("")

	log.Println("AcceptDevice - old dkgResult:", dkgResult)
	log.Println("AcceptDevice - new dkgResult:", newDkgResult)

	// Timer to verify that we get what we need from client ? if not, remove stuff if we need to.

	// CLOSE WEBSOCKET
	c.Close(websocket.StatusNormalClosure, "dkg process finished successfully")

	return nil // UPDATE
}
