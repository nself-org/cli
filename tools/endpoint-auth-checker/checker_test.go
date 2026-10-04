// Package main — checker_test.go
// Unit and integration tests for the endpoint-auth-checker tool.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- allowlist / exempt tests ---

func TestIsExempt_ExactMatch(t *testing.T) {
	if !isExempt("/health") {
		t.Error("expected /health to be exempt")
	}
	if !isExempt("/metrics") {
		t.Error("expected /metrics to be exempt")
	}
}

func TestIsExempt_NotExempt(t *testing.T) {
	if isExempt("/api/data") {
		t.Error("/api/data should NOT be exempt")
	}
	if isExempt("/admin") {
		t.Error("/admin should NOT be exempt")
	}
}

func TestIsExempt_ChiParameterPattern(t *testing.T) {
	// A {param} segment matches exactly one path segment (neutral route).
	if !matchChiPattern("/items/{id}/status", "/items/42/status") {
		t.Error("expected chi parameter pattern to match")
	}
	// Different suffix should not match.
	if matchChiPattern("/items/{id}/status", "/items/42/secret") {
		t.Error("/items/42/secret should NOT match /items/{id}/status")
	}
	// Extra segments should not match.
	if matchChiPattern("/items/{id}/status", "/items/42/status/extra") {
		t.Error("extra segment should NOT match")
	}
	// The retired Q01 route is no longer exempt (D15).
	if isExempt("/plugin/identity/myplugin/public-key") {
		t.Error("retired identity route must not be exempt")
	}
}

func TestAllowedAuthMiddleware_Known(t *testing.T) {
	knownMWs := []string{
		"RequireLicenseKey",
		"RequireHasuraAdminKey",
		// RequireInternalSecret removed (P6-E11-W2-S3-T16 row 23): it was marked
		// "deprecated until Phase B-3 cutover" and B-3 has passed with zero
		// remaining callers anywhere in the estate (cli, plugins, plugins-pro).
		"RequireUserJWT",
		"AdminOnly",
		"InternalNetworkOnly",
	}
	for _, mw := range knownMWs {
		if _, ok := AllowedAuthMiddleware[mw]; !ok {
			t.Errorf("expected %q to be in allowlist", mw)
		}
	}
}

func TestAllowedAuthMiddleware_Unknown(t *testing.T) {
	unknownMWs := []string{"myCustomMiddleware", "doSomething", "", "RequirePluginJWT"}
	for _, mw := range unknownMWs {
		if _, ok := AllowedAuthMiddleware[mw]; ok {
			t.Errorf("did not expect %q to be in allowlist", mw)
		}
	}
}

// --- middleware classification tests ---

func makeRoute(path string, middlewares ...string) RouteRegistration {
	return RouteRegistration{
		File:        "test.go",
		Line:        1,
		Method:      "Handle",
		Path:        path,
		Middlewares: middlewares,
	}
}

func TestClassifyRoute_Passes_WithAllowedMW(t *testing.T) {
	r := makeRoute("/api/data", "RequireLicenseKey")
	cr := ClassifyRoute(r, true)
	if !cr.Passed {
		t.Errorf("expected route with RequireLicenseKey to pass")
	}
	if cr.MatchedMW != "RequireLicenseKey" {
		t.Errorf("expected MatchedMW=RequireLicenseKey, got %q", cr.MatchedMW)
	}
}

func TestClassifyRoute_Fails_NoMW(t *testing.T) {
	r := makeRoute("/api/leak")
	cr := ClassifyRoute(r, true)
	if cr.Passed {
		t.Error("expected route with no middleware to fail")
	}
}

func TestClassifyRoute_Fails_UnknownMW(t *testing.T) {
	r := makeRoute("/api/data", "myBespokeMW")
	cr := ClassifyRoute(r, true)
	if cr.Passed {
		t.Error("expected route with unknown middleware to fail")
	}
	if len(cr.UnknownMWs) == 0 || cr.UnknownMWs[0] != "myBespokeMW" {
		t.Errorf("expected UnknownMWs=[myBespokeMW], got %v", cr.UnknownMWs)
	}
}

func TestClassifyRoute_Exempt_Health(t *testing.T) {
	r := makeRoute("/health") // no middleware
	cr := ClassifyRoute(r, true)
	if !cr.Passed {
		t.Error("expected /health to be exempt and pass")
	}
	if !cr.Exempt {
		t.Error("expected cr.Exempt=true for /health")
	}
}

func TestClassifyRoute_MultipleMiddlewares_OneAllowed(t *testing.T) {
	r := makeRoute("/api/admin", "logRequest", "RequireHasuraAdminKey", "rateLimiter")
	cr := ClassifyRoute(r, true)
	if !cr.Passed {
		t.Error("expected route to pass when one middleware is in allowlist")
	}
}

// --- AST scanner tests ---

func TestScanDirs_Compliant(t *testing.T) {
	routes, err := ScanDirs([]string{"testdata/compliant_plugin"})
	if err != nil {
		t.Fatalf("ScanDirs error: %v", err)
	}
	failures := ClassifyAll(routes)
	if len(failures) != 0 {
		t.Errorf("expected 0 violations in compliant_plugin, got %d: %+v", len(failures), failures)
	}
}

func TestScanDirs_Noncompliant(t *testing.T) {
	routes, err := ScanDirs([]string{"testdata/noncompliant_plugin"})
	if err != nil {
		t.Fatalf("ScanDirs error: %v", err)
	}
	failures := ClassifyAll(routes)
	if len(failures) == 0 {
		t.Error("expected at least 1 violation in noncompliant_plugin, got 0")
	}
	// Verify the flagged path is /api/leak
	found := false
	for _, f := range failures {
		if f.Route.Path == "/api/leak" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected /api/leak to be flagged, violations: %+v", failures)
	}
}

func TestScanDirs_ExemptHealth(t *testing.T) {
	routes, err := ScanDirs([]string{"testdata/exempt_health"})
	if err != nil {
		t.Fatalf("ScanDirs error: %v", err)
	}
	failures := ClassifyAll(routes)
	if len(failures) != 0 {
		t.Errorf("expected 0 violations in exempt_health (health route should be exempt), got %d: %+v", len(failures), failures)
	}
}

func TestScanDirs_MissingDirFailsClosed(t *testing.T) {
	routes, err := ScanDirs([]string{"doesnotexist/"})
	if err == nil {
		t.Fatalf("expected an error for a missing directory, got routes %+v", routes)
	}
	if !strings.Contains(err.Error(), "doesnotexist") {
		t.Errorf("error should name the path, got %q", err)
	}
}

func TestScanDirs_MissingAmongValidDirFailsClosed(t *testing.T) {
	if _, err := ScanDirs([]string{"testdata/compliant_plugin", "doesnotexist"}); err == nil {
		t.Error("expected an error when any --dirs entry is missing")
	}
}

func TestScanDirs_FileEntryFailsClosed(t *testing.T) {
	if _, err := ScanDirs([]string{"testdata/compliant_plugin/plugin.go"}); err == nil {
		t.Error("expected an error when a --dirs entry is a file")
	}
}

const unauthedRoute = `package x

import "net/http"

func Register(mux *http.ServeMux) { mux.HandleFunc("/leak", nil) }
`

func writeGo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(unauthedRoute), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanDirs_SkipsVendoredAndFixtureTrees(t *testing.T) {
	root := t.TempDir()
	writeGo(t, filepath.Join(root, "own", "routes.go"))
	writeGo(t, filepath.Join(root, "vendor", "dep", "routes.go"))
	writeGo(t, filepath.Join(root, "own", "testdata", "routes.go"))

	routes, err := ScanDirs([]string{root})
	if err != nil {
		t.Fatalf("ScanDirs error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected only own/routes.go to be scanned, got %d routes: %+v", len(routes), routes)
	}
	if !strings.Contains(routes[0].File, filepath.Join("own", "routes.go")) {
		t.Errorf("unexpected file scanned: %s", routes[0].File)
	}
}

func TestScanDirs_RootInsideSkippedNameStillScanned(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vendor", "dep")
	writeGo(t, filepath.Join(root, "routes.go"))
	routes, err := ScanDirs([]string{root})
	if err != nil {
		t.Fatalf("ScanDirs error: %v", err)
	}
	if len(routes) != 1 {
		t.Errorf("an entry that points into vendor/ must be scanned, got %d routes", len(routes))
	}
}

func TestScanDirs_NodeModulesIsNotSkipped(t *testing.T) {
	// Go compiles a package at x/node_modules/y, so its routes must be seen.
	root := t.TempDir()
	writeGo(t, filepath.Join(root, "x", "node_modules", "y", "routes.go"))
	routes, err := ScanDirs([]string{root})
	if err != nil {
		t.Fatalf("ScanDirs error: %v", err)
	}
	if len(routes) != 1 {
		t.Errorf("expected the node_modules route to be scanned, got %d routes", len(routes))
	}
}

func TestScanDirs_SymlinkRootWithoutTrailingSlashIsScanned(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	writeGo(t, filepath.Join(real, "routes.go"))
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, entry := range []string{link, link + string(filepath.Separator)} {
		routes, err := ScanDirs([]string{entry})
		if err != nil {
			t.Fatalf("ScanDirs(%q) error: %v", entry, err)
		}
		if len(routes) != 1 {
			t.Errorf("ScanDirs(%q): expected 1 route through the symlink, got %d", entry, len(routes))
		}
	}
}

func TestScanDirs_EmptyDirFailsClosed(t *testing.T) {
	if _, err := ScanDirs([]string{t.TempDir()}); err == nil {
		t.Error("expected an error when no .go file is scanned in an empty directory")
	}
}

func TestScanDirs_OnlyUnparseableOrNonGoFailsClosed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "broken.go"), []byte("package ("), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ScanDirs([]string{root}); err == nil {
		t.Error("expected an error when no .go file parsed")
	}
}

func TestScanDirs_OnlyVendoredGoFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeGo(t, filepath.Join(root, "vendor", "dep", "routes.go"))
	if _, err := ScanDirs([]string{root}); err == nil {
		t.Error("expected an error when every .go file is under a skipped directory")
	}
}

// --- reporter exit-code tests ---

func TestReport_NoFailures_Returns0(t *testing.T) {
	code := Report(nil, FormatText, true)
	if code != 0 {
		t.Errorf("expected exit 0 with no failures, got %d", code)
	}
}

func TestReport_Failures_FailOnUnknown_Returns1(t *testing.T) {
	failures := []CheckResult{
		{Route: makeRoute("/api/leak"), Passed: false},
	}
	code := Report(failures, FormatText, true)
	if code != 1 {
		t.Errorf("expected exit 1 with failures + failOnUnknown, got %d", code)
	}
}

func TestReport_Failures_NoFailOnUnknown_Returns0(t *testing.T) {
	failures := []CheckResult{
		{Route: makeRoute("/api/leak"), Passed: false},
	}
	code := Report(failures, FormatText, false)
	if code != 0 {
		t.Errorf("expected exit 0 with failures but !failOnUnknown, got %d", code)
	}
}
