package vault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"

	"github.com/getmeemaw/meemaw/server/database"
	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/types"
)

type Vault struct {
	_queries *database.Queries
}

func NewVault(queries *database.Queries) *Vault {
	return &Vault{_queries: queries}
}

// WalletExists verifies if a wallet already exists
func (vault *Vault) WalletExists(ctx context.Context, userId string) error {
	_, err := vault._queries.GetUserByForeignKey(ctx, userId)
	return err
}

///////

// StoreWallet upserts a wallet (if it already exists, it does nothing, no error returned)
// Tested in integration tests (with throw away db)
func (vault *Vault) StoreWallet(ctx context.Context, userAgent string, foreignKey string, dkgResults *tss.DkgResult) (string, error) {

	// Encode dkgResults to json
	jsonDkgResults, err := json.Marshal(dkgResults)
	if err != nil {
		log.Println("could not marshal dkgResults to json")
		return "", err
	}

	// Generate client key :
	clientKey := make([]byte, 32) // return to client
	if _, err := io.ReadFull(rand.Reader, clientKey); err != nil {
		return "", err
	}

	// Encrypt dkgResults with client key (so that server shares are not fully exposed in case of a breach)
	nonceClient, ClientEncryptedDkgResults, err := encryptAES(jsonDkgResults, clientKey)
	if err != nil {
		log.Println("error while encrypting with client key:", err)
		return "", err
	}

	// Store in DB
	user, err := vault._queries.AddUser(ctx, foreignKey)
	if err != nil {
		return "", err
	}

	walletQueryParams := database.AddWalletParams{
		UserId:              user.ID,
		PublicAddress:       dkgResults.Address,
		EncryptedDkgResults: ClientEncryptedDkgResults,
		Nonce:               nonceClient,
	}

	wallet, err := vault._queries.AddWallet(ctx, walletQueryParams)
	if err != nil {
		return "", err
	}

	deviceQueryParams := database.AddDeviceParams{
		UserId:    user.ID,
		WalletId:  wallet.ID,
		UserAgent: userAgent,
	}

	_, err = vault._queries.AddDevice(ctx, deviceQueryParams)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(clientKey), nil
}

// RetrieveWallet retrieves a wallet from DB based on the userID of the user (which is a loose foreign key, the format will depend on the auth provider)
// Tested in integration tests (with throw away db)
func (vault *Vault) RetrieveWallet(ctx context.Context, foreignKey string) (*tss.DkgResult, error) {

	// get dkgResults
	res, err := vault._queries.GetUserSigningParameters(ctx, foreignKey)
	if err != nil {
		log.Println("error getting signing params:", err)
		return nil, &types.ErrNotFound{}
	}

	// get client key from context
	clientKeyStr, ok := ctx.Value(types.ContextKey("metadata")).(string)
	if !ok {
		return nil, errors.New("could not find customer identifier")
	}

	clientKey, err := hex.DecodeString(clientKeyStr)
	if err != nil {
		log.Println("error hex decoding clientKey(", clientKeyStr, "):", err)
		return nil, err
	}

	// decrypt dkg results
	jsonDkgResults, err := decryptAES(res.Nonce, res.EncryptedDkgResults, clientKey)
	if err != nil {
		log.Println("could not decrypt AES using clientKey:", err)
		return nil, err
	}

	// decode json into *tss.DkgResult
	dkgResult := &tss.DkgResult{}
	err = json.Unmarshal(jsonDkgResults, dkgResult)
	if err != nil {
		log.Println("could not unmarshal jsonDkgResults")
		return nil, err
	}

	return dkgResult, nil
}

// Encrypt a plaintext message using AES-GCM.
func encryptAES(plaintext, key []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	// Generate a random nonce. Ensure it is unique for each encryption with the same key.
	nonce, err := generateRandomBytes(aesGCM.NonceSize())
	if err != nil {
		return nil, nil, err
	}

	// Encrypt the plaintext using the nonce.
	ciphertext := aesGCM.Seal(nil, nonce, plaintext, nil)
	return nonce, ciphertext, nil
}

// Decrypt a ciphertext message using AES-GCM.
func decryptAES(nonce, ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Decrypt the ciphertext using the nonce.
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// Generate random bytes using crypto/rand, which is secure for cryptographic purposes.
func generateRandomBytes(size int) ([]byte, error) {
	bytes := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return nil, err
	}
	return bytes, nil
}
