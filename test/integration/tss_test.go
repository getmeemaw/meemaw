package integration

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getmeemaw/meemaw/client"
	"github.com/getmeemaw/meemaw/server"
	"github.com/getmeemaw/meemaw/server/database"
	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
	"github.com/google/uuid"
)

// test cases to add (common dkg & sign) :
// - if wrong access token (i.e. /authorize status code != 200), specific error, stop process
// - outdated JWT
// - tss message dropped
// - unsecure connexion (ws, http) => should specific error directly
// - tss message with different session ID (see binance tss-lib security guidelines)
// - wrong origin for CORS

//////////////////////
/// TEST SCENARIOS ///
//////////////////////

func TestDkg(t *testing.T) {

	var params map[string]string
	var dkgResultClient *tss.DkgResult
	var dkgResultServer *tss.DkgResult
	var err error
	var testCase string

	///////////////////
	/// TEST 1 : happy path

	testCase = "test 1 (happy path)"

	params = getStdParameters()
	params["dkgResultServerStr"] = ""

	dkgResultClient, dkgResultServer, err = dkgTestProcessLimitedInTime(params)
	if err != nil {
		t.Errorf("Failed %s - Error while dkg : %s", testCase, err)
	}

	if dkgResultClient.Address != dkgResultServer.Address {
		t.Errorf("Failed %s: Different address between client and server", testCase)
	}
	if dkgResultClient.Pubkey.X != dkgResultServer.Pubkey.X || dkgResultClient.Pubkey.Y != dkgResultServer.Pubkey.Y {
		t.Errorf("Failed %s: Different pubkey between client and server", testCase)
	}
	if dkgResultClient.Share == dkgResultServer.Share {
		t.Errorf("Failed %s: Same share between client and server", testCase)
	}

	t.Logf("Successful %s - dkgResultClient: %+v\n", testCase, dkgResultClient)

	///////////////////
	/// TEST 2 : user already has a wallet on server

	// NOTE : TO BE UPDATED WHEN IMPLEMENTING MULTI DEVICE : If new device, should be allowed

	testCase = "test 2 (user already has a wallet on server)"

	params = getStdParameters()

	dkgResultClient, _, err = dkgTestProcessLimitedInTime(params)
	types.ProcessShouldError(testCase, err, &types.ErrConflict{}, dkgResultClient, t)
}

func TestSign(t *testing.T) {

	var params map[string]string
	var signature *tss.Signature
	var err error

	///////////////////
	/// TEST 1 : happy path

	params = getStdParameters()

	signature, err = signingTestProcessLimitedInTime(params)
	if err != nil {
		t.Errorf("Error while signing test 1 happy path: %s", err)
	} else {
		t.Logf("Successful test 1 happy path - signature: %s\n", hex.EncodeToString(signature.Signature))
	}

	///////////////////
	/// TEST 2 : user does not have a wallet on server

	params = getStdParameters()
	params["dkgResultServerStr"] = ""
	// HOW TO STORE USER BUT NOT THE WALLET ? (otherwise same test as test 4)

	signature, err = signingTestProcessLimitedInTime(params)
	types.ProcessShouldError("test 2 (user does not have a wallet on server)", err, &types.ErrNotFound{}, signature, t)

	///////////////////
	/// TEST 3 : different public keys stored on client and server

	params = getStdParameters()
	params["dkgResultServerStr"] = `{"Pubkey":{"X":"64927784304280585002232059641609611887834878205473395822489518307235035286544","Y":"25782693251874019172725009347410644829502824377621177953293307398262537993134"},"BKs":{"client":{"X":"111886675541902333686715770753860772166725964179322493963066654360904646044329","Rank":0},"server":{"X":"105724717407398644489128719447825679148844350134379485277252132502254966714726","Rank":0}},"Share":"98852749347118528790599917495626273581652498656930690683302586059893129350566","Address":"0x5749A8Ed0C00C963c7b19ea05A51131077305c8A"}`

	signature, err = signingTestProcessLimitedInTime(params)
	types.ProcessShouldError("test 3 (different public keys stored on client and server)", err, &types.ErrBadRequest{}, signature, t)

	///////////////////
	/// TEST 4 : user not found

	params = getStdParameters()
	params["dkgResultServerStr"] = ""

	signature, err = signingTestProcessLimitedInTime(params)
	types.ProcessShouldError("test 4 (user not found)", err, &types.ErrNotFound{}, signature, t)

	///////////////////
	/// TEST 5 : wrong share on client

	params = getStdParameters()
	params["dkgResultClientStr"] = `{"Pubkey":{"X":"64927784304280585002232059641609611887834878205473395822489518307235035286543","Y":"25782693251874019172725009347410644829502824377621177953293307398262537993134"},"BKs":{"client":{"X":"111886675541902333686715770753860772166725964179322493963066654360904646044329","Rank":0},"server":{"X":"105724717407398644489128719447825679148844350134379485277252132502254966714726","Rank":0}},"Share":"18768601278517953072996637218594114592905334395321844506825806423166074118045","Address":"0x5749A8Ed0C00C963c7b19ea05A51131077305c8A"}`

	signature, err = signingTestProcessLimitedInTime(params)
	types.ProcessShouldError("test 5 (wrong share on client)", err, &types.ErrTssProcessFailed{}, signature, t)

	///////////////////
	/// TEST 6 : wrong share on server

	params = getStdParameters()
	params["dkgResultServerStr"] = `{"Pubkey":{"X":"64927784304280585002232059641609611887834878205473395822489518307235035286543","Y":"25782693251874019172725009347410644829502824377621177953293307398262537993134"},"BKs":{"client":{"X":"111886675541902333686715770753860772166725964179322493963066654360904646044329","Rank":0},"server":{"X":"105724717407398644489128719447825679148844350134379485277252132502254966714726","Rank":0}},"Share":"98852749347118528790599917495626273581652498656930690683302586059893129350567","Address":"0x5749A8Ed0C00C963c7b19ea05A51131077305c8A"}`

	signature, err = signingTestProcessLimitedInTime(params)
	types.ProcessShouldError("test 6 (wrong share on server)", err, &types.ErrTssProcessFailed{}, signature, t)

}

/////////////
/// UTILS ///
/////////////

func dkgTestProcessLimitedInTime(parameters map[string]string) (*tss.DkgResult, *tss.DkgResult, error) {
	dkgResultArrayCh := make(chan []*tss.DkgResult)
	errorCh := make(chan error)

	go func() {
		dkgResultClient, dkgResultServer, err := dkgTestProcess(parameters)

		if err != nil {
			errorCh <- err
		} else {
			res := make([]*tss.DkgResult, 2)
			res[0] = dkgResultClient
			res[1] = dkgResultServer
			dkgResultArrayCh <- res
		}
	}()

	select {
	case dkgResultArray := <-dkgResultArrayCh:
		return dkgResultArray[0], dkgResultArray[1], nil
	case err := <-errorCh:
		return nil, nil, err
	case <-time.After(1 * time.Minute):
		return nil, nil, &types.ErrTimeOut{}
	}
}

func signingTestProcessLimitedInTime(parameters map[string]string) (*tss.Signature, error) {
	signatureCh := make(chan *tss.Signature)
	errorCh := make(chan error)

	go func() {
		signature, err := signingTestProcess(parameters)

		if err != nil {
			errorCh <- err
		} else {
			signatureCh <- signature
		}
	}()

	select {
	case signature := <-signatureCh:
		return signature, nil
	case err := <-errorCh:
		return nil, err
	case <-time.After(1 * time.Minute):
		return nil, &types.ErrTimeOut{}
	}
}

func dkgTestProcess(parameters map[string]string) (*tss.DkgResult, *tss.DkgResult, error) {

	authServer := httptest.NewServer(http.HandlerFunc(getCustomAuthHandler(parameters["userIdUsed"])))

	var config = server.Config{
		AuthServerUrl: "http://" + authServer.Listener.Addr().String(),
		AuthType:      "custom",
		ClientOrigin:  "localhost",
		DevMode:       true,
	}

	queries := database.New(db)

	_, err := queries.Status(context.Background())
	if err != nil {
		log.Println("Could not connect to db... ", err)
		return nil, nil, err
	}
	log.Println("Connected to db")

	_server := server.NewServer(queries, &config, logging)
	// _server.Start() // No need to start, we test the handler directly

	// // debug : leave time to manually check db status
	// log.Println("wallet stored, check in db !")
	// time.Sleep(2 * time.Minute)

	// Insert wallet in DB (if required)
	if len(parameters["dkgResultServerStr"]) > 0 {
		var dkgResultServer tss.DkgResult
		err = json.Unmarshal([]byte(parameters["dkgResultServerStr"]), &dkgResultServer)
		if err != nil {
			log.Println("error unmarshaling signingParameters:", err)
			return nil, nil, err
		}

		log.Printf("dkgResultServer: %+v\n", dkgResultServer)

		err = _server.StoreWallet(parameters["userAgent"], parameters["userIdStored"], &dkgResultServer)
		if err != nil {
			log.Println("Error storing wallet:", err)
			return nil, nil, err
		}
	}

	meemawServer := httptest.NewServer(_server.Router())
	defer meemawServer.Close()

	host := "http://" + meemawServer.Listener.Addr().String()
	authData := "auth-data-test"

	log.Println("client.Dkg with host:", host, " and authData:", authData)
	log.Printf("%q", host)

	dkgResultClient, err := client.Dkg(host, authData)
	if err != nil {
		log.Println("Error client.Dkg:", err)
		return nil, nil, err
	}

	// // debug : leave time to manually check db status
	// log.Println("wallet stored, check in db !")
	// time.Sleep(2 * time.Minute)

	time.Sleep(1 * time.Second) // Give it 1 second to make sure it's in DB. Sometimes the test fails because of race conditions.

	res, err := queries.GetUserSigningParameters(context.Background(), parameters["userIdUsed"])
	if err != nil {
		log.Println("Error getting user signing parameters:", err)
		return nil, nil, err
	}

	var signingParamsServer tss.SigningParameters
	err = json.Unmarshal(res.Params.RawMessage, &signingParamsServer)
	if err != nil {
		return nil, nil, err
	}

	dkgResultServer := &tss.DkgResult{
		Pubkey:  signingParamsServer.Pubkey,
		BKs:     signingParamsServer.BKs,
		Share:   res.Share.String,
		Address: res.PublicAddress.String,
	}

	return dkgResultClient, dkgResultServer, nil
}

func signingTestProcess(parameters map[string]string) (*tss.Signature, error) {

	authServer := httptest.NewServer(http.HandlerFunc(getCustomAuthHandler(parameters["userIdUsed"])))

	var config = server.Config{
		AuthServerUrl: "http://" + authServer.Listener.Addr().String(),
		AuthType:      "custom",
		ClientOrigin:  "localhost",
		DevMode:       true,
	}

	queries := database.New(db)

	_, err := queries.Status(context.Background())
	if err != nil {
		return nil, err
	}
	log.Println("Connected to db")

	_server := server.NewServer(queries, &config, logging)
	// _server.Start() // No need to start, we test the handler directly

	// Insert wallet in DB (if required)
	if len(parameters["dkgResultServerStr"]) > 0 {
		var dkgResultServer tss.DkgResult
		err = json.Unmarshal([]byte(parameters["dkgResultServerStr"]), &dkgResultServer)
		if err != nil {
			log.Println("error unmarshaling signingParameters:", err)
			return nil, err
		}

		log.Printf("dkgResultServer: %+v\n", dkgResultServer)

		err = _server.StoreWallet(parameters["userAgent"], parameters["userIdStored"], &dkgResultServer)
		if err != nil {
			return nil, err
		}
	}

	// debug : leave time to manually check db status
	// log.Println("wallet stored, check in db !")
	// time.Sleep(2 * time.Minute)

	meemawServer := httptest.NewServer(_server.Router())
	defer meemawServer.Close()

	host := "http://" + meemawServer.Listener.Addr().String()
	authData := "auth-data-test"

	log.Println("client.Sign with host:", host, " and authData:", authData)
	log.Printf("%q", host)

	signature, err := client.Sign(host, []byte("test"), parameters["dkgResultClientStr"], authData)
	if err != nil {
		return nil, err
	}

	return signature, nil
}

type AuthData struct {
	Auth string `json:"auth"`
}

// getCustomAuthHandler is used to mock an auth provider with very simplistic logic : it will always return retUserId when asked for a userID based on authData (JWT or other)
func getCustomAuthHandler(retUserId string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(retUserId))
	}
}

func getStdParameters() map[string]string {
	stdParameters := make(map[string]string)

	guid := uuid.New().String()

	stdParameters["userIdStored"] = "my-super-user-id-" + guid
	stdParameters["userIdUsed"] = "my-super-user-id-" + guid
	stdParameters["dkgResultServerStr"] = `{"Pubkey":{"X":"64927784304280585002232059641609611887834878205473395822489518307235035286543","Y":"25782693251874019172725009347410644829502824377621177953293307398262537993134"},"BKs":{"client":{"X":"111886675541902333686715770753860772166725964179322493963066654360904646044329","Rank":0},"server":{"X":"105724717407398644489128719447825679148844350134379485277252132502254966714726","Rank":0}},"Share":"98852749347118528790599917495626273581652498656930690683302586059893129350566","Address":"0x5749A8Ed0C00C963c7b19ea05A51131077305c8A"}`
	stdParameters["dkgResultClientStr"] = `{"Pubkey":{"X":"64927784304280585002232059641609611887834878205473395822489518307235035286543","Y":"25782693251874019172725009347410644829502824377621177953293307398262537993134"},"BKs":{"client":{"X":"111886675541902333686715770753860772166725964179322493963066654360904646044329","Rank":0},"server":{"X":"105724717407398644489128719447825679148844350134379485277252132502254966714726","Rank":0}},"Share":"18768601278517953072996637218594114592905334395321844506825806423166074118044","Address":"0x5749A8Ed0C00C963c7b19ea05A51131077305c8A"}`
	stdParameters["userAgent"] = "my-super-user-agent-" + guid

	return stdParameters
}
