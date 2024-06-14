package integration

import (
	"context"
	"encoding/json"
	"log"
	"testing"

	"github.com/getmeemaw/meemaw/server"
	"github.com/getmeemaw/meemaw/server/database"
	"github.com/getmeemaw/meemaw/server/vault"
	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
)

func TestStoreAndRetrieveWallet(t *testing.T) {

	var config = server.Config{
		ClientOrigin: "localhost",
	}

	queries := database.New(db)

	vault := vault.NewVault(queries)

	ctx := context.Background()

	_, err := queries.Status(ctx)
	if err != nil {
		t.Errorf("Failed RetrieveWallet: could not get db access\n")
	}
	log.Println("Connected to db")

	_server := server.NewServer(vault, &config, nil, logging)

	dkgResultStr := `{"Pubkey":{"X":"64927784304280585002232059641609611887834878205473395822489518307235035286543","Y":"25782693251874019172725009347410644829502824377621177953293307398262537993134"},"BKs":{"client":{"X":"111886675541902333686715770753860772166725964179322493963066654360904646044329","Rank":0},"server":{"X":"105724717407398644489128719447825679148844350134379485277252132502254966714726","Rank":0}},"Share":"98852749347118528790599917495626273581652498656930690683302586059893129350566","Address":"0x5749A8Ed0C00C963c7b19ea05A51131077305c8A"}`

	var dkgResult tss.DkgResult
	err = json.Unmarshal([]byte(dkgResultStr), &dkgResult)
	if err != nil {
		t.Errorf("Failed RetrieveWallet: could not unmarshal dkgResult\n")
	}

	var dkgResultRetrieved *tss.DkgResult
	var testDescription string

	///////////////////
	/// TEST 1 : happy path (store and retrieve)

	testDescription = "test 1 (happy case)"
	successful := true

	metadata, err := _server.Vault().StoreWallet(ctx, "userAgent", "my-user-id-retrieve-one", &dkgResult)
	if err != nil {
		successful = false
		t.Errorf("Failed "+testDescription+": could not store dkgResult: %+v\n", dkgResult)
	}

	ctx = context.WithValue(ctx, types.ContextKey("metadata"), metadata)

	dkgResultRetrieved, err = _server.Vault().RetrieveWallet(ctx, "my-user-id-retrieve-one")
	if err != nil {
		successful = false
		t.Errorf("Failed "+testDescription+": expected dkgResult, got error: %s\n", err)
	}

	// Test reflect.DeepEqual
	if dkgResult.Pubkey.X != dkgResultRetrieved.Pubkey.X || dkgResult.Pubkey.Y != dkgResultRetrieved.Pubkey.Y || dkgResult.Share != dkgResultRetrieved.Share || dkgResult.Address != dkgResultRetrieved.Address {
		successful = false
		t.Errorf("Failed "+testDescription+": expected dkgResult stored to be retrieved intact - expected %+v, got %+v\n", dkgResult, dkgResultRetrieved)
	}

	for key, value := range dkgResult.BKs {
		val, ok := dkgResultRetrieved.BKs[key]
		if ok {
			if val != value {
				successful = false
				t.Errorf("Failed "+testDescription+": expected dkgResult stored to be retrieved intact - expected %+v, got %+v\n", dkgResult.BKs[key], dkgResultRetrieved.BKs[key])
			}
		} else {
			successful = false
			t.Errorf("Failed "+testDescription+": expected dkgResult stored to be retrieved intact - expected %+v, got nothing\n", dkgResult.BKs[key])
		}
	}

	if successful {
		t.Logf("Successful %s : expected %+v, got %+v\n", testDescription, dkgResult, dkgResultRetrieved)
	}

	///////////////////
	/// TEST 2 : foreign key not found

	testDescription = "test 2 (foreign key not found)"

	dkgResultRetrieved, err = _server.Vault().RetrieveWallet(ctx, "my-user-id-not-found")
	types.ProcessShouldError(testDescription, err, &types.ErrNotFound{}, dkgResultRetrieved, t)

	///////////////////
	/// TEST 3 : store wallet for existing user but new device (still happy path)

	// ADD WHEN DEVELOPING MULTI-DEVICE

}
