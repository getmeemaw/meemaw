package server

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/getmeemaw/meemaw/server/database"
	"github.com/getmeemaw/meemaw/server/vault"
	"github.com/getmeemaw/meemaw/utils/types"
)

var _userId string

func TestSupabase(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(getCustomAuthHandler()))

	queries := database.New(nil)

	vault := vault.New(queries)

	var config = Config{
		SupabaseUrl: "http://" + authServer.Listener.Addr().String(),
		AuthType:    "supabase",
	}

	_server := NewServer(vault, &config, false)

	var testDescription string
	var userId string
	var err error

	_userId = "1d3b3b4f-c4c9-45e6-afe6-41f72e6fd71c"

	///////////////////
	/// TEST 1 : happy path (with correct userId format)

	testDescription = "test 1 (happy path)"

	_userId = `{
		"id": "1d3b3b4f-c4c9-45e6-afe6-41f72e6fd71c",
		"aud": "authenticated",
		"role": "authenticated",
		"email": "super_email@gmail.com",
		"email_confirmed_at": "2023-01-22T13:59:17.584048Z",
		"phone": "",
		"confirmation_sent_at": "2023-01-22T13:58:55.375979Z",
		"confirmed_at": "2023-01-22T13:59:17.584048Z",
		"last_sign_in_at": "2023-09-19T04:24:30.886588Z",
		"app_metadata": {
			"provider": "email",
			"providers": [
				"email"
			]
		},
		"user_metadata": {},
		"identities": [
			{
				"id": "1d3b3b4f-c4c9-45e6-afe6-41f72e6fd71c",
				"user_id": "1d3b3b4f-c4c9-45e6-afe6-41f72e6fd71c",
				"identity_data": {
					"email": "super_email@gmail.com",
					"sub": "1d3b3b4f-c4c9-45e6-afe6-41f72e6fd71c"
				},
				"provider": "email",
				"last_sign_in_at": "2023-01-22T13:58:55.374456Z",
				"created_at": "2023-01-22T13:58:55.374492Z",
				"updated_at": "2023-01-22T13:58:55.374492Z"
			}
		],
		"created_at": "2023-01-22T13:58:55.371763Z",
		"updated_at": "2023-09-19T04:24:30.88819Z"
	}`

	userId, err = _server.Supabase(_server._config.SupabaseUrl, _server._config.SupabaseApiKey, "my-token")
	if err != nil {
		t.Errorf("Failed "+testDescription+": expected userId, got error: %s\n", err)
	} else {
		t.Logf("Successful "+testDescription+" - got userId: %s\n", userId)
	}

	///////////////////
	/// TEST 2 : wrong userId format returned

	testDescription = "test 2 (wrong userId format returned)"

	_userId = `{
		"id": "bad-user-id",
		"aud": "authenticated",
		"role": "authenticated",
		"email": "super_email@gmail.com",
		"email_confirmed_at": "2023-01-22T13:59:17.584048Z",
		"phone": "",
		"confirmation_sent_at": "2023-01-22T13:58:55.375979Z",
		"confirmed_at": "2023-01-22T13:59:17.584048Z",
		"last_sign_in_at": "2023-09-19T04:24:30.886588Z",
		"app_metadata": {
			"provider": "email",
			"providers": [
				"email"
			]
		},
		"user_metadata": {},
		"identities": [
			{
				"id": "bad-user-id",
				"user_id": "bad-user-id",
				"identity_data": {
					"email": "super_email@gmail.com",
					"sub": "bad-user-id"
				},
				"provider": "email",
				"last_sign_in_at": "2023-01-22T13:58:55.374456Z",
				"created_at": "2023-01-22T13:58:55.374492Z",
				"updated_at": "2023-01-22T13:58:55.374492Z"
			}
		],
		"created_at": "2023-01-22T13:58:55.371763Z",
		"updated_at": "2023-09-19T04:24:30.88819Z"
	}`

	userId, err = _server.Supabase(_server._config.SupabaseUrl, _server._config.SupabaseApiKey, "my-token")
	types.ProcessShouldError(testDescription, err, &types.ErrBadRequest{}, userId, t) // should be something else than bad request ?

	///////////////////
	/// TEST 3 : wrong response format from supabase (despite status 200)

	testDescription = "test 3 (wrong response format from supabase)"

	_userId = "my-user"

	userId, err = _server.Supabase(_server._config.SupabaseUrl, _server._config.SupabaseApiKey, "my-token")
	types.ProcessShouldError(testDescription, err, &types.ErrBadRequest{}, userId, t) // should be different kind of error ??

	///////////////////
	/// COMMON TESTS
	fn := func(token string) (string, error) {
		return _server.Supabase(_server._config.SupabaseUrl, _server._config.SupabaseApiKey, token)
	}

	commonTests(fn, t)
}

func TestCustomAuth(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(getCustomAuthHandler()))

	queries := database.New(nil)

	vault := vault.New(queries)

	var config = Config{
		AuthServerUrl: "http://" + authServer.Listener.Addr().String(),
		AuthType:      "custom",
	}

	_server := NewServer(vault, &config, false)

	var testDescription string
	var userId string
	var err error

	///////////////////
	/// TEST 1 : happy path

	testDescription = "test 1 (happy path)"

	_userId = "my-user-id"

	userId, err = _server.CustomAuth(_server._config.AuthServerUrl, "my-token")
	if err != nil {
		t.Errorf("Failed "+testDescription+": expected userId, got error: %s\n", err)
	} else {
		if userId == _userId {
			t.Logf("Successful "+testDescription+" - expected %s got %s\n", _userId, userId)
		} else {
			t.Errorf("Failed "+testDescription+": expected %s, got %s\n", _userId, userId)
		}
	}

	///////////////////
	/// COMMON TESTS

	fn := func(token string) (string, error) {
		return _server.CustomAuth(_server._config.AuthServerUrl, token)
	}

	commonTests(fn, t)
}

func commonTests(authFn func(string) (string, error), t *testing.T) {
	var testDescription string
	var userId string
	var err error

	///////////////////
	/// COMMON TEST 1 : empty jwt

	testDescription = "common test 1 (empty jwt)"

	userId, err = authFn("")
	types.ProcessShouldError(testDescription, err, &types.ErrBadRequest{}, userId, t)

	///////////////////
	/// COMMON TEST 2 : received response other than 200 from auth

	testDescription = "common test 2 (received response other than 200 from auth)"
	failed := false

	for _, statusCode := range []int{201, 202, 203, 204, 205, 206, 207, 208, 226, 300, 301, 302, 303, 304, 305, 306, 307, 308, 400, 401, 402, 403, 404, 405, 406, 407, 408, 409, 410, 411, 412, 413, 414, 415, 416, 417, 418, 421, 422, 423, 424, 425, 426, 428, 429, 431, 451, 500, 501, 502, 503, 504, 505, 506, 507, 508, 510, 511} {
		_userId = strconv.Itoa(statusCode)
		_, err = authFn("my-token")
		if err == nil {
			t.Errorf("Failed " + testDescription + ": expected error, got nil\n")
			failed = true
			break
		}
	}
	if !failed {
		t.Logf("Successful " + testDescription + " - expected error, got errors")
	}

	///////////////////
	/// COMMON TEST 3 : received 400

	testDescription = "common test 3 (received 400)"

	_userId = "400"

	userId, err = authFn("my-token")
	types.ProcessShouldError(testDescription, err, &types.ErrBadRequest{}, userId, t)

	///////////////////
	/// COMMON TEST 4 : received 401

	testDescription = "common test 4 (received 401)"

	_userId = "401"

	userId, err = authFn("my-token")
	types.ProcessShouldError(testDescription, err, &types.ErrUnauthorized{}, userId, t)

	///////////////////
	/// COMMON TEST 5 : received 404

	testDescription = "common test 5 (received 404)"

	_userId = "404"

	userId, err = authFn("my-token")
	types.ProcessShouldError(testDescription, err, &types.ErrNotFound{}, userId, t)
}

func getCustomAuthHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		statusToReturn, err := strconv.Atoi(_userId)
		if err == nil {
			http.Error(w, "error "+strconv.Itoa(statusToReturn), statusToReturn)
		} else {
			w.Write([]byte(_userId))
		}
	}
}
