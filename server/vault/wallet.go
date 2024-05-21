package vault

import (
	"context"
	"encoding/json"
	"log"

	"github.com/getmeemaw/meemaw/server/database"
	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
)

type Vault struct {
	_queries *database.Queries
}

func New(queries *database.Queries) *Vault {
	return &Vault{_queries: queries}
}

// RetrieveWallet retrieves a wallet from DB based on the userID of the user (which is a loose foreign key, the format will depend on the auth provider)
// Tested in integration tests (with throw away db)
func (vault *Vault) RetrieveWallet(ctx context.Context, foreignKey string) (*tss.DkgResult, error) {
	res, err := vault._queries.GetUserSigningParameters(ctx, foreignKey)
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
func (vault *Vault) StoreWallet(ctx context.Context, userAgent string, userId string, dkgResults *tss.DkgResult) error {
	user, err := vault._queries.AddUser(ctx, userId)
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

	walletQueryParams := database.AddWalletParams{
		UserId:        user.ID,
		PublicAddress: dkgResults.Address,
		Share:         dkgResults.Share,
		Params:        params,
	}

	wallet, err := vault._queries.AddWallet(ctx, walletQueryParams)
	if err != nil {
		return err
	}

	deviceQueryParams := database.AddDeviceParams{
		UserId:    user.ID,
		WalletId:  wallet.ID,
		UserAgent: userAgent,
	}

	_, err = vault._queries.AddDevice(ctx, deviceQueryParams)
	if err != nil {
		return err
	}

	return nil
}

// WalletExists verifies if a wallet already exists
func (vault *Vault) WalletExists(ctx context.Context, userId string) error {
	_, err := vault._queries.GetUserByForeignKey(ctx, userId)
	return err
}
