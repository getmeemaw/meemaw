//go:build js && wasm

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"syscall/js"

	"github.com/getmeemaw/meemaw/client"
	"github.com/getmeemaw/meemaw/utils/tss"
	"github.com/getmeemaw/meemaw/utils/tx"
)

// compile :
// GOOS=js GOARCH=wasm go build -o wasm_std.wasm ./main.go
// 		GOOS=js GOARCH=wasm go build -o wasm_std.wasm ./main.go && mv wasm_std.wasm ../wasm.wasm

// Get JS file (wasm_exec.js) :
// std go : cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" .

func main() {

	js.Global().Set("Identify", asyncFunc(Identify))
	js.Global().Set("Dkg", asyncFunc(Dkg))
	js.Global().Set("SignBytes", asyncFunc(SignBytes))
	// js.Global().Set("SignEthMessage", asyncFunc(SignEthMessage))
	js.Global().Set("SignEthTransaction", asyncFunc(SignEthTransaction))
	js.Global().Set("Recover", asyncFunc(Recover))

	select {}
}

// input : host, authData
// output : userId (string), error
func Identify(this js.Value, args []js.Value) (any, error) {
	host := args[0].String()
	authData := args[1].String()

	userId, err := client.Identify(host, authData)
	if err != nil {
		log.Println("error while getting userId:", err)
		return nil, err
	}

	return string(userId), err
}

type dkgResponse struct {
	DkgResult *tss.DkgResult `json:"dkgResult"`
	Metadata  string         `json:"metadata"`
}

// input : host, authData
// output : json encoded dkgResult, error
func Dkg(this js.Value, args []js.Value) (any, error) {
	host := args[0].String()
	authData := args[1].String()

	dkgResult, metadata, err := client.Dkg(host, authData)
	if err != nil {
		log.Println("error while dkg:", err)
		return nil, err
	}

	resp := dkgResponse{
		DkgResult: dkgResult,
		Metadata:  metadata,
	}

	respJSON, err := json.Marshal(resp)
	if err != nil {
		log.Println("error while marshaling dkgresult json:", err)
		return nil, err
	}

	return string(respJSON), err
}

// input : host, message (hex encoded bytes), dkgResultStr, authData
// output : signed message, error
func SignBytes(this js.Value, args []js.Value) (any, error) {
	host := args[0].String()
	dkgResultStr := args[2].String()
	metadata := args[3].String()
	authData := args[4].String()

	hexEncodedMsg := args[1].String()
	trimmedHexEncodedMsg := strings.TrimPrefix(strings.TrimSuffix(strings.ReplaceAll(hexEncodedMsg, "\"", ""), "\n"), "0x")
	message, err := hex.DecodeString(trimmedHexEncodedMsg)
	if err != nil {
		log.Println("error while hex decoding message:", err)
		return nil, err
	}

	signature, err := client.Sign(host, message, dkgResultStr, metadata, authData)
	if err != nil {
		log.Println("error while signing:", err)
		return nil, err
	}

	ret := hex.EncodeToString(signature.Signature)

	return ret, nil
}

// input : host, dkgResultStr, authData
// output : privateKey, error
func Recover(this js.Value, args []js.Value) (any, error) {
	host := args[0].String()
	dkgResultStr := args[1].String()
	metadata := args[2].String()
	authData := args[3].String()

	privateKey, err := client.Recover(host, dkgResultStr, metadata, authData)
	if err != nil {
		log.Println("error while signing:", err)
		return nil, err
	}

	return privateKey, nil
}

// input : host, json encoded transaction parameters, dkgResultStr, authData, chainId
// output : signed message, error
func SignEthTransaction(this js.Value, args []js.Value) (any, error) {
	if len(args) != 6 {
		log.Println("error when SignEthTransaction: not the correct number of arguments")
		return nil, fmt.Errorf("not the correct number of arguments")
	}

	host := args[0].String()
	dkgResultStr := args[2].String()
	metadata := args[3].String()
	authData := args[4].String()
	chainId := args[5].String()

	jsonEncodedTx := args[1].String()

	_tx, err := tx.NewEthereumTxWithJson(jsonEncodedTx, chainId)
	if err != nil {
		log.Println("error while initialising tx:", err)
		return nil, err
	}

	message := _tx.GenerateMessage()

	signature, err := client.Sign(host, message, dkgResultStr, metadata, authData)
	if err != nil {
		log.Println("error while signing:", err)
		return nil, err
	}

	signedTx, err := _tx.Sign(signature.Signature)

	return signedTx, nil
}

////////////
/// UTIL ///
////////////

// ASYNC FUNCTION : https://clavinjune.dev/en/blogs/golang-wasm-async-function/
// => solve deadlock : https://github.com/golang/go/issues/41310

type fn func(this js.Value, args []js.Value) (any, error)

var (
	jsErr     js.Value = js.Global().Get("Error")
	jsPromise js.Value = js.Global().Get("Promise")
)

// asyncFunc transforms a Go function into a promise once in Javascript
func asyncFunc(innerFunc fn) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) any {
		handler := js.FuncOf(func(_ js.Value, promFn []js.Value) any {
			resolve, reject := promFn[0], promFn[1]

			go func() {
				defer func() {
					if r := recover(); r != nil {
						reject.Invoke(jsErr.New(fmt.Sprint("panic:", r)))
					}
				}()

				res, err := innerFunc(this, args)
				if err != nil {
					reject.Invoke(jsErr.New(err.Error()))
				} else {
					resolve.Invoke(res)
				}
			}()

			return nil
		})

		return jsPromise.New(handler)
	})
}
