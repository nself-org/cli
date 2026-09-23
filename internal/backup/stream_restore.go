package backup

// stream_restore.go — restoring a streamed backup from a remote destination.
//
// Purpose: resolve encryption recipients (including GitHub-hosted SSH keys), build the target Postgres URL and drive RestoreFromRemote, split out of stream.go for file size.
// Inputs: the remote backup location, recipient identifiers and the target database connection info.
// Outputs: a restored database, or an error identifying which stage of the restore failed.
// Constraints: pure move from stream.go (CLI-R12 Batch E); no behaviour change.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/httptimeout"
)

// resolveRecipients expands "github:<username>" entries into SSH public keys.
func resolveRecipients(ctx context.Context, recipients []string) ([]string, error) {
	var resolved []string
	for _, r := range recipients {
		if strings.HasPrefix(r, "github:") {
			username := strings.TrimPrefix(r, "github:")
			keys, err := fetchGitHubKeys(ctx, username)
			if err != nil {
				return nil, fmt.Errorf("fetch GitHub keys for %s: %w", username, err)
			}
			resolved = append(resolved, keys...)
		} else {
			resolved = append(resolved, r)
		}
	}
	return resolved, nil
}

// fetchGitHubKeys fetches SSH public keys for a GitHub username.
func fetchGitHubKeys(ctx context.Context, username string) ([]string, error) {
	url := "https://api.github.com/users/" + username + "/keys"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httptimeout.Backup.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub API request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d for user %s", resp.StatusCode, username)
	}

	var apiKeys []struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiKeys); err != nil {
		return nil, fmt.Errorf("decode GitHub API response: %w", err)
	}
	if len(apiKeys) == 0 {
		return nil, fmt.Errorf("no SSH keys found for GitHub user %s", username)
	}

	keys := make([]string, len(apiKeys))
	for i, k := range apiKeys {
		keys[i] = k.Key
	}
	return keys, nil
}

// buildPgURL constructs a postgres DSN from config.
func buildPgURL(cfg *config.Config) string {
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}
	password := cfg.Postgres.Password
	host := "localhost"
	port := cfg.Postgres.Port
	if port == 0 {
		port = 5432
	}
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}
	if password != "" {
		return fmt.Sprintf("postgresql://%s@%s:%d/%s?sslmode=disable", config.URLUserInfo(user, password), host, port, db)
	}
	return fmt.Sprintf("postgresql://%s@%s:%d/%s?sslmode=disable", config.URLUserInfo(user, ""), host, port, db)
}

// redactURL removes the password component of a postgres DSN for logging.
// It handles "scheme://user:pass@host" but not "scheme://user@host" (no password).
func redactURL(url string) string {
	// Find the authority part: after "://"
	schemeEnd := strings.Index(url, "://")
	if schemeEnd < 0 {
		return url
	}
	authority := url[schemeEnd+3:]

	atIdx := strings.Index(authority, "@")
	if atIdx < 0 {
		// No @ means no userinfo at all.
		return url
	}

	userinfo := authority[:atIdx]
	colonIdx := strings.Index(userinfo, ":")
	if colonIdx < 0 {
		// userinfo has no colon — no password present.
		return url
	}

	// Replace password with ***.
	redacted := url[:schemeEnd+3] + userinfo[:colonIdx+1] + "***" + url[schemeEnd+3+atIdx:]
	return redacted
}

// checkBinaries verifies that required external programs are on PATH.
func checkBinaries(recipients []string) error {
	required := []string{"pg_dump", "rclone"}
	if len(recipients) > 0 {
		required = append(required, "age")
	}
	for _, bin := range required {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("required binary %q not found on PATH: install it before running backup stream", bin)
		}
	}
	return nil
}
