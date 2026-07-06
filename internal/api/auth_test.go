package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestAuthSessionsAndAPIKeyRegen(t *testing.T) {
	a := newTestAPI(t, nil)

	// Auth is disabled until credentials are set.
	var status struct {
		AuthEnabled   bool `json:"authEnabled"`
		Authenticated bool `json:"authenticated"`
	}
	a.want(a.call("GET", "/api/v1/auth/status", nil, &status), http.StatusOK)
	if status.AuthEnabled {
		t.Fatal("auth enabled on a fresh config")
	}

	// Logging in while disabled is a 400; short passwords are rejected.
	a.want(a.call("POST", "/api/v1/auth/login",
		map[string]string{"username": "dan", "password": "whatever1"}, nil), http.StatusBadRequest)
	a.want(a.call("PUT", "/api/v1/auth/credentials",
		map[string]string{"username": "dan", "password": "short"}, nil), http.StatusBadRequest)

	// Enable the account (API-key authenticated).
	a.want(a.call("PUT", "/api/v1/auth/credentials",
		map[string]string{"username": "dan", "password": "secret-pass-1"}, nil), http.StatusOK)
	a.want(a.call("GET", "/api/v1/auth/status", nil, &status), http.StatusOK)
	if !status.AuthEnabled {
		t.Fatal("auth still disabled after setting credentials")
	}

	// Wrong password is rejected; right one issues a session cookie.
	post := func(body string) *http.Response {
		resp, err := http.Post(a.srv.URL+"/api/v1/auth/login", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	if resp := post(`{"username":"dan","password":"wrong-pass-1"}`); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: status %d, want 401", resp.StatusCode)
	}
	resp := post(`{"username":"dan","password":"secret-pass-1"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d, want 200", resp.StatusCode)
	}
	var session *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			session = c
		}
	}
	if session == nil || session.Value == "" {
		t.Fatal("login response set no session cookie")
	}

	// The session alone (no API key) authenticates API requests.
	withCookie := func(c *http.Cookie) int {
		req, _ := http.NewRequest("GET", a.srv.URL+"/api/v1/author", nil)
		if c != nil {
			req.AddCookie(c)
		}
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r.StatusCode
	}
	if got := withCookie(session); got != http.StatusOK {
		t.Fatalf("session request: status %d, want 200", got)
	}
	if got := withCookie(nil); got != http.StatusUnauthorized {
		t.Fatalf("bare request: status %d, want 401", got)
	}

	// Logout revokes the session.
	req, _ := http.NewRequest("POST", a.srv.URL+"/api/v1/auth/logout", nil)
	req.AddCookie(session)
	lr, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	lr.Body.Close()
	if got := withCookie(session); got != http.StatusUnauthorized {
		t.Fatalf("session after logout: status %d, want 401", got)
	}

	// Regenerate the API key: old key dies, new key works.
	oldKey := a.apiKey
	var regen struct {
		APIKey string `json:"apiKey"`
	}
	a.want(a.call("POST", "/api/v1/auth/apikey/regenerate", nil, &regen), http.StatusOK)
	if regen.APIKey == "" || regen.APIKey == oldKey {
		t.Fatalf("regenerated key %q should be fresh", regen.APIKey)
	}
	if r := a.call("GET", "/api/v1/author", nil, nil); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old API key: status %d, want 401", r.StatusCode)
	}
	a.apiKey = regen.APIKey
	a.want(a.call("GET", "/api/v1/author", nil, nil), http.StatusOK)
}
