package tx

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
)

func TestParseBigInt(t *testing.T) {
	type Test struct {
		input  any
		output *big.Int
	}

	tests := []Test{
		Test{input: "", output: big.NewInt(0)},
		Test{input: "1", output: big.NewInt(1)},
		Test{input: "1.0", output: big.NewInt(1)},
		Test{input: "1.0.1", output: big.NewInt(0)},
		Test{input: 1.0, output: big.NewInt(1)},
		Test{input: 1, output: big.NewInt(1)},
		Test{input: big.NewInt(1), output: big.NewInt(1)},
		Test{input: "0xa", output: big.NewInt(10)},
	}

	for _, test := range tests {
		res := ParseBigInt(test.input)
		if res.String() != test.output.String() {
			t.Errorf("Failed test (input %s) : expected %s, got %s\n", test.input, test.output.String(), res.String())
		} else {
			t.Logf("Successful test (input %s) : expected %s, got %s\n", test.input, test.output.String(), res.String())
		}
	}
}

func TestSign(t *testing.T) {

	var testDescription string
	var jsonEncodedTx string
	var hexEncodedSignature string
	var chainId int
	var err error

	//////////
	// Test 1 (happy path)

	testDescription = "test 1 (happy path)"

	jsonEncodedTx = `{"to":"0x809ccc37d2dd55a8e8fa58fc51d101c6b22425a8","value":10000000000000,"nonce":5,"gasLimit":21000,"gasPrice":34}`
	hexEncodedSignature = "aec485f62b6ea68cbbf7df13461e73cf0e60089bb0c95f360ee9c375a5ba8b2415612f12913fafd8734ae9735847a9321bc798b7b0b528a214d0b519af0dcf4701"
	chainId = 1

	err = processSign(testDescription, jsonEncodedTx, hexEncodedSignature, chainId)
	if err != nil {
		t.Errorf(err.Error())
	} else {
		t.Logf("Successful " + testDescription + ": transactions correspond\n")
	}

	//////////
	// Test 2 (different chainId)

	testDescription = "test 2 (different chainId)"

	jsonEncodedTx = `{"to":"0x809ccc37d2dd55a8e8fa58fc51d101c6b22425a8","value":10000000000000,"nonce":5,"gasLimit":21000,"gasPrice":34}`
	hexEncodedSignature = "aec485f62b6ea68cbbf7df13461e73cf0e60089bb0c95f360ee9c375a5ba8b2415612f12913fafd8734ae9735847a9321bc798b7b0b528a214d0b519af0dcf4701"
	chainId = 11155111

	err = processSign(testDescription, jsonEncodedTx, hexEncodedSignature, chainId)
	if err != nil {
		t.Errorf(err.Error())
	} else {
		t.Logf("Successful " + testDescription + ": transactions correspond\n")
	}

	//////////
	// Test 3 (wrongly formatted signature)

	testDescription = "test 3 (wrongly formatted signature)"

	jsonEncodedTx = `{"to":"0x809ccc37d2dd55a8e8fa58fc51d101c6b22425a8","value":10000000000000,"nonce":5,"gasLimit":21000,"gasPrice":34}`
	hexEncodedSignature = "bec485f62b6ea68cbbf7df13461e73cf0e60089bb0c95f360ee9c375a5ba8b2415612f12913fafd8734ae9735847a9321bc798b7b0b528a214d0b519af0dcf4701"
	chainId = 11155111

	err = processSign(testDescription, jsonEncodedTx, hexEncodedSignature, chainId)
	if err != nil {
		t.Logf("Successful "+testDescription+": received error when trying sign with wrong signature: %s\n", err)
	} else {
		t.Errorf("Failed " + testDescription + ": did not error with wrong signature.")
	}
}

func processSign(testDescription string, jsonEncodedTx string, hexEncodedSignature string, chainId int) error {
	//////////
	// Create and sign original transaction (output: signed raw tx)

	signature, err := hex.DecodeString(hexEncodedSignature)
	if err != nil {
		return fmt.Errorf("Failed "+testDescription+": could not decode hexEncodedSignature: %s\n", err)
	}

	_tx, err := NewEthereumTxWithJson(jsonEncodedTx, chainId)
	if err != nil {
		return fmt.Errorf("Failed "+testDescription+": could not create Ethereum TX with json: %s\n", err)
	}

	hexEncodedRawSignedTx, err := _tx.Sign(signature)

	rawSignedTx, err := hex.DecodeString(hexEncodedRawSignedTx)
	if err != nil {
		return fmt.Errorf("Failed "+testDescription+": could not decode hexEncodedRawSignedTx: %s\n", err)
	}

	//////////
	// Recover a transaction from the signed raw tx

	recoveredTx := new(types.Transaction)
	err = recoveredTx.UnmarshalBinary(rawSignedTx)
	if err != nil {
		return fmt.Errorf("Failed "+testDescription+": could not rlp decode rawSignedTx: %s\n", err)
	}

	// Recover the public key from the signatures.
	signer := types.NewEIP155Signer(ParseBigInt(chainId))

	sender, err := types.Sender(signer, _tx.tx)
	if err != nil {
		return fmt.Errorf("Failed "+testDescription+": could not find sender of original transaction: %s\n", err)
	}
	recoveredSender, err := types.Sender(signer, recoveredTx)
	if err != nil {
		return fmt.Errorf("Failed "+testDescription+": could not find sender of recoveredTx: %s\n", err)
	}

	//////////
	// Compare original transaction with transaction recovered from signature

	if _tx.tx.Nonce() != recoveredTx.Nonce() {
		return fmt.Errorf("Failed "+testDescription+": nonces do not correspond:\n_tx.tx.Nonce() : %d, recoveredTx.Nonce() : %d\n", _tx.tx.Nonce(), recoveredTx.Nonce())
	} else if sender.Hex() != recoveredSender.Hex() {
		return fmt.Errorf("Failed "+testDescription+": senders do not correspond:\nsender.Hex(): %s, recoveredSender.Hex() : %s\n", sender.Hex(), recoveredSender.Hex())
	} else if _tx.tx.To().Hex() != recoveredTx.To().Hex() {
		return fmt.Errorf("Failed "+testDescription+": TOs do not correspond:\n_tx.tx.To() : %s, recoveredTx.To() : %s\n", _tx.tx.To().Hex(), recoveredTx.To().Hex())
	} else if _tx.tx.Value().String() != recoveredTx.Value().String() {
		return fmt.Errorf("Failed "+testDescription+": values do not correspond:\n_tx.tx.Value() : %s, recoveredTx.Value() : %s\n", _tx.tx.Value(), recoveredTx.Value())
	}

	return nil
}
