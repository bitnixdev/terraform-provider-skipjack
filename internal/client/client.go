// Package client is a thin HTTP client for Skipjack's machine-facing token API
// mounted at /v1. Auth is Authorization: Bearer <API key or OIDC JWT>.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	DefaultURL     = "https://skipjack.bitnix.dev"
	APIKeyPrefix   = "sjk_"
	defaultTimeout = 30 * time.Second
)

// Client talks to the Skipjack /v1 token API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	userAgent  string
}

// New creates a Client. baseURL should be an absolute https origin without a
// trailing slash. token is either a Skipjack API key (sjk_...) or a short-lived
// OIDC JWT for a configured workload identity.
func New(baseURL, token, userAgent string) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultURL
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, fmt.Errorf("url must use http or https, got %q", parsed.Scheme)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("url must not contain credentials, a query, or a fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, fmt.Errorf("url must be an origin (no path); got %q", parsed.Path)
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("token is required")
	}

	if userAgent == "" {
		userAgent = "terraform-provider-skipjack"
	}

	return &Client{
		baseURL: strings.TrimRight(parsed.String(), "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		userAgent: userAgent,
	}, nil
}

// IsAPIKey reports whether token looks like a Skipjack API key.
func IsAPIKey(token string) bool {
	return strings.HasPrefix(strings.TrimSpace(token), APIKeyPrefix)
}

// IsJWT reports whether token looks like a compact JWT (three base64url segments).
func IsJWT(token string) bool {
	parts := strings.Split(strings.TrimSpace(token), ".")
	return len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != ""
}

// Scope identifies an org-level or project-level token-API mount.
type Scope struct {
	Org     string
	Project string // empty means org-level
}

// PathPrefix returns /v1/orgs/:org[/projects/:proj].
func (s Scope) PathPrefix() string {
	org := url.PathEscape(s.Org)
	if s.Project == "" {
		return fmt.Sprintf("/v1/orgs/%s", org)
	}
	return fmt.Sprintf("/v1/orgs/%s/projects/%s", org, url.PathEscape(s.Project))
}

// ResourceID builds a stable Terraform id: org/project/name or org//name.
func ResourceID(org, project, name string) string {
	return fmt.Sprintf("%s/%s/%s", org, project, name)
}

// ParseResourceID parses org/project/name. Project may be empty (org//name).
func ParseResourceID(id string) (org, project, name string, err error) {
	// Split into at most 3 parts so names cannot contain "/".
	parts := strings.SplitN(id, "/", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("expected id format org/project/name or org//name, got %q", id)
	}
	org, project, name = parts[0], parts[1], parts[2]
	if org == "" || name == "" {
		return "", "", "", fmt.Errorf("org and name are required in id %q", id)
	}
	return org, project, name, nil
}

// APIError is a non-2xx response from Skipjack.
type APIError struct {
	StatusCode int
	Code       string
	Detail     string
	Body       string
}

func (e *APIError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("skipjack API error (HTTP %d, %s): %s", e.StatusCode, e.Code, e.Detail)
	}
	if e.Code != "" {
		return fmt.Sprintf("skipjack API error (HTTP %d, %s)", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("skipjack API error (HTTP %d): %s", e.StatusCode, e.Body)
}

// NotFound reports whether the error is an HTTP 404.
func (e *APIError) NotFound() bool {
	return e != nil && e.StatusCode == http.StatusNotFound
}

// Secret is a single secret returned by the token API.
type Secret struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Version   int64  `json:"version"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

// Variable is a single variable returned by the token API.
type Variable struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

type putBody struct {
	Value       string  `json:"value"`
	Description *string `json:"description,omitempty"`
}

type putSecretResponse struct {
	Name    string `json:"name"`
	Version int64  `json:"version"`
}

type putVariableResponse struct {
	Name string `json:"name"`
}

type listSecretsResponse struct {
	Secrets []Secret `json:"secrets"`
}

type listVariablesResponse struct {
	Variables []Variable `json:"variables"`
}

// GetSecret reads one secret by name.
func (c *Client) GetSecret(ctx context.Context, scope Scope, name string) (*Secret, error) {
	path := fmt.Sprintf("%s/secrets/%s", scope.PathPrefix(), url.PathEscape(name))
	var out Secret
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListSecrets lists (and decrypts) all secrets in scope.
func (c *Client) ListSecrets(ctx context.Context, scope Scope) ([]Secret, error) {
	path := scope.PathPrefix() + "/secrets"
	var out listSecretsResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Secrets, nil
}

// PutSecret creates or updates a secret. Returns the new version.
func (c *Client) PutSecret(ctx context.Context, scope Scope, name, value string, description *string) (*putSecretResponse, error) {
	path := fmt.Sprintf("%s/secrets/%s", scope.PathPrefix(), url.PathEscape(name))
	body := putBody{Value: value, Description: description}
	var out putSecretResponse
	if err := c.do(ctx, http.MethodPut, path, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteSecret deletes a secret by name.
func (c *Client) DeleteSecret(ctx context.Context, scope Scope, name string) error {
	path := fmt.Sprintf("%s/secrets/%s", scope.PathPrefix(), url.PathEscape(name))
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// GetVariable reads one variable by name.
func (c *Client) GetVariable(ctx context.Context, scope Scope, name string) (*Variable, error) {
	path := fmt.Sprintf("%s/variables/%s", scope.PathPrefix(), url.PathEscape(name))
	var out Variable
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListVariables lists all variables in scope.
func (c *Client) ListVariables(ctx context.Context, scope Scope) ([]Variable, error) {
	path := scope.PathPrefix() + "/variables"
	var out listVariablesResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Variables, nil
}

// PutVariable creates or updates a variable.
func (c *Client) PutVariable(ctx context.Context, scope Scope, name, value string, description *string) error {
	path := fmt.Sprintf("%s/variables/%s", scope.PathPrefix(), url.PathEscape(name))
	body := putBody{Value: value, Description: description}
	var out putVariableResponse
	return c.do(ctx, http.MethodPut, path, body, &out)
}

// DeleteVariable deletes a variable by name.
func (c *Client) DeleteVariable(ctx context.Context, scope Scope, name string) error {
	path := fmt.Sprintf("%s/variables/%s", scope.PathPrefix(), url.PathEscape(name))
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseAPIError(resp.StatusCode, respBody)
	}

	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func parseAPIError(status int, body []byte) error {
	apiErr := &APIError{
		StatusCode: status,
		Body:       string(body),
		Code:       "unknown_error",
	}
	var payload struct {
		Error  string `json:"error"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if payload.Error != "" {
			apiErr.Code = payload.Error
		}
		apiErr.Detail = payload.Detail
	}
	return apiErr
}

// IsNotFound reports whether err is an API 404.
func IsNotFound(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.NotFound()
	}
	return false
}

// GitHubActionsOIDCAvailable reports whether the process is running in GitHub
// Actions with permission to mint an OIDC id-token.
func GitHubActionsOIDCAvailable() bool {
	return os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL") != "" &&
		os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN") != ""
}

// FetchGitHubActionsOIDCToken requests a short-lived JWT from the GitHub
// Actions OIDC endpoint. audience should match a configured Skipjack OIDC
// workload identity (often the Skipjack deployment URL).
func FetchGitHubActionsOIDCToken(ctx context.Context, audience string) (string, error) {
	reqURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	reqToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if reqURL == "" || reqToken == "" {
		return "", fmt.Errorf("GitHub Actions OIDC is not available (missing ACTIONS_ID_TOKEN_REQUEST_URL/TOKEN)")
	}

	audience = strings.TrimSpace(audience)
	if audience == "" {
		return "", fmt.Errorf("oidc audience is required")
	}

	u, err := url.Parse(reqURL)
	if err != nil {
		return "", fmt.Errorf("invalid ACTIONS_ID_TOKEN_REQUEST_URL: %w", err)
	}
	q := u.Query()
	q.Set("audience", audience)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build OIDC request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+reqToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "terraform-provider-skipjack")

	httpClient := &http.Client{Timeout: defaultTimeout}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request GitHub Actions OIDC token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read OIDC response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GitHub Actions OIDC token request failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var payload struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode OIDC response: %w", err)
	}
	if payload.Value == "" {
		return "", fmt.Errorf("GitHub Actions OIDC response missing value")
	}
	return payload.Value, nil
}
