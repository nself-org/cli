package doctor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestParseSemver verifies semver parsing across good and bad inputs.
func TestParseSemver(t *testing.T) {
	cases := []struct {
		in        string
		wantMajor int
		wantMinor int
		wantPatch int
		wantErr   bool
	}{
		{"1.0.13", 1, 0, 13, false},
		{"0.9.0", 0, 9, 0, false},
		{"2.1.3-beta", 2, 1, 3, false}, // pre-release suffix ignored
		{"1.0", 0, 0, 0, true},
		{"notver", 0, 0, 0, true},
	}
	for _, tc := range cases {
		maj, min, pat, err := parseSemver(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseSemver(%q): expected error, got none", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSemver(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if maj != tc.wantMajor || min != tc.wantMinor || pat != tc.wantPatch {
			t.Errorf("parseSemver(%q) = (%d,%d,%d), want (%d,%d,%d)",
				tc.in, maj, min, pat, tc.wantMajor, tc.wantMinor, tc.wantPatch)
		}
	}
}

// TestVersionDriftStatus covers all drift scenarios.
func TestVersionDriftStatus(t *testing.T) {
	cases := []struct {
		sdkVer     string
		cliVer     string
		wantStatus string
	}{
		{"1.0.12", "1.0.13", "warn"}, // patch diff == 1
		{"1.0.11", "1.0.13", "warn"}, // patch diff == 2
		{"1.1.0", "1.0.13", "fail"},  // minor mismatch
		{"2.0.0", "1.0.13", "fail"},  // major mismatch
		{"1.0.14", "1.0.13", "warn"}, // SDK ahead by 1
		{"1.0.15", "1.0.13", "warn"}, // SDK ahead by 2
	}
	for _, tc := range cases {
		got, _ := versionDriftStatus("pkg", tc.sdkVer, tc.cliVer)
		if got != tc.wantStatus {
			t.Errorf("versionDriftStatus(sdk=%s cli=%s) = %q, want %q",
				tc.sdkVer, tc.cliVer, got, tc.wantStatus)
		}
	}
}

// TestCheckOneSDK_Pass tests the happy path using a local HTTP server.
func TestCheckOneSDK_Pass(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.0.13"})
	}))
	defer ts.Close()

	client := ts.Client()
	entry := sdkEntry{
		name: "SDK-VERSION-01/test",
		pkg:  "test-sdk (npm)",
		fetchFn: func(ctx context.Context, c *http.Client) (string, error) {
			return fetchNPMVersionURL(ctx, c, ts.URL+"/latest")
		},
	}

	result := checkOneSDK(context.Background(), client, entry, "1.0.13")
	if result.Status != "pass" {
		t.Errorf("expected pass, got %q: %s", result.Status, result.Message)
	}
}

// TestCheckOneSDK_Warn tests that a 404 (not published) yields a warn.
func TestCheckOneSDK_Warn(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client := ts.Client()
	entry := sdkEntry{
		name: "SDK-VERSION-01/test",
		pkg:  "test-sdk (npm)",
		fetchFn: func(ctx context.Context, c *http.Client) (string, error) {
			return fetchNPMVersionURL(ctx, c, ts.URL+"/latest")
		},
	}

	result := checkOneSDK(context.Background(), client, entry, "1.0.13")
	if result.Status != "warn" {
		t.Errorf("expected warn (not published), got %q: %s", result.Status, result.Message)
	}
}

// TestCheckOneSDK_Drift tests that a drifted version yields warn with FixCmd.
func TestCheckOneSDK_Drift(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.0.10"})
	}))
	defer ts.Close()

	client := ts.Client()
	entry := sdkEntry{
		name: "SDK-VERSION-01/test",
		pkg:  "test-sdk (npm)",
		fetchFn: func(ctx context.Context, c *http.Client) (string, error) {
			return fetchNPMVersionURL(ctx, c, ts.URL+"/latest")
		},
	}

	result := checkOneSDK(context.Background(), client, entry, "1.0.13")
	if result.Status != "warn" {
		t.Errorf("expected warn (drift), got %q: %s", result.Status, result.Message)
	}
	if result.FixCmd != "nself sdk release" {
		t.Errorf("expected FixCmd 'nself sdk release', got %q", result.FixCmd)
	}
}

// TestCheckOneSDK_MajorMismatch tests that major mismatch yields fail.
func TestCheckOneSDK_MajorMismatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"version": "2.0.0"})
	}))
	defer ts.Close()

	client := ts.Client()
	entry := sdkEntry{
		name: "SDK-VERSION-01/test",
		pkg:  "test-sdk (npm)",
		fetchFn: func(ctx context.Context, c *http.Client) (string, error) {
			return fetchNPMVersionURL(ctx, c, ts.URL+"/latest")
		},
	}

	result := checkOneSDK(context.Background(), client, entry, "1.0.13")
	if result.Status != "fail" {
		t.Errorf("expected fail (major mismatch), got %q: %s", result.Status, result.Message)
	}
}

// TestCheckOneSDK_NetworkError tests that a connection failure yields warn.
func TestCheckOneSDK_NetworkError(t *testing.T) {
	client := &http.Client{Timeout: 1 * time.Second}
	entry := sdkEntry{
		name: "SDK-VERSION-01/test",
		pkg:  "test-sdk (npm)",
		fetchFn: func(ctx context.Context, c *http.Client) (string, error) {
			// Point at a port that nothing listens on.
			return fetchNPMVersionURL(ctx, c, "http://127.0.0.1:19999/latest")
		},
	}

	result := checkOneSDK(context.Background(), client, entry, "1.0.13")
	if result.Status != "warn" {
		t.Errorf("expected warn (network error), got %q: %s", result.Status, result.Message)
	}
}

// TestCheckSDKVersions_Smoke ensures CheckSDKVersions returns exactly 3 results
// (go, py, ts; the retired Dart SDK was removed 2026-06-30) and that all
// names start with "SDK-VERSION-01/".
// In pre-publication state all 3 SDKs return warn (404); that is expected.
func TestCheckSDKVersions_Smoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results := CheckSDKVersions(ctx)
	if len(results) != 3 {
		t.Fatalf("expected 3 CheckResults, got %d", len(results))
	}
	for i, r := range results {
		if r.Section != "sdk" {
			t.Errorf("result[%d].Section = %q, want \"sdk\"", i, r.Section)
		}
		if !strings.HasPrefix(r.Name, "SDK-VERSION-01/") {
			t.Errorf("result[%d].Name = %q, want prefix \"SDK-VERSION-01/\"", i, r.Name)
		}
		if r.Status == "" {
			t.Errorf("result[%d].Status is empty", i)
		}
	}
}

// fetchNPMVersionURL is a test-injectable variant of fetchNPMVersion that
// accepts an explicit URL rather than constructing one from the package name.
func fetchNPMVersionURL(ctx context.Context, client *http.Client, url string) (string, error) {
	body, err := getJSON(ctx, client, url)
	if err != nil {
		return "", err
	}
	if body == nil {
		return "", nil
	}
	var resp struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	return resp.Version, nil
}
