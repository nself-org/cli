package auth

// client_team.go — team member management API calls.
//
// Purpose: list, invite, remove and change the role of team members, and activate a license for the account, used by team-related commands, split out of client.go for file size.
// Inputs: an account/session token and the target team member's details.
// Outputs: TeamMember values, or an error from the auth server.
// Constraints: GetTeamMembers maps a 404 to E225 (auth_server serves no /account/team route); the rest is a pure move from client.go.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// TeamMember represents one member of an account's team.
type TeamMember struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	JoinedAt string `json:"joined_at"`
}

// GetTeamMembers returns the list of team members for the account.
func GetTeamMembers(ctx context.Context, accessToken string) ([]TeamMember, error) {
	url := fmt.Sprintf("%s/account/team", AuthServerURL())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", fmt.Sprintf("nself_auth_token=%s", accessToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting team: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errServerLacksPath("/account/team")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp)
	}

	var result struct {
		Members []TeamMember `json:"members"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing team response: %w", err)
	}
	return result.Members, nil
}

// InviteTeamMember sends a team invitation to the given email.
func InviteTeamMember(ctx context.Context, accessToken, email string) error {
	url := fmt.Sprintf("%s/account/team/invite", AuthServerURL())
	body, _ := json.Marshal(map[string]string{"email": email})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", fmt.Sprintf("nself_auth_token=%s", accessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("inviting team member: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return parseAPIError(resp)
	}
	return nil
}

// RemoveTeamMember removes a member from the account's team.
func RemoveTeamMember(ctx context.Context, accessToken, email string) error {
	url := fmt.Sprintf("%s/account/team/%s", AuthServerURL(), email)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", fmt.Sprintf("nself_auth_token=%s", accessToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("removing team member: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return parseAPIError(resp)
	}
	return nil
}

// SetTeamMemberRole updates a member's role on the account's team.
func SetTeamMemberRole(ctx context.Context, accessToken, email, role string) error {
	url := fmt.Sprintf("%s/account/team/%s/role", AuthServerURL(), email)
	body, _ := json.Marshal(map[string]string{"role": role})
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", fmt.Sprintf("nself_auth_token=%s", accessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("setting team member role: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return parseAPIError(resp)
	}
	return nil
}

// ─── Licenses (extended) ──────────────────────────────────────────────────────

// ActivateLicense activates a license key on the current device.
func ActivateLicense(ctx context.Context, accessToken, licenseID string) error {
	url := fmt.Sprintf("%s/account/licenses/%s/activate", AuthServerURL(), licenseID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", fmt.Sprintf("nself_auth_token=%s", accessToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("activating license: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return parseAPIError(resp)
	}
	return nil
}
