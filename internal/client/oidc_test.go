package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsAPIKeyAndJWT(t *testing.T) {
	t.Parallel()

	if !IsAPIKey("sjk_abc") {
		t.Fatal("expected API key")
	}
	if IsAPIKey("eyJhbGciOiJSUzI1NiJ9.e30.sig") {
		t.Fatal("JWT should not look like API key")
	}
	if !IsJWT("eyJhbGciOiJSUzI1NiJ9.e30.sig") {
		t.Fatal("expected JWT")
	}
	if IsJWT("sjk_abc") {
		t.Fatal("API key should not look like JWT")
	}
	if IsJWT("not.a") {
		t.Fatal("two segments is not a JWT")
	}
}

func TestNewAcceptsOIDCJWT(t *testing.T) {
	t.Parallel()

	jwt := "eyJhbGciOiJSUzI1NiJ9.e30.sig"
	c, err := New("https://skipjack.example.com", jwt, "test")
	if err != nil {
		t.Fatal(err)
	}
	if c.token != jwt {
		t.Fatalf("token = %q", c.token)
	}
}

func TestFetchGitHubActionsOIDCToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer gha-request-token" {
			t.Errorf("Authorization = %q", got)
		}
		if r.URL.Query().Get("audience") != "https://skipjack.example.com" {
			t.Errorf("audience = %q", r.URL.Query().Get("audience"))
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"value": "eyJhbGciOiJSUzI1NiJ9.payload.sig",
		})
	}))
	t.Cleanup(srv.Close)

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL+"?api-version=2.0")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "gha-request-token")

	if !GitHubActionsOIDCAvailable() {
		t.Fatal("expected GHA OIDC available")
	}

	token, err := FetchGitHubActionsOIDCToken(context.Background(), "https://skipjack.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if token != "eyJhbGciOiJSUzI1NiJ9.payload.sig" {
		t.Fatalf("token = %q", token)
	}
}

func TestFetchGitHubActionsOIDCTokenMissingEnv(t *testing.T) {
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")

	if GitHubActionsOIDCAvailable() {
		t.Fatal("expected unavailable")
	}
	_, err := FetchGitHubActionsOIDCToken(context.Background(), "https://example.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClientSendsOIDCBearer(t *testing.T) {
	t.Parallel()

	jwt := "eyJhbGciOiJSUzI1NiJ9.e30.sig"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+jwt {
			t.Errorf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "REGION", "value": "us-west-2",
		})
	}))
	t.Cleanup(srv.Close)

	c, err := New(srv.URL, jwt, "test")
	if err != nil {
		t.Fatal(err)
	}
	v, err := c.GetVariable(context.Background(), Scope{Org: "acme", Project: "infra"}, "REGION")
	if err != nil {
		t.Fatal(err)
	}
	if v.Value != "us-west-2" {
		t.Fatalf("value = %q", v.Value)
	}
}
