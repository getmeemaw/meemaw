package tsslib

import (
	"encoding/json"

	"github.com/getmeemaw/meemaw/client"
	"github.com/getmeemaw/meemaw/utils/tss"
)

////////

// compile :
// gomobile bind -target ios,iossimulator,macos
// 		OR : gomobile bind -target ios,iossimulator,macos -o ./MeemawSDK/Sources/Tsslib.xcframework
// gomobile bind -target android

////////

func Identify(host string, authData string) *SwiftResultString {
	userId, err := client.Identify(host, authData)
	return swiftResultString(userId, err)
}

func Dkg(host string, authData string) *SwiftResultString {
	dkgResult, _, err := client.Dkg(host, authData) // update to return and store metadata
	if err != nil {
		swiftResultDkg(nil, err)
	}
	return swiftResultDkg(dkgResult, nil)
}

func Sign(host string, message []byte, dkgResultStr string, authData string) *SwiftResultBytes {
	signature, err := client.Sign(host, message, dkgResultStr, "", authData)
	if err != nil {
		swiftResultSignature(nil, err)
	}
	return swiftResultSignature(signature, nil)
}

func swiftResultDkg(res *tss.DkgResult, err error) *SwiftResultString {
	if err != nil {
		return &SwiftResultString{
			Successful: false,
			Result:     "",
			Error:      err.Error(),
		}
	} else {
		ret, err := json.Marshal(res)
		if err != nil {
			return &SwiftResultString{
				Successful: false,
				Result:     "",
				Error:      err.Error(),
			}
		} else {
			return &SwiftResultString{
				Successful: true,
				Result:     string(ret),
				Error:      "",
			}
		}
	}
}

func swiftResultSignature(sig *tss.Signature, err error) *SwiftResultBytes {
	if err != nil {
		return &SwiftResultBytes{
			Successful: false,
			Result:     nil,
			Error:      err.Error(),
		}
	} else {
		return &SwiftResultBytes{
			Successful: true,
			Result:     sig.Signature,
			Error:      "",
		}
	}
}

func swiftResultString(ret string, err error) *SwiftResultString {
	if err != nil {
		return &SwiftResultString{
			Successful: false,
			Result:     "",
			Error:      err.Error(),
		}
	}
	return &SwiftResultString{
		Successful: true,
		Result:     ret,
		Error:      "",
	}
}

type SwiftResultString struct {
	Successful bool
	Result     string // json
	Error      string
}

type SwiftResultBytes struct {
	Successful bool
	Result     []byte
	Error      string
}
