package integration

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getmeemaw/meemaw/client"
	"github.com/getmeemaw/meemaw/server"
	"github.com/getmeemaw/meemaw/server/database"
	"github.com/getmeemaw/meemaw/server/vault"
	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
)

//////////////////////
/// TEST SCENARIOS ///
//////////////////////

func TestMultiDevice(t *testing.T) {

	log.Println("starting TestMultiDevice")

	///////////////////
	/// TEST 1 : happy path

	testCase := "test 1 (happy path)"

	err := multiDeviceTestProcessLimitedInTime()
	if err != nil {
		t.Errorf("Failed %s - Error while multiDevice : %s", testCase, err)
	}

	t.Logf("Successful %s\n", testCase)

	///////////////////
	/// TODO

	// - registerDevice when there is no existing device/wallet
	// - acceptDevice when there is no device to register
	// - two devices registering
	// - two devices accepting
}

/////////////
/// UTILS ///
/////////////

func multiDeviceTestProcessLimitedInTime() error {
	errorCh := make(chan error)
	doneCh := make(chan struct{})

	go func() {
		err := multiDeviceTestProcess()

		if err != nil {
			errorCh <- err
		}

		doneCh <- struct{}{}
	}()

	select {
	case <-doneCh:
		log.Println("multi device test done")
		return nil
	case err := <-errorCh:
		return err
	case <-time.After(1 * time.Minute):
		log.Println("Aleeeert, time out")
		return &types.ErrTimeOut{}
	}
}

func multiDeviceTestProcess() error {
	userId := "my-user"

	authServer := httptest.NewServer(http.HandlerFunc(getCustomAuthHandler(userId)))
	defer authServer.Close()

	var config = server.Config{
		AuthServerUrl: "http://" + authServer.Listener.Addr().String(),
		AuthType:      "custom",
		ClientOrigin:  "localhost",
		DevMode:       true,
		MultiDevice:   true,
	}

	queries := database.New(db)

	vault := vault.NewVault(queries)

	ctx := context.Background()

	_, err := queries.Status(ctx)
	if err != nil {
		log.Println("Could not connect to db... ", err)
		return err
	}
	log.Println("Connected to db")

	_server := server.NewServer(vault, &config, nil, logging)
	// _server.Start() // No need to start, we test the handler directly

	// // debug : leave time to manually check db status
	// log.Println("wallet stored, check in db !")
	// time.Sleep(2 * time.Minute)

	meemawServer := httptest.NewServer(_server.Router())
	defer meemawServer.Close()

	host := "http://" + meemawServer.Listener.Addr().String()
	authData := "auth-data-test"

	log.Println("client.Dkg with host:", host, " and authData:", authData)
	log.Printf("%q", host)

	// Generate wallet first client + server
	dkgResultFirstClient, metadataFirstClient, err := client.Dkg(host, authData)
	if err != nil {
		log.Println("Error client.Dkg:", err)
		panic(err)
	}

	// Add other devices
	dkgResultSecondClient, metadataSecondClient, err := addDevice(host, authData, dkgResultFirstClient, metadataFirstClient)
	if err != nil {
		log.Println("Error addDevice:", err)
		return err
	}

	dkgResultThirdClient, metadataThirdClient, err := addDevice(host, authData, dkgResultSecondClient, metadataSecondClient)
	if err != nil {
		log.Println("Error addDevice:", err)
		return err
	}

	dkgResultFourthClient, metadataFourthClient, err := addDevice(host, authData, dkgResultFirstClient, metadataFirstClient)
	if err != nil {
		log.Println("Error addDevice:", err)
		return err
	}

	// Test values
	if metadataFirstClient != metadataSecondClient || metadataFirstClient != metadataThirdClient || metadataFirstClient != metadataFourthClient {
		return errors.New("different metadata")
	}

	if dkgResultFirstClient.Pubkey.X != dkgResultSecondClient.Pubkey.X ||
		dkgResultFirstClient.Pubkey.Y != dkgResultSecondClient.Pubkey.Y ||
		dkgResultFirstClient.Pubkey.X != dkgResultThirdClient.Pubkey.X ||
		dkgResultFirstClient.Pubkey.Y != dkgResultThirdClient.Pubkey.Y ||
		dkgResultFirstClient.Pubkey.X != dkgResultFourthClient.Pubkey.X ||
		dkgResultFirstClient.Pubkey.Y != dkgResultFourthClient.Pubkey.Y {
		return errors.New("different pubkey")
	}

	time.Sleep(1 * time.Second) // Give it 1 second to make sure it's in DB. Sometimes the test fails because of race conditions.

	ctx = context.WithValue(ctx, types.ContextKey("metadata"), metadataFirstClient)

	dkgResultServer, err := _server.Vault().RetrieveWallet(ctx, userId)
	if err != nil {
		log.Println("Error retrieveWallet:", err)
		return err
	}

	if _, exists := dkgResultServer.BKs[dkgResultFirstClient.PeerID]; !exists {
		return errors.New("server BKs dont have first client peerID")
	}

	if _, exists := dkgResultServer.BKs[dkgResultSecondClient.PeerID]; !exists {
		return errors.New("server BKs dont have second client peerID")
	}

	if _, exists := dkgResultServer.BKs[dkgResultThirdClient.PeerID]; !exists {
		return errors.New("server BKs dont have third client peerID")
	}

	if _, exists := dkgResultServer.BKs[dkgResultFourthClient.PeerID]; !exists {
		return errors.New("server BKs dont have fourth client peerID")
	}

	return nil
}

func addDevice(host, authData string, dkgResultFirstClient *tss.DkgResult, metadataFirstClient string) (*tss.DkgResult, string, error) {
	// Add new device
	newClientDone := make(chan struct{})
	var dkgResultNewClient *tss.DkgResult
	var metadataNewClient string

	var err error
	errs := make(chan error, 1)

	go func() {
		log.Println("AddDevice - starting registerDevice")
		dkgResultNewClient, metadataNewClient, err = client.RegisterDevice(host, authData, "device")
		if err != nil {
			log.Println("Error registerDevice:", err)
			errs <- err
			return
		}

		newClientDone <- struct{}{}
	}()

	time.Sleep(200 * time.Millisecond)

	log.Println("AddDevice - starting acceptDevice")

	dkgResultFirstClientBytes, err := json.Marshal(dkgResultFirstClient)
	if err != nil {
		log.Println("Error marshaling dkgResult:", err)
		return nil, "", err
	}

	err = client.AcceptDevice(host, string(dkgResultFirstClientBytes), metadataFirstClient, authData)
	if err != nil {
		log.Println("Error acceptDevice:", err)
		return nil, "", err
	}

	log.Println("client.AcceptDevice done")

	<-newClientDone

	select {
	case processErr := <-errs:
		log.Println("error during RegisterDevice:", processErr)
		return nil, "", processErr
	default:
		log.Println("no error during RegisterDevice")
	}

	return dkgResultNewClient, metadataNewClient, nil
}
