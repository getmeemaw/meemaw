package server

import (
	"context"
	"encoding/json"
	"log"

	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
)

// RetrieveWallet retrieves a wallet from DB based on the userID of the user (which is a loose foreign key, the format will depend on the auth provider)
// Tested in integration tests (with throw away db)
func (server *Server) RetrieveWallet(foreignKey string) (*tss.DkgResult, error) {
	res, err := server._queries.GetUserSigningParameters(context.Background(), foreignKey)
	if err != nil {
		log.Println("error getting signing params:", err)
		return nil, &types.ErrNotFound{}
	}

	// log.Println("GetUserSigningParameters res.Params.RawMessage:", string(res.Params.RawMessage))

	var signingParams tss.SigningParameters
	err = json.Unmarshal(res.Params.RawMessage, &signingParams)
	if err != nil {
		log.Println("error unmarshaling signing params:", err)
		return nil, err
	}

	dkgResult := tss.DkgResult{
		Pubkey:  signingParams.Pubkey,
		BKs:     signingParams.BKs,
		Share:   res.Share.String,
		Address: res.PublicAddress.String,
	}

	return &dkgResult, nil
}

// StoreWallet upserts a wallet (if it already exists, it does nothing, no error returned)
// Tested in integration tests (with throw away db)
func (server *Server) StoreWallet(userAgent string, userId string, dkgResults *tss.DkgResult) error {
	user, err := server._queries.AddUser(context.Background(), userId)
	if err != nil {
		return err
	}

	signingParams := tss.SigningParameters{
		Pubkey: dkgResults.Pubkey,
		BKs:    dkgResults.BKs,
	}

	params, err := json.Marshal(signingParams)
	if err != nil {
		return err
	}

	walletQueryParams := AddWalletParams{
		UserId:        user.ID,
		PublicAddress: dkgResults.Address,
		Share:         dkgResults.Share,
		Params:        params,
	}

	wallet, err := server._queries.AddWallet(context.Background(), walletQueryParams)
	if err != nil {
		return err
	}

	deviceQueryParams := AddDeviceParams{
		UserId:    user.ID,
		WalletId:  wallet.ID,
		UserAgent: userAgent,
	}

	_, err = server._queries.AddDevice(context.Background(), deviceQueryParams)
	if err != nil {
		return err
	}

	return nil
}
