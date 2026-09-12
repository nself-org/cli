package server

// Purpose: the Client interface every `nself server` operation programs
// against, plus hetznerClient, its concrete implementation over the Hetzner
// Cloud API. The interface exists so cmd/commands/server_*.go can inject a
// fake in tests (mirroring internal/access.Transport) and so orchestration
// logic in provision.go/resize.go/destroy.go never depends on HTTP directly.
// Inputs: a Hetzner Cloud API token (see token.go for env-var resolution).
// Outputs: typed Server/ServerType/Image/Action/PrimaryIP values, or a
// wrapped error describing what the API call was and why it failed.
// Constraints: hetznerAPIBaseURL and hetznerHTTPClient are var indirections
// — same pattern as internal/access/hetzner_mismatch.go — so tests point at
// an httptest.Server instead of the real Hetzner Cloud API. No test in this
// package or its callers may reach the real API.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var (
	hetznerAPIBaseURL = "https://api.hetzner.cloud/v1"
	hetznerHTTPClient = &http.Client{Timeout: 30 * time.Second}
)

// Client is every Hetzner Cloud operation `nself server` needs. Introducing
// a second provider later means adding a second implementation of this
// interface, not touching provision.go/resize.go/destroy.go/list.go.
type Client interface {
	CreateServer(ctx context.Context, req ProvisionRequest) (*Server, error)
	ListServers(ctx context.Context, opts ListOptions) ([]Server, error)
	GetServer(ctx context.Context, id int64) (*Server, error)
	DeleteServer(ctx context.Context, id int64) error
	ListServerTypes(ctx context.Context) ([]ServerType, error)
	ChangeServerType(ctx context.Context, serverID int64, targetType string, upgradeDisk bool) (*Action, error)
	CreateSnapshot(ctx context.Context, serverID int64, description string) (*Image, *Action, error)
	GetImage(ctx context.Context, id int64) (*Image, error)
	GetAction(ctx context.Context, id int64) (*Action, error)
	ListPrimaryIPs(ctx context.Context, serverID int64) ([]PrimaryIP, error)
	SetPrimaryIPAutoDelete(ctx context.Context, ipID int64, autoDelete bool) error
}

// hetznerClient is the real Client, talking to api.hetzner.cloud/v1.
type hetznerClient struct {
	token string
}

// NewHetznerClient builds a Client backed by the live Hetzner Cloud API.
// token must be non-empty; resolve it with ResolveToken before calling this.
func NewHetznerClient(token string) Client {
	return &hetznerClient{token: token}
}

// apiError is Hetzner's standard error envelope, e.g.
// {"error":{"code":"invalid_input","message":"..."}}.
type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// do performs one Hetzner API call, decoding a JSON body into out (which may
// be nil for calls like DeleteServer that discard the response body).
func (c *hetznerClient) do(ctx context.Context, method, path string, body, out interface{}) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, hetznerAPIBaseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := hetznerHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("hetzner API %s %s: network error: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("hetzner API %s %s: read response: %w", method, path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var ae apiError
		if json.Unmarshal(raw, &ae) == nil && ae.Error.Message != "" {
			return fmt.Errorf("hetzner API %s %s: %d %s: %s", method, path, resp.StatusCode, ae.Error.Code, ae.Error.Message)
		}
		return fmt.Errorf("hetzner API %s %s: %d: %s", method, path, resp.StatusCode, string(raw))
	}

	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("hetzner API %s %s: decode response: %w", method, path, err)
	}
	return nil
}
