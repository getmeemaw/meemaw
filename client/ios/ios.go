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
	dkgResult, metadata, err := client.Dkg(host, authData)
	if err != nil {
		return swiftResultDkg(nil, "", err)
	}
	return swiftResultDkg(dkgResult, metadata, nil)
}

func RegisterDevice(host string, authData string) *SwiftResultString {

	dkgResult, metadata, err := client.RegisterDevice(host, authData, "ios")
	if err != nil {
		return swiftResultDkg(nil, "", err)
	}

	return swiftResultDkg(dkgResult, metadata, nil)
}

func AcceptDevice(host string, dkgResultStr string, authData string) *SwiftResultString {
	var upgradedDkgResult upgradedDkgResult
	err := json.Unmarshal([]byte(dkgResultStr), &upgradedDkgResult)
	if err != nil {
		return swiftResultString("", err)
	}

	err = client.AcceptDevice(host, upgradedDkgResult.DkgResultStr, upgradedDkgResult.Metadata, authData)
	if err != nil {
		return swiftResultString("", err)
	}

	return swiftResultString("", nil)
}

func Sign(host string, message []byte, dkgResultStr string, authData string) *SwiftResultBytes {
	var upgradedDkgResult upgradedDkgResult
	err := json.Unmarshal([]byte(dkgResultStr), &upgradedDkgResult)
	if err != nil {
		return swiftResultSignature(nil, err)
	}

	signature, err := client.Sign(host, message, upgradedDkgResult.DkgResultStr, upgradedDkgResult.Metadata, authData)
	if err != nil {
		return swiftResultSignature(nil, err)
	}
	return swiftResultSignature(signature, nil)
}

func Recover(host string, dkgResultStr string, authData string) *SwiftResultString {
	var upgradedDkgResult upgradedDkgResult
	err := json.Unmarshal([]byte(dkgResultStr), &upgradedDkgResult)
	if err != nil {
		return swiftResultString("", err)
	}

	privateKey, err := client.Recover(host, upgradedDkgResult.DkgResultStr, upgradedDkgResult.Metadata, authData)
	if err != nil {
		return swiftResultString("", err)
	}
	return swiftResultString(privateKey, nil)
}

type upgradedDkgResult struct {
	DkgResultStr string
	Metadata     string
}

func swiftResultDkg(dkgResult *tss.DkgResult, metadata string, err error) *SwiftResultString {
	if err != nil {
		return &SwiftResultString{
			Successful: false,
			Result:     "",
			Error:      err.Error(),
		}
	} else {
		dkgResultStr, err := json.Marshal(dkgResult)
		if err != nil {
			return &SwiftResultString{
				Successful: false,
				Result:     "",
				Error:      err.Error(),
			}
		}

		res := upgradedDkgResult{
			DkgResultStr: string(dkgResultStr),
			Metadata:     metadata,
		}

		ret, err := json.Marshal(res)
		if err != nil {
			return &SwiftResultString{
				Successful: false,
				Result:     "",
				Error:      err.Error(),
			}
		}

		return &SwiftResultString{
			Successful: true,
			Result:     string(ret),
			Error:      "",
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
