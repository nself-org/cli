package commands

// Purpose: ping_api HTTP calls backing `nself license migrate`, split out
// of license_migrate.go (CLI-R12 Batch B mechanical file-size split).
// Inputs: a context, ping_api base URL, and (for the link request) the
// license key + account UUID to associate.
// Outputs: the decoded migrateResponse/migrationStatusResponse/
// migrationInfo, or a display name for a product slug.
// Constraints: pure move, no behavior change. The response type
// definitions remain in license_migrate.go.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/httptimeout"
)

// sendMigrateRequest — POST /license/migrate on the ping_api.
func sendMigrateRequest(ctx context.Context, licenseKey, accountID, pingURL string) (*migrateResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	payload := map[string]string{
		"license_key": licenseKey,
		"account_id":  accountID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", pingURL+"/license/migrate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httptimeout.License.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result migrateResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("invalid server response: %w", err)
	}

	// Surface HTTP-level errors.
	if resp.StatusCode >= 400 && result.Error == "" {
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	return &result, nil
}

// fetchMigrationStatus uses the server-supported X-License-Key header to keep keys out of URLs.
func fetchMigrationStatus(ctx context.Context, key string, pingURL string) (*migrationStatusResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	url := pingURL + "/license/migration-status"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-License-Key", key)

	resp, err := httptimeout.License.Do(req)
	if err != nil {
		return nil, httptimeout.StripURL(err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result migrationStatusResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("invalid server response: %w", err)
	}

	return &result, nil
}

// fetchMigrationInfo is kept for backward compatibility with any callers.
func fetchMigrationInfo(ctx context.Context, key string, pingURL string) (*migrationInfo, error) { //nolint:unused // kept: licence-migration path has no entry point; see qa/bugs/declared-but-never-wired-symbols.md
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	bodyStr := fmt.Sprintf(`{"license_key":"%s"}`, key)
	req, err := http.NewRequestWithContext(ctx, "POST", pingURL+"/license/validate", strings.NewReader(bodyStr))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httptimeout.License.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var info migrationInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

func productDisplayName(product string) string { //nolint:unused // kept: licence-migration path has no entry point; see qa/bugs/declared-but-never-wired-symbols.md
	names := map[string]string{
		"claw":   "ɳClaw ($0.99/mo)",
		"chat":   "ɳChat ($0.99/mo)",
		"media":  "nTV ($0.99/mo)",
		"family": "nFamily ($0.99/mo)",
		"clawde": "ClawDE ($0.99/mo)",
		"plus":   "ɳSelf+ ($3.99/mo)",
		"owner":  "Owner (all access)",
	}
	if n, ok := names[product]; ok {
		return n
	}
	return product
}
