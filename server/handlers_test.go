package server

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getmeemaw/meemaw/server/database"
	"github.com/getmeemaw/meemaw/server/vault"
	"github.com/google/uuid"
)

func TestAuthorize(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(getCustomAuthHandler()))

	queries := database.New(nil)

	vault := vault.NewVault(queries)

	var config = Config{
		AuthServerUrl: "http://" + authServer.Listener.Addr().String(),
		AuthType:      "custom",
		DevMode:       true,
	}

	_server := NewServer(vault, &config, false)

	authorizeServer := httptest.NewServer(_server.Router())

	log.Println(authorizeServer)
	var err error
	var testDescription string

	var token string
	var statusCode int

	authData := "my-auth-data"
	path := "http://" + authorizeServer.Listener.Addr().String() + "/authorize"

	///////////////////
	/// TEST 1 : happy path

	testDescription = "test 1 (happy path)"
	_userId = "1d3b3b4f-c4c9-45e6-afe6-41f72e6fd71c"

	token, statusCode, err = requestToken(path, testDescription, authData, true, t)
	if err == nil {
		if statusCode == 200 {
			// verify token format
			_, err = uuid.Parse(token)
			if err != nil {
				t.Errorf("Failed "+testDescription+": token is not uuid: %s\n", token)
				return
			}

			t.Logf("Successful "+testDescription+" - got access token: %s\n", token)
		} else {
			t.Errorf("Failed "+testDescription+": status != 200: %d\n", statusCode)
		}
	} else {
		t.Errorf("Failed "+testDescription+": error when requesting token: %s\n", err)
	}

	///////////////////
	/// TEST 2 : 400, 401 or 404 from auth provider

	testDescription = "test 2 (4xx from auth provider)"
	_userId = "401"

	token, statusCode, err = requestToken(path, testDescription, authData, true, t)
	if statusCode != 401 {
		t.Errorf("Failed "+testDescription+": response is not 401 - token:%s - statusCode:%d - err:%s\n", token, statusCode, err)
	} else {
		if len(token) > 0 {
			t.Errorf("Failed "+testDescription+" - token is not empty: %s\n", token)
		} else {
			t.Logf("Successful " + testDescription + " : got 401\n")
		}
	}

	///////////////////
	/// TEST 3 : no authorization header

	testDescription = "test 3 (no authorization header)"
	_userId = "1d3b3b4f-c4c9-45e6-afe6-41f72e6fd71c"

	token, statusCode, err = requestToken(path, testDescription, authData, false, t)
	if statusCode != 401 {
		t.Errorf("Failed "+testDescription+": response is not 401 - token:%s - statusCode:%d - err:%s\n", token, statusCode, err)
	} else {
		if len(token) > 0 {
			t.Errorf("Failed "+testDescription+" - token is not empty: %s\n", token)
		} else {
			t.Logf("Successful " + testDescription + " : got 401\n")
		}
	}

	///////////////////
	/// TEST 4 : empty authorization header

	testDescription = "test 4 (empty authorization header)"
	_userId = "1d3b3b4f-c4c9-45e6-afe6-41f72e6fd71c"

	token, statusCode, err = requestToken(path, testDescription, "", true, t)
	if statusCode != 401 {
		t.Errorf("Failed "+testDescription+": response is not 401 - token:%s - statusCode:%d - err:%s\n", token, statusCode, err)
	} else {
		if len(token) > 0 {
			t.Errorf("Failed "+testDescription+" - token is not empty: %s\n", token)
		} else {
			t.Logf("Successful " + testDescription + " : got 401\n")
		}
	}

}

// tested through integration tests
// func TestDkg(t *testing.T) {}

// tested through integration tests
// func TestSign(t *testing.T) {}

func requestToken(path, testDescription, authData string, header bool, t *testing.T) (string, int, error) {
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		log.Println("error while creating new request:", err)
		t.Errorf("Failed "+testDescription+": error while creating request: %s\n", err)
		return "", 0, err
	}

	if header {
		req.Header.Set("Authorization", "Bearer "+authData)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("error while doing request to /authorize:", err)
		t.Errorf("Failed "+testDescription+": error while requesting /authorize: %s\n", err)
		return "", 0, err
	}

	if resp.StatusCode != 200 {
		return "", resp.StatusCode, nil
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("error while reading response body of /authorize:", err)
		t.Errorf("Failed "+testDescription+": error while reading body: %s\n", err)
		return "", resp.StatusCode, err
	}

	token := string(body)

	return token, resp.StatusCode, nil
}
