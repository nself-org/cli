package commands

// Purpose: GitHub-release and ping-API lookups used by `nself update` and
// `nself update --check`: resolving the latest (or a specific) release tag
// and download URL, normalizing version strings for comparison, resolving a
// migration-guide URL for the current version via the ping API, and
// printing the check-only result. Split out of update.go (CLI-R12) to
// separate these read-only lookups from the self-update orchestration
// (update_selfupdate.go) and file primitives (update_binary_ops.go).
// Inputs: an optional release tag or GitHub API URL, and the current CLI
// version string.
// Outputs: a resolved tag/URL pair, a normalized version string, a
// migration-guide URL (or ""), and the printed --check report.
// Constraints: pure move — no behavior changes.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/ui"
)

func fetchLatestRelease() (tag string, url string, err error) {
	return fetchReleaseFromURL(githubLatestReleaseURL)
}

// fetchReleaseByTag queries the GitHub API for a specific tagged release.
func fetchReleaseByTag(tag string) (string, string, error) {
	return fetchReleaseFromURL(fmt.Sprintf(githubReleaseByTagURL, tag))
}

// fetchReleaseFromURL fetches and parses a GitHub release from the given API URL.
func fetchReleaseFromURL(apiURL string) (tag string, htmlURL string, err error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", &releaseAPIStatusError{StatusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read response: %w", err)
	}

	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return "", "", fmt.Errorf("failed to parse release JSON: %w", err)
	}

	if release.TagName == "" {
		return "", "", fmt.Errorf("no tag_name in release response")
	}

	return release.TagName, release.HTMLURL, nil
}

// normalizeVersion strips a leading "v" for comparison.
func normalizeVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}

// pingVersionResponse is the minimal shape we need from GET ping.nself.org/version.
type pingVersionResponse struct {
	Migration *struct {
		Available bool   `json:"available"`
		From      string `json:"from"`
		To        string `json:"to"`
		GuideURL  string `json:"guide_url"`
	} `json:"migration,omitempty"`
}

// fetchMigrationGuideURL queries ping.nself.org/version with the current CLI
// User-Agent. If the server returns a migration block (v0.9.x UA), the guide
// URL is returned. Returns an empty string on any error — callers treat it as
// "no migration info available" and proceed without blocking.
func fetchMigrationGuideURL(currentVersion string) string {
	pingURL := "https://ping.nself.org/version"
	ua := fmt.Sprintf("nself/%s", strings.TrimPrefix(currentVersion, "v"))
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, pingURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	var pv pingVersionResponse
	if err := json.Unmarshal(body, &pv); err != nil {
		return ""
	}
	if pv.Migration != nil && pv.Migration.Available && pv.Migration.GuideURL != "" {
		return pv.Migration.GuideURL
	}
	return ""
}

// printCheckResult displays the version comparison result.
// For v0.9.x CLIs, it also queries ping.nself.org for a migration guide URL.
func printCheckResult(current, latest, releaseURL string, upToDate bool) error {
	fmt.Printf("%s %s\n", ui.C(ui.Bold, "Current version:"), current)
	fmt.Printf("%s %s\n", ui.C(ui.Bold, "Latest release: "), latest)

	// S60-T06: Surface migration guide URL for v0.9.x CLIs.
	normalCurrent := normalizeVersion(current)
	if strings.HasPrefix(normalCurrent, "0.") {
		if guideURL := fetchMigrationGuideURL(current); guideURL != "" {
			fmt.Println()
			ui.Warn("You are running a legacy v0.9 CLI. Migration to v1.0.9 is required.")
			ui.Info(fmt.Sprintf("Upgrade guide: %s", guideURL))
			fmt.Println()
		}
	}

	if upToDate {
		ui.Success("You are running the latest version.")
	} else {
		ui.Warn(fmt.Sprintf("Update available: %s -> %s", current, latest))
		if releaseURL != "" {
			ui.Info(fmt.Sprintf("Release notes: %s", releaseURL))
		}
		fmt.Printf("\nRun %s to update.\n", ui.C(ui.Cyan, "nself update"))
	}
	return nil
}

// releaseAPIStatusError preserves the response status for update-plan decisions.
type releaseAPIStatusError struct{ StatusCode int }

func (e *releaseAPIStatusError) Error() string {
	return fmt.Sprintf("GitHub API returned status %d", e.StatusCode)
}
