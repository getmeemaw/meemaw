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
	"strings"
	"time"

	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
	"nhooyr.io/websocket"

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
		log.Println("error getting access token:", err)
		return nil, "", err
	}

	// Prepare DKG process
	path := "/dkg?token=" + token

	_host, err := urlToWs(host)
	if err != nil {
		log.Println("error getting ws host:", err)
		return nil, "", err
	}

	dkg, err := tss.NewClientDkg()
	if err != nil {
		return nil, "", err
	}

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

	errs := make(chan error, 2)

	go tss.Send(dkg, ctx, errs, c)
	go tss.Receive(dkg, ctx, errs, c)

	// Start DKG process.
	res, err := dkg.Process()
	if err != nil {
		return nil, "", err
	}

	// Error management
	processErr := <-errs // wait for websocket closure from server
	if ctx.Err() == context.Canceled {
		log.Println("websocket closed by context cancellation:", processErr)
		return nil, "", processErr
	} else if websocket.CloseStatus(processErr) == websocket.StatusNormalClosure {
		log.Println("websocket closed normally")
		cancel()
	} else {
		log.Println("error during websocket connection:", processErr) // it's ok, as the proper completion will be verified with /dkgtwo
		cancel()
	}

	//////////
	/// METADATA

	_host, err = urlToHttp(host)
	if err != nil {
		log.Println("error getting http host:", err)
		return nil, "", err
	}

	// Get metadata from /dkgtwo
	path = "/dkgtwo?token=" + token
	resp, err = http.Get(_host + path)
	if err != nil {
		log.Println("failed to call dkgtwo:", err)
		return nil, "", err
	}
	defer resp.Body.Close() // Ensure the response body is closed

	if resp.StatusCode != 200 {
		log.Println("error during dkgtwo:", resp.Status)
		return nil, "", errors.New("error during dkgtwo")
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response body: %v", err)
	}

	metadata := string(body)

	// time.Sleep(120 * time.Hour) // debug

	// c.Close(websocket.StatusNormalClosure, "")

	return res, metadata, nil
}

// Sign performs the full signing process on the client side
// Requires the message to be signed, the dkgResult (i.e. client-side of wallet), authData (to confirm authorization and identify user) and host
func Sign(host string, message []byte, dkgResultStr string, metadata string, authData string) (*tss.Signature, error) {

	// Get temporary access token from server based on auth data
	token, err := getAccessToken(host, metadata, authData)
	if err != nil {
		log.Println("error getting access token:", err)
		return nil, &types.ErrUnauthorized{}
	}

	// Prepare signing process
	path := "/sign?msg=" + hex.EncodeToString(message) + "&token=" + token

	_host, err := urlToWs(host)
	if err != nil {
		log.Println("error getting ws host:", err)
		return nil, &types.ErrBadRequest{}
	}

	var dkgResult tss.DkgResult
	err = json.Unmarshal([]byte(dkgResultStr), &dkgResult)
	if err != nil {
		log.Println("error unmarshaling signingParameters:", err)
		return nil, &types.ErrBadRequest{}
	}

	pubkeyStr := dkgResult.Pubkey
	BKs := dkgResult.BKs
	share := dkgResult.Share

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	c, resp, err := websocket.Dial(ctx, _host+path, nil)
	if err != nil {
		if resp == nil {
			log.Println("error dialing websocket:", err)
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
			log.Println("error dialing websocket:", err)
			return nil, err
		}
	}
	defer c.Close(websocket.StatusInternalError, "the sky is falling")

	signer, err := tss.NewClientSigner(pubkeyStr, share, BKs, message)
	if err != nil {
		log.Println("error when getting new client signer:", err)
		return nil, &types.ErrBadRequest{}
	}

	errs := make(chan error, 2)

	go tss.Send(signer, ctx, errs, c)
	go tss.Receive(signer, ctx, errs, c)

	// Start signing process
	signature, err := signer.Process()
	if err != nil {
		log.Println("error processing signing:", err)
		return nil, &types.ErrTssProcessFailed{}
	}

	// Error management
	processErr := <-errs // wait for websocket closure from server
	if ctx.Err() == context.Canceled {
		log.Println("websocket closed by context cancellation:", processErr)
		return nil, processErr
	} else if websocket.CloseStatus(processErr) == websocket.StatusNormalClosure {
		log.Println("websocket closed normally")
		cancel()
	} else {
		log.Println("error during websocket connection:", processErr) // even if badly closed, we continue as we have the signature
		cancel()
	}

	// time.Sleep(time.Second)

	// c.Close(websocket.StatusNormalClosure, "")

	return signature, nil
}

// Recover recovers the private key from the server and client shares
// Requires the dkgResult (i.e. client-side of wallet), authData (to confirm authorization and identify user) and host
func Recover(host string, dkgResultStr string, metadata string, authData string) (string, error) {

	// Get temporary access token from server based on auth data
	token, err := getAccessToken(host, metadata, authData)
	if err != nil {
		log.Println("error getting access token:", err)
		return "", &types.ErrUnauthorized{}
	}

	// Prepare query
	path := "/recover?token=" + token

	_host, err := urlToHttp(host)
	if err != nil {
		log.Println("error getting ws host:", err)
		return "", &types.ErrBadRequest{}
	}

	// Get client share
	var dkgResult tss.DkgResult
	err = json.Unmarshal([]byte(dkgResultStr), &dkgResult)
	if err != nil {
		log.Println("error unmarshaling signingParameters:", err)
		return "", &types.ErrBadRequest{}
	}

	share := dkgResult.Share

	// Create the form data
	formData := url.Values{
		"share": {share},
	}

	// Send POST request
	resp, err := http.PostForm(_host+path, formData)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return "", &types.ErrBadRequest{}
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error reading response body:", err)
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
		log.Println("error while creating new request:", err)
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+authData)
	req.Header.Set("M-METADATA", metadata)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("error while doing request to", endpoint, ":", err)
		return "", err
	}

	if resp.StatusCode != 200 {
		log.Println("error while", endpoint, ", status not 200")
		log.Printf("resp:%+v\n", resp)
		return "", fmt.Errorf(endpoint, " status not 200")
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("error while reading response body of", endpoint, ":", err)
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
