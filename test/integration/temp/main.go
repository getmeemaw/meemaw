package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/getmeemaw/meemaw/client"
	"github.com/getmeemaw/meemaw/utils/tss"
)

func main() {
	log.Println("test")

	const host = "http://localhost:8421"
	const authData = "eyJhbGciOiJIUzI1NiIsImtpZCI6ImNEdE1CREFQNlphcm15QU8iLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL3NnZHh4d2FsdGNkaW1pdWlrZm1uLnN1cGFiYXNlLmNvL2F1dGgvdjEiLCJzdWIiOiIxZDNiM2I0Zi1jNGM5LTQ1ZTYtYWZlNi00MWY3MmU2ZmQ3MWMiLCJhdWQiOiJhdXRoZW50aWNhdGVkIiwiZXhwIjoxNzIxMzk4OTY5LCJpYXQiOjE3MjEzOTUzNjksImVtYWlsIjoibWFyY2VhdWxlY29tdGVAZ21haWwuY29tIiwicGhvbmUiOiIiLCJhcHBfbWV0YWRhdGEiOnsicHJvdmlkZXIiOiJlbWFpbCIsInByb3ZpZGVycyI6WyJlbWFpbCJdfSwidXNlcl9tZXRhZGF0YSI6e30sInJvbGUiOiJhdXRoZW50aWNhdGVkIiwiYWFsIjoiYWFsMSIsImFtciI6W3sibWV0aG9kIjoicGFzc3dvcmQiLCJ0aW1lc3RhbXAiOjE3MTk5MzUyMzR9XSwic2Vzc2lvbl9pZCI6ImFiYmQ2NTAzLTg3ZmItNGI4OC04MDRhLTI2MjViMGU4OTk0YiIsImlzX2Fub255bW91cyI6ZmFsc2V9.kDKxXKBTT2ewdaNyR-F7hKL9loYcnS2DkPAB5NrLwd0"
	// const device = "my-super-new-device"

	log.Println("Starting DKG")

	// Generate wallet first client + server
	dkgResultFirstClient, metadataFirstClient, err := client.Dkg(host, authData)
	if err != nil {
		log.Println("Error client.Dkg:", err)
		panic(err)
	}

	log.Println("DKG done, starting backup")

	// Backup
	dkgResultBytes, err := json.Marshal(dkgResultFirstClient)
	if err != nil {
		log.Println("error while marshaling dkgresult json:", err)
		panic(err)
	}

	dkgResultStr := string(dkgResultBytes)

	backup, err := client.Backup(host, dkgResultStr, metadataFirstClient, authData)
	if err != nil {
		log.Println("Error client.Backup:", err)
		panic(err)
	}

	log.Println("")
	log.Println("dkgResultFirstClient:", dkgResultFirstClient)
	log.Println("metadataFirstClient:", metadataFirstClient)

	log.Println("")
	log.Println("backup:", backup)

	// // Multi-device
	// dkgResultSecondClient, metadataSecondClient, err := AddDevice(host, authData, device, dkgResultFirstClient, metadataFirstClient)
	// if err != nil {
	// 	log.Println("Error client.Dkg:", err)
	// 	panic(err)
	// }

	// dkgResultThirdClient, metadataThirdClient, err := AddDevice(host, authData, device, dkgResultSecondClient, metadataSecondClient)
	// if err != nil {
	// 	log.Println("Error client.Dkg:", err)
	// 	panic(err)
	// }

	// dkgResultFourthClient, metadataFourthClient, err := AddDevice(host, authData, device, dkgResultFirstClient, metadataFirstClient)
	// if err != nil {
	// 	log.Println("Error client.Dkg:", err)
	// 	panic(err)
	// }

	// log.Println("")

	// log.Printf("dkgResultFirstClient: %+v \n", dkgResultFirstClient)
	// log.Println("metadataFirstClient:", metadataFirstClient)

	// log.Println("")

	// log.Printf("dkgResultSecondClient: %+v \n", dkgResultSecondClient)
	// log.Println("metadataSecondClient:", metadataSecondClient)

	// log.Println("")

	// log.Printf("dkgResultThirdClient: %+v \n", dkgResultThirdClient)
	// log.Println("metadataThirdClient:", metadataThirdClient)

	// log.Println("")

	// log.Printf("dkgResultFourthClient: %+v \n", dkgResultFourthClient)
	// log.Println("metadataFourthClient:", metadataFourthClient)

	// log.Println("")

	// dkgResultBytes, err := json.Marshal(dkgResultThirdClient)
	// if err != nil {
	// 	log.Println("error while marshaling dkgresult json:", err)
	// 	panic(err)
	// }

	// dkgResultStr := string(dkgResultBytes)

	// signature, err := client.Sign(host, []byte("test"), dkgResultStr, metadataThirdClient, authData)
	// if err != nil {
	// 	log.Println("error while signing:", err)
	// 	panic(err)
	// }

	// log.Println("signature:", signature)
}

func AddDevice(host, authData, device string, dkgResultFirstClient *tss.DkgResult, metadataFirstClient string) (*tss.DkgResult, string, error) {
	// Add new device
	newClientDone := make(chan struct{})
	var dkgResultNewClient *tss.DkgResult
	var metadataNewClient string

	var err error

	go func() {
		log.Println("AddDevice - starting registerDevice")
		dkgResultNewClient, metadataNewClient, err = client.RegisterDevice(host, authData, device)
		if err != nil {
			log.Println("Error registerDevice:", err)
			return
		}

		newClientDone <- struct{}{}
	}()

	time.Sleep(200 * time.Millisecond)

	log.Println("AddDevice - starting acceptDevice")

	dkgResultFirstClientBytes, err := json.Marshal(dkgResultFirstClient)
	if err != nil {
		log.Println("Error marshaling dkgResult:", err)
		return nil, "", err
	}

	err = client.AcceptDevice(host, string(dkgResultFirstClientBytes), metadataFirstClient, authData)
	if err != nil {
		log.Println("Error acceptDevice:", err)
		return nil, "", err
	}

	log.Println("client.AcceptDevice done")

	<-newClientDone

	return dkgResultNewClient, metadataNewClient, nil
}
