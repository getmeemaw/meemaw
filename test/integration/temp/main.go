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
	const authData = "eyJhbGciOiJIUzI1NiIsImtpZCI6ImNEdE1CREFQNlphcm15QU8iLCJ0eXAiOiJKV1QifQ.eyJhdWQiOiJhdXRoZW50aWNhdGVkIiwiZXhwIjoxNzIwNzE2MjQ4LCJpYXQiOjE3MjA3MTI2NDgsImlzcyI6Imh0dHBzOi8vc2dkeHh3YWx0Y2RpbWl1aWtmbW4uc3VwYWJhc2UuY28vYXV0aC92MSIsInN1YiI6IjFkM2IzYjRmLWM0YzktNDVlNi1hZmU2LTQxZjcyZTZmZDcxYyIsImVtYWlsIjoibWFyY2VhdWxlY29tdGVAZ21haWwuY29tIiwicGhvbmUiOiIiLCJhcHBfbWV0YWRhdGEiOnsicHJvdmlkZXIiOiJlbWFpbCIsInByb3ZpZGVycyI6WyJlbWFpbCJdfSwidXNlcl9tZXRhZGF0YSI6e30sInJvbGUiOiJhdXRoZW50aWNhdGVkIiwiYWFsIjoiYWFsMSIsImFtciI6W3sibWV0aG9kIjoicGFzc3dvcmQiLCJ0aW1lc3RhbXAiOjE3MTk5MzUyMzR9XSwic2Vzc2lvbl9pZCI6ImFiYmQ2NTAzLTg3ZmItNGI4OC04MDRhLTI2MjViMGU4OTk0YiIsImlzX2Fub255bW91cyI6ZmFsc2V9.5C-mvxtkt9H_NA8-mPdkI0zJj1okkAa8k1BK-lq1Zdc"
	const device = "my-super-new-device"

	log.Println("Starting DKG")

	// Generate wallet old client + server
	dkgResultOldClient, metadataOldClient, err := client.Dkg(host, authData)
	if err != nil {
		log.Println("Error client.Dkg:", err)
		return
	}

	dkgResultOldClientBytes, err := json.Marshal(dkgResultOldClient)
	if err != nil {
		log.Println("Error marshaling dkgResult:", err)
		return
	}

	// Add new device
	newClientDone := make(chan struct{})
	var dkgResultNewClient *tss.DkgResult
	var metadataNewClient string

	go func() {
		log.Println("Starting register device")

		dkgResultNewClient, metadataNewClient, err = client.RegisterDevice(host, authData, device)
		if err != nil {
			log.Println("Error registerDevice:", err)
			return
		}

		newClientDone <- struct{}{}
	}()

	time.Sleep(200 * time.Millisecond)

	log.Println("starting accepting device")

	err = client.AcceptDevice(host, string(dkgResultOldClientBytes), metadataOldClient, authData)
	if err != nil {
		log.Println("Error acceptDevice:", err)
		return
	}

	log.Println("client.AcceptDevice done")

	<-newClientDone

	log.Println("")

	log.Printf("dkgResultOldClient: %+v \n", dkgResultOldClient)
	log.Println("metadataOldClient:", metadataOldClient)

	log.Println("")

	// COMPARE
	log.Printf("dkgResultNewClient: %+v \n", dkgResultNewClient)
	log.Println("metadataNewClient:", metadataNewClient)

	log.Println("")

	log.Println("share old client:", dkgResultOldClient.Share)
	log.Println("share new client:", dkgResultNewClient.Share)
}
