// Package auth — HTTP client for nSelf auth server operations.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// AuthServerURL is the base URL of the auth server.
// Overridable via NSELF_AUTH_SERVER_URL for testing.
func AuthServerURL() string {
	if url := getEnv("NSELF_AUTH_SERVER_URL"); url != "" {
		return url
	}
	return "https://api.nself.org"
}

// CLIAuthBaseURL is the web UI for the CLI device auth page.
func CLIAuthBaseURL() string {
	if url := getEnv("NSELF_CLI_AUTH_URL"); url != "" {
		return url
	}
	return "https://nself.org/auth/cli"
}

// DeviceCodeResponse is returned by the device authorization endpoint.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`        // XXXX-YYYY format shown to user
	VerificationURL string `json:"verification_url"` // nself.org/auth/cli?code=...
	ExpiresInSec    int    `json:"expires_in"`
	IntervalSec     int    `json:"interval"`
}

// TokenResponse is returned when the device code is exchanged for a token.
type TokenResponse struct {
	AccessToken  string   `json:"access_token"`
	SessionToken string   `json:"session_token"`
	Email        string   `json:"email"`
	Tier         string   `json:"tier"`
	DisplayName  string   `json:"display_name,omitempty"`
	Bundles      []string `json:"bundles,omitempty"`
	ExpiresAt    string   `json:"expires_at"`
}

// AccountInfo is the response from GET /auth/session.
type AccountInfo struct {
	Authenticated bool `json:"authenticated"`
	Account       struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		DisplayName   string `json:"display_name"`
		AvatarURL     string `json:"avatar_url"`
		Tier          string `json:"tier"`
		EmailVerified bool   `json:"email_verified"`
		MFAEnabled    bool   `json:"mfa_enabled"`
	} `json:"account"`
}

// LicenseInfo represents a single license entry.
type LicenseInfo struct {
	ID            string   `json:"id"`
	Product       string   `json:"product"`
	Tier          string   `json:"tier"`
	Bundles       []string `json:"bundles"`
	SeatsIncluded int      `json:"seats_included"`
	SeatsUsed     int      `json:"seats_used"`
	IsActive      bool     `json:"is_active"`
	ActivatedAt   string   `json:"activated_at"`
	ExpiresAt     string   `json:"expires_at"`
}

// AuthAPIError is returned when the auth server returns a non-2xx response.
type AuthAPIError struct {
	Code    string `json:"error"`
	Message string `json:"message"`
	Status  int
}

func (e *AuthAPIError) Error() string {
	return fmt.Sprintf("auth server error [%s]: %s (HTTP %d)", e.Code, e.Message, e.Status)
}

// httpClient is the shared HTTP client with timeout.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// DeviceAuthorize initiates the device code flow.
// Returns a DeviceCodeResponse with the code to display to the user.
func DeviceAuthorize(ctx context.Context) (*DeviceCodeResponse, error) {
	url := fmt.Sprintf("%s/auth/device/authorize", AuthServerURL())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(`{}`))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting auth server: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, parseAPIError(resp)
	}

	var result DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing device authorization response: %w", err)
	}

	return &result, nil
}

// PollToken polls the auth server for the device code exchange result.
// Returns (nil, nil) if the user hasn't authorized yet (authorization_pending).
// Returns an error on timeout or other failure.
func PollToken(ctx context.Context, deviceCode string) (*TokenResponse, error) {
	url := fmt.Sprintf("%s/auth/device/token", AuthServerURL())

	body, _ := json.Marshal(map[string]string{"device_code": deviceCode})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("polling token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// authorization_pending → user hasn't clicked "Authorize" yet
	if resp.StatusCode == http.StatusAccepted {
		return nil, nil
	}

	if resp.StatusCode == http.StatusOK {
		var result TokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("parsing token response: %w", err)
		}
		return &result, nil
	}

	return nil, parseAPIError(resp)
}

// RefreshToken exchanges an existing session for a new access token.
func RefreshToken(ctx context.Context, accessToken string) (*TokenResponse, error) {
	url := fmt.Sprintf("%s/auth/refresh", AuthServerURL())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", fmt.Sprintf("nself_auth_token=%s", accessToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refreshing token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, parseSessionError(resp)
	}

	var result TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing refresh response: %w", err)
	}

	return &result, nil
}

// RevokeSession calls POST /auth/signout to revoke the session server-side.
func RevokeSession(ctx context.Context, accessToken string, all bool) error {
	url := fmt.Sprintf("%s/auth/signout", AuthServerURL())
	if all {
		url += "?all=true"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", fmt.Sprintf("nself_auth_token=%s", accessToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("revoking session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return parseSessionError(resp)
	}

	return nil
}

// GetSession returns the account info for the given access token.
func GetSession(ctx context.Context, accessToken string) (*AccountInfo, error) {
	url := fmt.Sprintf("%s/auth/session", AuthServerURL())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", fmt.Sprintf("nself_auth_token=%s", accessToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, parseSessionError(resp)
	}

	var result AccountInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing session response: %w", err)
	}

	return &result, nil
}

// GetLicenses returns the list of active licenses for the account.
func GetLicenses(ctx context.Context, accessToken string) ([]LicenseInfo, error) {
	url := fmt.Sprintf("%s/account/licenses", AuthServerURL())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", fmt.Sprintf("nself_auth_token=%s", accessToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting licenses: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, parseSessionError(resp)
	}

	var result struct {
		Licenses []LicenseInfo `json:"licenses"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing licenses response: %w", err)
	}

	return result.Licenses, nil
}
