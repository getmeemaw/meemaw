package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/getmeemaw/meemaw/utils/types"
	"github.com/google/uuid"
)

func (server *Server) authProviders() map[string]func(string) (string, error) {
	return map[string]func(string) (string, error){
		"supabase": server.Supabase,
		"custom":   server.CustomAuth,
	}
}

type SupabaseUser struct {
	ID string `json:"id"`
}

// Supabase calls Supabase server (address and API key in server config) to get the userId, based on the Supabase JWT provided
func (server *Server) Supabase(jwt string) (string, error) {

	// Verify jwt is not empty
	if len(jwt) == 0 {
		return "", &types.ErrBadRequest{}
	}

	// Get userId from Supabase
	req, err := http.NewRequest("GET", strings.TrimSuffix(server._config.SupabaseUrl, "/")+"/auth/v1/user", nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	req.Header.Set("apikey", server._config.SupabaseApiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Check http code from Supabase
	if resp.StatusCode != 200 {
		if resp.StatusCode == 400 {
			return "", &types.ErrBadRequest{}
		} else if resp.StatusCode == 401 {
			return "", &types.ErrUnauthorized{}
		} else if resp.StatusCode == 404 {
			return "", &types.ErrNotFound{}
		} else {
			return "", fmt.Errorf("unknown error from Supabase")
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var user SupabaseUser
	err = json.Unmarshal(body, &user)
	if err != nil {
		log.Println("Error unmarshaling Supabase response:", string(body))
		return "", &types.ErrBadRequest{}
	}

	if user == (SupabaseUser{}) {
		log.Println("Error getting user id from Supabase response")
		return "", &types.ErrBadRequest{}
	}

	// Verify that the userId fits the uuid format (used by Supabase)
	_, err = uuid.Parse(user.ID)
	if err != nil {
		return "", &types.ErrBadRequest{}
	}

	return user.ID, nil

}

// CustomAuth gets the userId from a generic CustomAuth auth provider, based on a token representing a session or connexion
// Calls the generic CustomAuth auth provider using the webhook provided (server config) with pre-established API contract
func (server *Server) CustomAuth(token string) (string, error) {

	// Verify jwt is not empty
	if len(token) == 0 {
		return "", &types.ErrBadRequest{}
	}

	// Get userId from custom auth provider
	url := server._config.AuthServerUrl

	type AuthData struct {
		Token string `json:"token"`
	}

	auth := AuthData{
		Token: token,
	}
	jsonReq, err := json.Marshal(auth)
	if err != nil {
		log.Printf("Error encoding payload for custom auth webhook: %s", err)
		return "", err
	}

	// Send POST request to custom auth webhook
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonReq))
	if err != nil {
		log.Printf("Error sending request to custom auth webhook (with url: %s): %s", url, err)
		return "", err
	}
	defer resp.Body.Close()

	// Check http code from webhook
	if resp.StatusCode != 200 {
		if resp.StatusCode == 400 {
			return "", &types.ErrBadRequest{}
		} else if resp.StatusCode == 401 {
			return "", &types.ErrUnauthorized{}
		} else if resp.StatusCode == 404 {
			return "", &types.ErrNotFound{}
		} else {
			return "", fmt.Errorf("unknown error from custom auth")
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body from custom auth webhook: %s", err)
		return "", err
	}

	userId := string(body)

	return userId, nil
}
