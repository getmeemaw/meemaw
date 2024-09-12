package client

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
	"github.com/getmeemaw/meemaw/utils/ws"
	"github.com/google/uuid"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	_ "golang.org/x/mobile/bind"
)

// Identify gets the userId from the server (which then interacts with the auth provider) based on authData (session, access token, etc)
// Requires authData (to confirm authorization and identify user) and host
func Identify(host, authData string) (string, error) {
	return getAuthDataFromServer(host, authData, "", "/identify")
}

// Dkg performs the full dkg process on the client side
// Requires authData (to confirm authorization and identify user) and host
func Dkg(host, authData string) (*tss.DkgResult, string, error) {
	// Get temporary access token from server based on auth data
	token, err := getAccessToken(host, "", authData)
	if err != nil {
		log.Println("Dkg - error getting access token:", err)
		return nil, "", err
	}

	// Prepare DKG process
	path := "/dkg?token=" + token

	_hostHttp, err := urlToHttp(host)
	if err != nil {
		log.Println("Dkg - error getting http host:", err)
		return nil, "", err
	}

	// Check if wallet already exists
	resp, err := http.Get(_hostHttp + path)
	if err != nil {
		log.Println("Dkg - error dialing server /dkg (first call):", err)
		return nil, "", err
	}

	if resp.StatusCode == 401 {
		return nil, "", &types.ErrUnauthorized{}
	} else if resp.StatusCode == 400 {
		return nil, "", &types.ErrBadRequest{}
	} else if resp.StatusCode == 404 {
		return nil, "", &types.ErrNotFound{}
	} else if resp.StatusCode == 409 {
		log.Println("Dkg - error: existing wallet")
		return nil, "", &types.ErrConflict{}
	} else if resp.StatusCode == 426 {
		log.Println("Dkg - no existing wallet")
	} else {
		log.Println("Dkg - unknown behavior")
		return nil, "", errors.New("Dkg - unknown behavior")
	}
	defer resp.Body.Close()

	_host, err := urlToWs(host)
	if err != nil {
		log.Println("Dkg - error getting ws host:", err)
		return nil, "", err
	}

	// Init DKG
	peerID := uuid.New().String()
	if strings.HasSuffix(os.Args[0], ".test") {
		peerID = "client"
	}

	dkg, err := tss.NewClientDkg(peerID)
	if err != nil {
		log.Println("Dkg - error creating new client dkg:", err)
		return nil, "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	c, _, err := websocket.Dial(ctx, _host+path, nil)
	if err != nil {
		log.Println("Dkg - error dialing websocket:", err)
		return nil, "", err
	}
	defer c.Close(websocket.StatusInternalError, "the sky is falling")

	serverDone := make(chan struct{})
	errs := make(chan error, 2)

	var stage uint32 = 0

	var metadata string

	// send peerID
	peerIdMsg := ws.Message{
		Type: ws.PeerIdBroadcastMessage,
		Msg:  peerID,
	}
	err = wsjson.Write(ctx, c, peerIdMsg)
	if err != nil {
		log.Println("Dkg - peerIdMsg - error writing json through websocket:", err)
		errs <- err
		return nil, "", err
	}

	go func() {
		for {
			// log.Println("Dkg - wsjson.Read")
			var msg ws.Message
			err := wsjson.Read(ctx, c, &msg)
			if err != nil {
				// Check if the context was canceled
				if ctx.Err() == context.Canceled {
					// log.Println("Dkg - read operations canceled")
					return
				}

				// Check if the WebSocket was closed normally
				closeStatus := websocket.CloseStatus(err)
				if closeStatus == websocket.StatusNormalClosure || closeStatus == websocket.StatusGoingAway {
					// log.Println("Dkg - WebSocket closed normally")
					return
				}

				// Handle other errors
				log.Println("Dkg - error reading message from websocket:", err)
				log.Println("Dkg - websocket.CloseStatus(err):", closeStatus)
				errs <- err
				return
			}

			// log.Println("received message in Dkg:", msg)

			switch msg.Type {
			case ws.TssMessage:
				// verify stage : if tss message but we're at storage stage or further, discard
				if stage > msg.Type.MsgStage {
					// discard
					log.Println("Dkg - error: received TSS message but we're at later stage; stage:", stage)
					continue
				}

				// log.Println("Dkg - received tss message:", msg)

				// Decode TSS msg
				byteString, err := hex.DecodeString(msg.Msg)
				if err != nil {
					log.Println("Dkg - error decoding hex (tss message):", err)
					errs <- err
					return
				}

				tssMsg := &tss.Message{}
				err = json.Unmarshal(byteString, &tssMsg)
				if err != nil {
					log.Println("Dkg - could not unmarshal tss msg:", err)
					errs <- err
					return
				}

				// log.Println("Dkg - trying to handle tssMsg:", tssMsg)

				// Handle tss message (NOTE : will automatically, in ServerAdd.HandleMessage, redirect to other client if needs be)
				err = dkg.HandleMessage(tssMsg)
				if err != nil {
					log.Println("Dkg - error while handling tss msg:", err)
					errs <- err
					return
				}

				// log.Println("Dkg - tssMsg handled")

			case ws.MetadataMessage:
				// note : have a timer somewhere, if after X seconds we don't have this message, then it means the process failed.

				// update metadata to return it at the end
				metadata = string(msg.Msg)

				// log.Println("Dkg - received metadata (=> sending metadataAck):", metadata)

				//

				// SEND MetadataAckMessage
				ack := ws.Message{
					Type: ws.MetadataAckMessage,
					Msg:  "",
				}
				err = wsjson.Write(ctx, c, ack)
				if err != nil {
					log.Println("Dkg - MetadataAckMessage - error writing json through websocket:", err)
					errs <- err
					return
				}

				close(serverDone)

			default:
				log.Println("Dkg - Unexpected message type:", msg.Type)
				errorMsg := ws.Message{Type: ws.ErrorMessage, Msg: "error: Unexpected message type"}
				err := wsjson.Write(ctx, c, errorMsg)
				if err != nil {
					log.Println("Dkg - errorMsg - error writing json through websocket:", err)
					errs <- err
					return
				}
			}
		}
	}()

	// TSS sending and listening for finish signal
	go ws.TssSend(dkg.GetNextMessageToSend, serverDone, errs, ctx, c, "Dkg")

	// Start adder
	dkgResult, err := dkg.Process()
	if err != nil {
		log.Println("Dkg - error processing adder:", err)
		errs <- err
		return nil, "", nil
	}

	// log.Println("Dkg process finished")

	// Error management
	err = ws.ProcessErrors(errs, ctx, c, "Dkg")
	if err != nil {
		c.Close(websocket.StatusInternalError, "dkg process failed")
		return nil, "", err
	}

	stage = 40 // only move to next stage after tss process is done

	// Timer to verify that we get what we need from client ? if not, remove stuff if we need to.

	// Avoid closing connection until we received everything we needed from server
	<-serverDone

	// time.Sleep(2000 * time.Millisecond)
	cancel()

	// log.Println("Dkg serverDone")

	// CLOSE WEBSOCKET
	c.Close(websocket.StatusNormalClosure, "dkg process finished successfully")

	return dkgResult, metadata, nil
}

// Sign performs the full signing process on the client side
// Requires the message to be signed, the dkgResult (i.e. client-side of wallet), authData (to confirm authorization and identify user) and host
func Sign(host string, message []byte, dkgResultStr string, metadata string, authData string) (*tss.Signature, error) {

	// Get temporary access token from server based on auth data
	token, err := getAccessToken(host, metadata, authData)
	if err != nil {
		log.Println("Sign - error getting access token:", err)
		return nil, &types.ErrUnauthorized{}
	}

	var dkgResult tss.DkgResult
	err = json.Unmarshal([]byte(dkgResultStr), &dkgResult)
	if err != nil {
		log.Println("Sign - error unmarshaling signingParameters:", err)
		return nil, &types.ErrBadRequest{}
	}

	// Prepare signing process

	pubkeyStr := dkgResult.Pubkey
	BKs := dkgResult.BKs
	share := dkgResult.Share
	clientPeerID := dkgResult.PeerID

	path := "/sign?msg=" + hex.EncodeToString(message) + "&token=" + token + "&peer=" + clientPeerID

	_host, err := urlToWs(host)
	if err != nil {
		log.Println("Sign - error getting ws host:", err)
		return nil, &types.ErrBadRequest{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	c, resp, err := websocket.Dial(ctx, _host+path, nil)
	if err != nil {
		if resp == nil {
			log.Println("Sign - error dialing websocket:", err)
			return nil, err
		}

		if resp.StatusCode == 401 {
			return nil, &types.ErrUnauthorized{}
		} else if resp.StatusCode == 400 {
			return nil, &types.ErrBadRequest{}
		} else if resp.StatusCode == 404 {
			return nil, &types.ErrNotFound{}
		} else if resp.StatusCode == 409 {
			return nil, &types.ErrConflict{}
		} else {
			log.Println("Sign - error dialing websocket:", err)
			return nil, err
		}
	}
	defer c.Close(websocket.StatusInternalError, "the sky is falling")

	signer, err := tss.NewClientSigner(clientPeerID, pubkeyStr, share, BKs, message)
	if err != nil {
		log.Println("Sign - error when getting new client signer:", err)
		return nil, &types.ErrBadRequest{}
	}

	errs := make(chan error, 2)

	go tss.Send(signer, ctx, errs, c)
	go tss.Receive(signer, ctx, errs, c)

	// Start signing process
	signature, err := signer.Process()
	if err != nil {
		log.Println("Sign - error processing signing:", err)
		return nil, &types.ErrTssProcessFailed{}
	}

	// Error management
	processErr := <-errs // wait for websocket closure from server
	if ctx.Err() == context.Canceled {
		log.Println("Sign - websocket closed by context cancellation:", processErr)
		return nil, processErr
	} else if websocket.CloseStatus(processErr) == websocket.StatusNormalClosure {
		// log.Println("Sign - websocket closed normally")
		cancel()
	} else {
		log.Println("Sign - error during websocket connection:", processErr) // even if badly closed, we continue as we have the signature
		cancel()
	}

	// time.Sleep(time.Second)

	// c.Close(websocket.StatusNormalClosure, "")

	return signature, nil
}

// Export exports the private key from the server and client shares
// Requires the dkgResult (i.e. client-side of wallet), authData (to confirm authorization and identify user) and host
func Export(host string, dkgResultStr string, metadata string, authData string) (string, error) {

	// Get temporary access token from server based on auth data
	token, err := getAccessToken(host, metadata, authData)
	if err != nil {
		log.Println("Export - error getting access token:", err)
		return "", &types.ErrUnauthorized{}
	}

	// Prepare query
	path := "/export?token=" + token

	_host, err := urlToHttp(host)
	if err != nil {
		log.Println("Export - error getting ws host:", err)
		return "", &types.ErrBadRequest{}
	}

	// Get client share
	var dkgResult tss.DkgResult
	err = json.Unmarshal([]byte(dkgResultStr), &dkgResult)
	if err != nil {
		log.Println("Export - error unmarshaling signingParameters:", err)
		return "", &types.ErrBadRequest{}
	}

	share := dkgResult.Share
	clientPeerID := dkgResult.PeerID

	// Create the form data
	formData := url.Values{
		"share":        {share},
		"clientPeerID": {clientPeerID},
	}

	// Send POST request
	resp, err := http.PostForm(_host+path, formData)
	if err != nil {
		fmt.Println("Export - error sending request:", err)
		return "", &types.ErrBadRequest{}
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Export - error reading response body:", err)
		return "", &types.ErrBadRequest{}
	}

	return string(body), nil
}

//////////////
//// UTIL ////
//////////////

func getAccessToken(host, metadata, authData string) (string, error) {
	return getAuthDataFromServer(host, authData, metadata, "/authorize")
}

func getAuthDataFromServer(host, authData, metadata, endpoint string) (string, error) {
	_host, err := urlToHttp(host)
	if err != nil {
		return "", err
	}

	// Request access token
	req, err := http.NewRequest("GET", _host+endpoint, nil)
	if err != nil {
		log.Println("getAuthDataFromServer - error while creating new request:", err)
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+authData)
	req.Header.Set("M-METADATA", metadata)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("getAuthDataFromServer - error while doing request to", endpoint, ":", err)
		return "", err
	}

	if resp.StatusCode != 200 {
		log.Println("getAuthDataFromServer - error while", endpoint, ", status not 200")
		log.Printf("getAuthDataFromServer - resp:%+v\n", resp)
		return "", fmt.Errorf(endpoint, " status not 200")
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("getAuthDataFromServer - error while reading response body of", endpoint, ":", err)
		return "", err
	}
	retValue := string(body)

	return retValue, nil
}

func urlToHttp(_url string) (string, error) {
	parsedURL, err := url.Parse(_url)
	if err != nil {
		return "", err
	}

	switch parsedURL.Scheme {
	case "https", "wss":
		parsedURL.Scheme = "https"
	case "http", "ws":
		parsedURL.Scheme = "http"
	}

	return strings.TrimSuffix(parsedURL.String(), "/"), nil
}

func urlToWs(_url string) (string, error) {
	parsedURL, err := url.Parse(_url)
	if err != nil {
		return "", err
	}

	switch parsedURL.Scheme {
	case "https", "wss":
		parsedURL.Scheme = "wss"
	case "http", "ws":
		parsedURL.Scheme = "ws"
	}

	return strings.TrimSuffix(parsedURL.String(), "/"), nil
}
