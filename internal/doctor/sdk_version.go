package doctor

// sdk_version.go — S03-T06 SDK-VERSION-01 deep doctor check.
//
// Queries each of the 3 SDK registries and compares the published version
// against the local CLI version. A drift of more than one patch segment is
// a warn; a major or minor mismatch is a fail. Registry errors are warn-only
// (network is best-effort in doctor --deep).
// The retired Dart SDK was removed 2026-06-30 per ASI Policy 2.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/version"
)

// sdkEntry describes one SDK package to check.
type sdkEntry struct {
	name    string // human-readable name for the check result
	pkg     string // package identifier (npm name, PyPI name, etc.)
	fetchFn func(ctx context.Context, client *http.Client) (string, error)
}

// CheckSDKVersions implements the SDK-VERSION-01 deep doctor check.
// It returns one CheckResult per SDK (4 total), each independently
// pass / warn / fail so a single drifted SDK does not mask the others.
func CheckSDKVersions(ctx context.Context) []CheckResult {
	cli := version.GetVersion()
	cli = strings.TrimPrefix(cli, "v")

	client := &http.Client{Timeout: 8 * time.Second}

	sdks := []sdkEntry{
		{
			name: "SDK-VERSION-01/ts",
			pkg:  "@nself/sdk (npm)",
			fetchFn: func(ctx context.Context, c *http.Client) (string, error) {
				return fetchNPMVersion(ctx, c, "@nself/sdk")
			},
		},
		{
			name: "SDK-VERSION-01/py",
			pkg:  "nself-sdk (PyPI)",
			fetchFn: func(ctx context.Context, c *http.Client) (string, error) {
				return fetchPyPIVersion(ctx, c, "nself-sdk")
			},
		},
		{
			name: "SDK-VERSION-01/go",
			pkg:  "github.com/nself-org/sdk",
			fetchFn: func(ctx context.Context, c *http.Client) (string, error) {
				return fetchGoProxyVersion(ctx, c, "github.com/nself-org/sdk")
			},
		},
	}

	results := make([]CheckResult, 0, len(sdks))
	for _, sdk := range sdks {
		results = append(results, checkOneSDK(ctx, client, sdk, cli))
	}
	return results
}

// checkOneSDK evaluates a single SDK entry against the local CLI version.
func checkOneSDK(ctx context.Context, client *http.Client, sdk sdkEntry, cliVer string) CheckResult {
	sdkVer, err := sdk.fetchFn(ctx, client)
	if err != nil {
		return CheckResult{
			Section: "sdk",
			Name:    sdk.name,
			Status:  "warn",
			Message: fmt.Sprintf("SDK-VERSION-01: %s — cannot fetch version: %v", sdk.pkg, err),
		}
	}
	if sdkVer == "" {
		return CheckResult{
			Section: "sdk",
			Name:    sdk.name,
			Status:  "warn",
			Message: fmt.Sprintf("SDK-VERSION-01: %s — not yet published", sdk.pkg),
		}
	}

	sdkVer = strings.TrimPrefix(sdkVer, "v")
	if sdkVer == cliVer {
		return CheckResult{
			Section: "sdk",
			Name:    sdk.name,
			Status:  "pass",
			Message: fmt.Sprintf("SDK-VERSION-01: %s v%s matches CLI v%s", sdk.pkg, sdkVer, cliVer),
		}
	}

	status, msg := versionDriftStatus(sdk.pkg, sdkVer, cliVer)
	return CheckResult{
		Section: "sdk",
		Name:    sdk.name,
		Status:  status,
		Message: fmt.Sprintf("SDK-VERSION-01: %s v%s vs CLI v%s — %s", sdk.pkg, sdkVer, cliVer, msg),
		FixCmd:  "nself sdk release",
	}
}

// versionDriftStatus returns (status, detail) for a mismatched sdk/cli pair.
// Drift rules:
//   - major or minor mismatch → fail
//   - patch diff > 1 → warn
//   - patch diff == 1 → warn (informational)
func versionDriftStatus(pkg, sdkVer, cliVer string) (string, string) {
	sM, sm, sp, sErr := parseSemver(sdkVer)
	cM, cm, cp, cErr := parseSemver(cliVer)
	if sErr != nil || cErr != nil {
		return "warn", "cannot compare versions (non-semver)"
	}
	if sM != cM || sm != cm {
		return "fail", fmt.Sprintf("major/minor mismatch (%d.%d vs %d.%d)", sM, sm, cM, cm)
	}
	diff := sp - cp
	if diff < 0 {
		diff = -diff
	}
	if diff > 1 {
		return "warn", fmt.Sprintf("patch drift %d (>1)", diff)
	}
	return "warn", "patch drift 1"
}

// parseSemver parses "major.minor.patch" into three ints.
func parseSemver(v string) (major, minor, patch int, err error) {
	parts := strings.Split(v, ".")
	if len(parts) < 3 {
		return 0, 0, 0, fmt.Errorf("not semver: %q", v)
	}
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return
	}
	// Ignore pre-release suffixes after patch (e.g. "1.0.0-beta").
	patchStr := strings.SplitN(parts[2], "-", 2)[0]
	patch, err = strconv.Atoi(patchStr)
	return
}

// fetchNPMVersion queries the npm registry for the latest version of pkg.
func fetchNPMVersion(ctx context.Context, client *http.Client, pkg string) (string, error) {
	// Encode scoped packages: @nself/sdk → @nself%2Fsdk
	encoded := strings.ReplaceAll(pkg, "/", "%2F")
	url := fmt.Sprintf("https://registry.npmjs.org/%s/latest", encoded)
	body, err := getJSON(ctx, client, url)
	if err != nil {
		return "", err
	}
	if body == nil {
		return "", nil // not yet published
	}
	var resp struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse npm response: %w", err)
	}
	return resp.Version, nil
}

// fetchPyPIVersion queries PyPI for the latest version of pkg.
func fetchPyPIVersion(ctx context.Context, client *http.Client, pkg string) (string, error) {
	url := fmt.Sprintf("https://pypi.org/pypi/%s/json", pkg)
	body, err := getJSON(ctx, client, url)
	if err != nil {
		return "", err
	}
	if body == nil {
		return "", nil // not yet published
	}
	var resp struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse pypi response: %w", err)
	}
	return resp.Info.Version, nil
}

// fetchGoProxyVersion queries the Go module proxy for the latest version of mod.
func fetchGoProxyVersion(ctx context.Context, client *http.Client, mod string) (string, error) {
	url := fmt.Sprintf("https://proxy.golang.org/%s/@latest", mod)
	body, err := getJSON(ctx, client, url)
	if err != nil {
		return "", err
	}
	if body == nil {
		return "", nil // not yet published
	}
	var resp struct {
		Version string `json:"Version"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse go proxy response: %w", err)
	}
	v := strings.TrimPrefix(resp.Version, "v")
	return v, nil
}

// getJSON performs a GET request and returns the response body.
// Returns ("", nil) for 404 (package not yet published).
func getJSON(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "nself-doctor/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // not yet published — caller treats empty string as warn
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return data, nil
}
