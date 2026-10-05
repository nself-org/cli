// Package license — validate.go implements the full license validation flow
// with ping.nself.org and grace state machine integration.
//
// Flow: try remote validation -> update cache -> fall back to cached result
// with grace state evaluation on network failure.
package license

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DefaultPingURL is the production license validation endpoint.
const DefaultPingURL = "https://ping.nself.org"

// DefaultCheckInterval is the default time between license checks (6 hours).
const DefaultCheckInterval = 6 * time.Hour

// ValidationResult holds the outcome of a full license validation.
type ValidationResult struct {
	Valid        bool
	Tier         string
	Plugins      []string
	ExpiresAt    time.Time
	GraceState   GraceState
	Message      string
	FromCache    bool
	WriteAllowed bool
}

// ValidateResponse is the JSON body returned by /license/validate on HTTP 200.
type ValidateResponse struct {
	Valid     bool     `json:"valid"`
	Reason    string   `json:"reason,omitempty"`
	Tier      string   `json:"tier"`
	Plugins   []string `json:"plugins"`
	ExpiresAt string   `json:"expires_at,omitempty"`
	Signature string   `json:"signature,omitempty"`
	KeyID     int      `json:"key_id,omitempty"`

	// What ping_api actually sends (routes/license-validate.ts); Plugins,
	// Signature and KeyID above are never sent. Use AllowedPlugins.
	PluginsAllowed []string `json:"plugins_allowed,omitempty"`
	JWT            string   `json:"jwt,omitempty"`
	JWTKid         string   `json:"jwt_kid,omitempty"`
	JWTExpiresAt   int64    `json:"jwt_expires_at,omitempty"`

	// RawBody and BodySig are the exact response bytes and the
	// X-NSelf-License-Sig header, set by validateRemote (never from JSON).
	RawBody string `json:"-"`
	BodySig string `json:"-"`
}

// PingURL returns the configured ping API URL.
func PingURL() string {
	if u := os.Getenv("LICENSE_PING_URL"); u != "" {
		return u
	}
	if u := os.Getenv("NSELF_PING_API_URL"); u != "" {
		return u
	}
	return DefaultPingURL
}

// ValidateFull performs the complete license validation flow:
// 1. Try remote validation against ping.nself.org
// 2. On success: update cache, return valid
// 3. On network failure: fall back to cache with grace state evaluation
// 4. On invalid response: mark cache as invalid, return error
func ValidateFull(ctx context.Context, key string) (*ValidationResult, error) {
	pingURL := PingURL()

	// Try remote validation.
	resp, err := validateRemote(ctx, key, pingURL)
	if err == nil {
		// Remote succeeded. Update cache.
		entry := responseToCache(key, resp)
		if writeErr := WriteCache(entry); writeErr != nil {
			// Cache write failure is non-fatal.
			fmt.Fprintf(os.Stderr, "warning: could not update license cache: %v\n", writeErr)
		}
		expiresAt := time.Time{}
		if resp.ExpiresAt != "" {
			expiresAt, _ = time.Parse(time.RFC3339, resp.ExpiresAt)
		}
		if !resp.Valid {
			return &ValidationResult{
				Valid:   false,
				Message: resp.Reason,
			}, nil
		}
		return &ValidationResult{
			Valid:        true,
			Tier:         resp.Tier,
			Plugins:      pluginsFor(resp),
			ExpiresAt:    expiresAt,
			GraceState:   GraceValid,
			WriteAllowed: true,
			FromCache:    false,
		}, nil
	}

	// Remote failed. Fall back to cache.
	entry, cacheErr := ReadCache()
	if cacheErr != nil || entry == nil {
		return &ValidationResult{
			Valid:   false,
			Message: fmt.Sprintf("License validation failed (network: %v) and no valid cache exists.", err),
		}, nil
	}

	// Verify the cached entry is for this key.
	if entry.KeyHash != HashKey(key) {
		return &ValidationResult{
			Valid:   false,
			Message: "License cache does not match current key. Connect to the internet to validate.",
		}, nil
	}

	if !cacheSignatureOK(entry) {
		return &ValidationResult{
			Valid:   false,
			Message: "License cache is not signed by ping (unsigned, altered or for another key). Connect to the internet to validate.",
		}, nil
	}

	// Evaluate grace state.
	grace := graceStateFor(entry)
	return &ValidationResult{
		Valid:        grace.CanProceed,
		Tier:         entry.Tier,
		Plugins:      entry.PluginsAllowed,
		ExpiresAt:    time.Unix(entry.ExpiresAt, 0),
		GraceState:   grace.State,
		Message:      grace.Message,
		WriteAllowed: grace.WriteAllowed,
		FromCache:    true,
	}, nil
}

// RefreshCache forces a remote validation and updates the cache.
// Returns the validation result or an error if the remote call fails.
func RefreshCache(ctx context.Context, key string) (*ValidationResult, error) {
	pingURL := PingURL()
	resp, err := validateRemote(ctx, key, pingURL)
	if err != nil {
		return nil, fmt.Errorf("license refresh failed: %w", err)
	}
	if !resp.Valid {
		return &ValidationResult{
			Valid:   false,
			Message: resp.Reason,
		}, nil
	}
	entry := responseToCache(key, resp)
	if writeErr := WriteCache(entry); writeErr != nil {
		return nil, fmt.Errorf("updating license cache: %w", writeErr)
	}
	expiresAt := time.Time{}
	if resp.ExpiresAt != "" {
		expiresAt, _ = time.Parse(time.RFC3339, resp.ExpiresAt)
	}
	return &ValidationResult{
		Valid:        true,
		Tier:         resp.Tier,
		Plugins:      pluginsFor(resp),
		ExpiresAt:    expiresAt,
		GraceState:   GraceValid,
		WriteAllowed: true,
	}, nil
}

// ExportCache reads the current cache and returns it as JSON bytes suitable
// for air-gap transfer.
func ExportCache() ([]byte, error) {
	entry, err := ReadCache()
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("no license cache to export — run 'nself license validate' first")
	}
	return json.MarshalIndent(entry, "", "  ")
}

// ImportCache reads a previously exported cache file and writes it to the
// local cache location. It verifies the Ed25519 signature before accepting.
func ImportCache(data []byte) error {
	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return fmt.Errorf("invalid cache file format: %w", err)
	}
	if entry.KeyHash == "" || entry.Tier == "" {
		return fmt.Errorf("cache file is missing required fields")
	}
	if !entry.VerifySignature() {
		// NSELF_LICENSE_SKIP_VERIFY=1 allows importing unsigned cache entries for offline/dev use,
		// but requires explicit --force acknowledgment (signalled by NSELF_LICENSE_SKIP_VERIFY_FORCE=1
		// set by the caller after flag parsing). Standalone skip without --force is rejected.
		if os.Getenv("NSELF_LICENSE_SKIP_VERIFY") != "1" {
			return fmt.Errorf("cache file signature verification failed")
		}
		if os.Getenv("NSELF_LICENSE_SKIP_VERIFY_FORCE") != "1" {
			return fmt.Errorf("NSELF_LICENSE_SKIP_VERIFY requires --force flag; standalone skip is not permitted")
		}
		fmt.Fprintf(os.Stderr, "warning: accepting unsigned cache entry (skip-verify mode, --force acknowledged)\n")
	}
	return WriteCache(&entry)
}

// validateRemote performs the HTTP POST to /license/validate.
// S10.T03: After reading the body it verifies the X-NSelf-License-Sig Ed25519
// header. A missing or invalid signature causes the call to return an error so
// the caller falls through to the cached license.
func validateRemote(ctx context.Context, key string, pingURL string) (*ValidateResponse, error) {
	type request struct {
		LicenseKey string `json:"license_key"`
		Product    string `json:"product"`
	}
	body, err := json.Marshal(request{
		LicenseKey: key,
		Product:    "plugins-pro",
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}

	url := strings.TrimRight(pingURL, "/") + "/license/validate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	// Read the raw body so we can verify the signature against it.
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	// S10.T03: verify the Ed25519 response signature before trusting tier/plugins.
	// Never skipped: with no usable key nothing verifies and the call fails.
	sigHex := resp.Header.Get("X-NSelf-License-Sig")
	if err := verifyResponseSig(rawBody, sigHex); err != nil {
		return nil, err
	}

	var vr ValidateResponse
	if err := json.Unmarshal(rawBody, &vr); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	vr.RawBody, vr.BodySig = string(rawBody), sigHex
	if err := checkReplyBinding(&vr, key, time.Now()); err != nil {
		return nil, err
	}
	return &vr, nil
}

// responseToCache converts a server validation response to a cache entry.
func responseToCache(key string, resp *ValidateResponse) *CacheEntry {
	now := time.Now().Unix()
	var expiresAt int64
	if resp.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, resp.ExpiresAt); err == nil {
			expiresAt = t.Unix()
		}
	}
	return &CacheEntry{
		KeyHash:        HashKey(key),
		Tier:           resp.Tier,
		PluginsAllowed: pluginsFor(resp),
		FetchedAt:      now,
		ExpiresAt:      expiresAt,
		RawBody:        resp.RawBody,
		BodySig:        resp.BodySig,
		JWT:            resp.JWT,
		JWTKid:         resp.JWTKid,
		Signature:      resp.Signature,
		SignatureKeyID: resp.KeyID,
	}
}
