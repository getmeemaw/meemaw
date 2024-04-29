package tx

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// Helpers allowing for easy transactions through the SDKs

////////////////
/// ETHEREUM ///
////////////////

type EthereumTx struct {
	tx     *types.Transaction
	signer *types.EIP155Signer
	hash   []byte
}

func (tx *EthereumTx) Tx() *types.Transaction {
	return tx.tx
}

func NewEthereumTxWithRlp(encodedRawTx []byte) (*EthereumTx, error) {
	var tx types.Transaction
	err := rlp.DecodeBytes(encodedRawTx, &tx)
	if err != nil {
		log.Println("error decoding rlp encoded raw tx")
		return nil, err
	}

	signer := types.NewEIP155Signer(tx.ChainId())

	ethTx := &EthereumTx{
		tx:     &tx,
		signer: &signer,
	}

	return ethTx, nil
}

func NewEthereumTxWithHash(encodedRawTx []byte, hash []byte) (*EthereumTx, error) {
	tx, err := NewEthereumTxWithRlp(encodedRawTx)
	if err != nil {
		return nil, err
	}
	tx.hash = hash
	return tx, nil
}

// NewEthereumTxWithJson creates a new EthereumTx object based on a json encoded transaction
// Used to process a transaction coming from the client
func NewEthereumTxWithJson(jsonData string, chainIdAny any) (*EthereumTx, error) {
	var params TransactionParams
	err := json.Unmarshal([]byte(jsonData), &params)
	if err != nil {
		log.Println("error unmarshaling ethereum tx json:", err)
		return nil, err
	}

	chainId := ParseBigInt(chainIdAny)

	nonce := ParseBigInt(params.Nonce).Uint64()
	value := ParseBigInt(params.Value)
	gasLimit := ParseBigInt(params.GasLimit).Uint64()
	to := common.HexToAddress(params.To)
	data := common.FromHex(params.Data)

	var tx *types.Transaction

	if params.MaxFeePerGas != nil {
		maxFeePerGas := ParseBigInt(params.MaxFeePerGas)
		maxPriorityFeePerGas := ParseBigInt(params.MaxPriorityFeePerGas)
		tx = types.NewTx(&types.DynamicFeeTx{
			ChainID:    chainId, // replace with your ChainID
			Nonce:      nonce,
			To:         &to,
			Value:      value,
			GasFeeCap:  maxFeePerGas,
			GasTipCap:  maxPriorityFeePerGas,
			Gas:        gasLimit,
			Data:       data,
			AccessList: params.AccessList,
		})
	} else {
		gasPrice := ParseBigInt(params.GasPrice)
		tx = types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			GasPrice: gasPrice,
			Gas:      gasLimit,
			To:       &to,
			Value:    value,
			Data:     data,
		})
	}

	signer := types.NewEIP155Signer(chainId)

	return &EthereumTx{tx: tx, signer: &signer}, nil
}

// GenerateMessage generates the hash to be signed through the TSS signing process for the transaction
func (tx *EthereumTx) GenerateMessage() []byte {
	message := tx.signer.Hash(tx.tx)
	tx.hash = message.Bytes()

	return tx.hash
}

// Sign adds the signature to tx and returns the rlp encoded raw transaction
// (to be processed by web3.js or other)
func (tx *EthereumTx) Sign(signature []byte) (string, error) {

	var err error

	// Add signature to tx
	tx.tx, err = tx.tx.WithSignature(tx.signer, signature)
	if err != nil {
		log.Println("Error signing transaction:", err)
		return "", err
	}

	// Compute and return rlp encoded rawTx
	buf := new(bytes.Buffer)
	err = tx.tx.EncodeRLP(buf)
	if err != nil {
		log.Println("Error encoding RLP:", err)
		return "", err
	}
	rawTxBytes := buf.Bytes()

	rawTxHex := hex.EncodeToString(rawTxBytes)

	return rawTxHex, nil
}

type TransactionParams struct {
	To                   string           `json:"to"`
	Nonce                interface{}      `json:"nonce"`
	Value                interface{}      `json:"value"`
	GasLimit             interface{}      `json:"gasLimit"`
	GasPrice             interface{}      `json:"gasPrice"`
	Data                 string           `json:"data"`
	AccessList           types.AccessList `json:"accessList"`
	MaxFeePerGas         interface{}      `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas interface{}      `json:"maxPriorityFeePerGas,omitempty"`
}

// ParseBigInt takes "something" and transforms it into a bigInt if it can
// Allows for more generic json inputs from the SDKs
// e.g. the nonce could be represented as a hex encoded string, a dec encoded string or an int
func ParseBigInt(value interface{}) *big.Int {
	switch v := value.(type) {
	case int64:
		return big.NewInt(v)
	case int:
		return big.NewInt(int64(v))
	case *big.Int:
		return v
	case float64:
		return big.NewInt(int64(v))
	case string:
		if len(v) > 2 && v[:2] == "0x" {
			result, successful := new(big.Int).SetString(v[2:], 16)
			if successful {
				return result
			} else {
				return big.NewInt(0)
			}
		} else if strings.Contains(v, ".") {
			result, successful := new(big.Float).SetString(v)
			if successful {
				ret, _ := result.Int(nil)
				return ret
			} else {
				return big.NewInt(0)
			}
		} else {
			result, successful := new(big.Int).SetString(v, 10)
			if successful {
				return result
			} else {
				return big.NewInt(0)
			}
		}
	default:
		return big.NewInt(0)
	}
}
