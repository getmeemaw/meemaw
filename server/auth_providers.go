package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/getmeemaw/meemaw/utils/types"
	"github.com/google/uuid"
)

type AuthConfig struct {
	AuthType       string
	AuthServerUrl  string
	SupabaseUrl    string
	SupabaseApiKey string
}

func (server *Server) authProviders(authConfig *AuthConfig, bearerToken string) (string, error) {
	if authConfig.AuthType == "supabase" {
		if len(authConfig.SupabaseApiKey) == 0 || len(authConfig.SupabaseUrl) == 0 {
			return "", errors.New("missing Supabase config")
		}
		return server.Supabase(authConfig.SupabaseUrl, authConfig.SupabaseApiKey, bearerToken)
	} else if authConfig.AuthType == "custom" {
		if len(authConfig.AuthServerUrl) == 0 {
			return "", errors.New("missing custom auth url")
		}
		return server.CustomAuth(authConfig.AuthServerUrl, bearerToken)
	} else {
		return "", errors.New("wrong auth type")
	}
}

type SupabaseUser struct {
	ID string `json:"id"`
}

// Supabase calls Supabase server to get the userId, based on the Supabase JWT provided
func (server *Server) Supabase(supabaseUrl, supabaseApiKey, jwt string) (string, error) {

	// Verify jwt is not empty
	if len(jwt) == 0 {
		return "", &types.ErrBadRequest{}
	}

	// Get userId from Supabase
	req, err := http.NewRequest("GET", strings.TrimSuffix(supabaseUrl, "/")+"/auth/v1/user", nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	req.Header.Set("apikey", supabaseApiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Check http code from Supabase
	if resp.StatusCode != 200 {
		log.Printf("Supabase response not 200: %+v", resp)
		if resp.StatusCode == 400 {
			return "", &types.ErrBadRequest{}
		} else if resp.StatusCode == 401 || resp.StatusCode == 403 {
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
// Calls the generic CustomAuth auth provider using the webhook provided (auth config) with agreed upon API contract
func (server *Server) CustomAuth(authServerUrl, token string) (string, error) {

	// Verify jwt is not empty
	if len(token) == 0 {
		return "", &types.ErrBadRequest{}
	}

	// Get userId from custom auth provider
	url := authServerUrl

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
