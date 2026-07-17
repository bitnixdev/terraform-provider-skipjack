package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseResourceID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id      string
		org     string
		project string
		name    string
		wantErr bool
	}{
		{id: "acme/shared-ci/NPM_TOKEN", org: "acme", project: "shared-ci", name: "NPM_TOKEN"},
		{id: "acme//ORG_SECRET", org: "acme", project: "", name: "ORG_SECRET"},
		{id: "bad", wantErr: true},
		{id: "only/two", wantErr: true},
		{id: "//NAME", wantErr: true},
		{id: "org//", wantErr: true},
	}

	for _, tc := range cases {
		org, project, name, err := ParseResourceID(tc.id)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("ParseResourceID(%q) expected error", tc.id)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseResourceID(%q) unexpected error: %v", tc.id, err)
		}
		if org != tc.org || project != tc.project || name != tc.name {
			t.Fatalf("ParseResourceID(%q) = (%q,%q,%q), want (%q,%q,%q)",
				tc.id, org, project, name, tc.org, tc.project, tc.name)
		}
	}
}

func TestScopePathPrefix(t *testing.T) {
	t.Parallel()

	if got := (Scope{Org: "acme"}).PathPrefix(); got != "/v1/orgs/acme" {
		t.Fatalf("org scope path = %q", got)
	}
	if got := (Scope{Org: "acme", Project: "shared-ci"}).PathPrefix(); got != "/v1/orgs/acme/projects/shared-ci" {
		t.Fatalf("project scope path = %q", got)
	}
}

func TestNewValidatesTokenAndURL(t *testing.T) {
	t.Parallel()

	if _, err := New("https://skipjack.example.com", "", ""); err == nil {
		t.Fatal("expected empty token error")
	}
	if _, err := New("ftp://example.com", "sjk_test", ""); err == nil {
		t.Fatal("expected scheme error")
	}
	if _, err := New("https://example.com/path", "sjk_test", ""); err == nil {
		t.Fatal("expected path error")
	}
	// API keys and opaque/JWT bearer tokens are both accepted; the server decides.
	c, err := New("https://skipjack.example.com/", "sjk_test", "ua")
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "https://skipjack.example.com" {
		t.Fatalf("baseURL = %q", c.baseURL)
	}
	if _, err := New("https://skipjack.example.com", "eyJhbGciOiJSUzI1NiJ9.e30.sig", "ua"); err != nil {
		t.Fatalf("JWT token should be accepted: %v", err)
	}
}

func TestClientSecretRoundTrip(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/orgs/acme/projects/shared-ci/secrets/NPM_TOKEN", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sjk_test" {
			t.Errorf("Authorization = %q", got)
		}
		switch r.Method {
		case http.MethodPut:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["value"] != "v1" {
				t.Fatalf("value = %#v", body["value"])
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "NPM_TOKEN", "version": 1})
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "NPM_TOKEN", "value": "v1", "version": 1,
			})
		case http.MethodDelete:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/v1/orgs/acme/projects/shared-ci/secrets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"secrets": []map[string]any{
				{"name": "NPM_TOKEN", "value": "v1", "version": 1, "updatedAt": 1},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Client requires https; for tests use http server by constructing client carefully.
	// Override via internal fields after New would fail on scheme — use custom transport.
	// Instead, temporarily allow http by calling New with http URL — we allow http for local/dev.
	c, err := New(srv.URL, "sjk_test", "test")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	scope := Scope{Org: "acme", Project: "shared-ci"}

	put, err := c.PutSecret(ctx, scope, "NPM_TOKEN", "v1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if put.Version != 1 {
		t.Fatalf("version = %d", put.Version)
	}

	got, err := c.GetSecret(ctx, scope, "NPM_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != "v1" || got.Version != 1 {
		t.Fatalf("get = %#v", got)
	}

	list, err := c.ListSecrets(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "NPM_TOKEN" {
		t.Fatalf("list = %#v", list)
	}

	if err := c.DeleteSecret(ctx, scope, "NPM_TOKEN"); err != nil {
		t.Fatal(err)
	}
}

func TestClientAPIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not_found"})
	}))
	t.Cleanup(srv.Close)

	c, err := New(srv.URL, "sjk_test", "test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.GetSecret(context.Background(), Scope{Org: "acme"}, "X")
	if !IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}
