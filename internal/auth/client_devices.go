package auth

// client_devices.go — device listing, revocation and license transfer.
//
// Purpose: list and revoke authorized devices, transfer a license between devices, and parse auth server error responses, used by device-related commands, split out of client.go for file size.
// Inputs: an account/session token and, for transfer/revoke, the target device id.
// Outputs: DeviceEntry values, or an error from the auth server.
// Constraints: GetDevices maps a 404 to E225 (auth_server serves no /account/devices route); the rest is a pure move from client.go.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/nself-org/cli/internal/errs"
)

// DeviceEntry represents one registered device for an account.
type DeviceEntry struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	OS         string `json:"os"`
	LastActive string `json:"last_active"`
	IsCurrent  bool   `json:"is_current"`
}

// GetDevices returns the list of registered devices for the account.
func GetDevices(ctx context.Context, accessToken string) ([]DeviceEntry, error) {
	url := fmt.Sprintf("%s/account/devices", AuthServerURL())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", fmt.Sprintf("nself_auth_token=%s", accessToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting devices: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errServerLacksPath("/account/devices")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp)
	}

	var result struct {
		Devices []DeviceEntry `json:"devices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing devices response: %w", err)
	}
	return result.Devices, nil
}

// RevokeDevice revokes a specific device session.
func RevokeDevice(ctx context.Context, accessToken, deviceID string) error {
	url := fmt.Sprintf("%s/account/devices/%s", AuthServerURL(), deviceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", fmt.Sprintf("nself_auth_token=%s", accessToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("revoking device: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return parseAPIError(resp)
	}
	return nil
}

// ─── Transfer ─────────────────────────────────────────────────────────────────

// TransferLicense transfers a license from the current account to another email.
func TransferLicense(ctx context.Context, accessToken, licenseID, toEmail string) error {
	url := fmt.Sprintf("%s/account/licenses/%s/transfer", AuthServerURL(), licenseID)
	body, _ := json.Marshal(map[string]string{"to_email": toEmail})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", fmt.Sprintf("nself_auth_token=%s", accessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("transferring license: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return parseAPIError(resp)
	}
	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// errServerLacksPath is the E225 error: the auth server answered 404 for a path
// the CLI calls but the server does not provide (never a raw 404, never an
// empty list). The path is the route only, never a token or query.
func errServerLacksPath(path string) error {
	return errs.Newf("E225", "this server does not provide %s", path)
}

func parseAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	var apiErr AuthAPIError
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return fmt.Errorf("auth server error (HTTP %d): %s", resp.StatusCode, string(body))
	}
	apiErr.Status = resp.StatusCode
	return &apiErr
}

func getEnv(key string) string {
	// os imported at package level via storage.go
	return os.Getenv(key)
}
