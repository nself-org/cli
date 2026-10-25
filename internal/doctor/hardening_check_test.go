package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginx"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// writeEnvFile writes key=value lines to fname inside dir.
func writeEnvFile(t *testing.T, dir, fname string, lines ...string) {
	t.Helper()
	path := filepath.Join(dir, fname)
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeEnvFile %s: %v", fname, err)
	}
}

// ─── SEC-HARDENING-02: rate-limit env vars ───────────────────────────────────

func TestCheckHardeningRateLimit_AllSet(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.dev",
		"AUTH_RATE_LIMIT_ENABLED=true",
		"AUTH_RATE_LIMIT_MAX_REQUESTS=100",
		"AUTH_RATE_LIMIT_WINDOW_SECONDS=60",
	)

	res := checkHardeningRateLimit(dir)
	if res.Status != "pass" {
		t.Errorf("all rate-limit vars set: status=%q, want pass; msg=%s", res.Status, res.Message)
	}
	if res.Name != "SEC-HARDENING-02" {
		t.Errorf("Name=%q, want SEC-HARDENING-02", res.Name)
	}
}

func TestCheckHardeningRateLimit_Missing(t *testing.T) {
	dir := t.TempDir()
	// Only one of the three required vars present.
	writeEnvFile(t, dir, ".env.dev", "AUTH_RATE_LIMIT_ENABLED=true")

	res := checkHardeningRateLimit(dir)
	if res.Status != "warn" {
		t.Errorf("missing rate-limit vars: status=%q, want warn; msg=%s", res.Status, res.Message)
	}
	if !strings.Contains(res.Message, "missing rate-limit") {
		t.Errorf("unexpected message: %s", res.Message)
	}
}

func TestCheckHardeningRateLimit_NoneSet(t *testing.T) {
	dir := t.TempDir()
	// Empty env file — no vars at all.
	writeEnvFile(t, dir, ".env.dev", "# nothing here")

	res := checkHardeningRateLimit(dir)
	if res.Status != "warn" {
		t.Errorf("no rate-limit vars: status=%q, want warn", res.Status)
	}
	if res.FixCmd == "" {
		t.Error("expected a FixCmd suggestion")
	}
}

// ─── SEC-HARDENING-03: MFA throttle ─────────────────────────────────────────

func TestCheckHardeningMFAThrottle_Set(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.dev", "AUTH_MFA_MAX_ATTEMPTS_PER_HOUR=5")

	res := checkHardeningMFAThrottle(dir)
	if res.Status != "pass" {
		t.Errorf("MFA throttle set: status=%q, want pass; msg=%s", res.Status, res.Message)
	}
}

func TestCheckHardeningMFAThrottle_Missing(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.dev", "# no MFA throttle")

	res := checkHardeningMFAThrottle(dir)
	if res.Status != "warn" {
		t.Errorf("MFA throttle missing: status=%q, want warn", res.Status)
	}
	if !strings.Contains(res.Message, "AUTH_MFA_MAX_ATTEMPTS_PER_HOUR") {
		t.Errorf("message missing key name: %s", res.Message)
	}
}

// ─── SEC-HARDENING-04: SSRF guard import ─────────────────────────────────────

func TestCheckHardeningSSRFImport_NoPaidDir(t *testing.T) {
	dir := t.TempDir()
	// No plugins-pro directory adjacent to dir — expect warn.
	res := checkHardeningSSRFImport(dir)
	if res.Status != "warn" {
		t.Errorf("no plugins-pro dir: status=%q, want warn; msg=%s", res.Status, res.Message)
	}
}

func TestCheckHardeningSSRFImport_WithGuard(t *testing.T) {
	// Build a fake project structure where plugins-pro/paid/ai/shared/ssrf.go exists
	// and contains a guard symbol.
	root := t.TempDir()
	projectDir := filepath.Join(root, "myproject")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	guardDir := filepath.Join(root, "plugins-pro", "paid", "ai", "shared")
	if err := os.MkdirAll(guardDir, 0o755); err != nil {
		t.Fatal(err)
	}
	guardFile := filepath.Join(guardDir, "ssrf.go")
	content := "package shared\n\nfunc IsBlockedIP(ip string) bool { return false }\n"
	if err := os.WriteFile(guardFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	res := checkHardeningSSRFImport(projectDir)
	// The ai plugin now has a guard; others are absent so skipped.
	// Result may be pass (no missing plugins) or warn (plugins-pro not found for missing ones).
	validStatuses := map[string]bool{"pass": true, "warn": true, "fail": true}
	if !validStatuses[res.Status] {
		t.Errorf("invalid status %q", res.Status)
	}
}

// ─── SEC-HARDENING-05: JWT public keys ──────────────────────────────────────

func TestCheckHardeningJWTPublicKeys_NotSet(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.dev", "# no JWT_PUBLIC_KEYS")

	res := checkHardeningJWTPublicKeys(dir)
	if res.Status != "warn" {
		t.Errorf("no JWT_PUBLIC_KEYS: status=%q, want warn", res.Status)
	}
}

func TestCheckHardeningJWTPublicKeys_OneKey(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.dev", `JWT_PUBLIC_KEYS=-----BEGIN PUBLIC KEY-----abc-----END PUBLIC KEY-----`)

	res := checkHardeningJWTPublicKeys(dir)
	if res.Status != "warn" {
		t.Errorf("single key: status=%q, want warn; msg=%s", res.Status, res.Message)
	}
}

func TestCheckHardeningJWTPublicKeys_TwoKeys(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.dev",
		`JWT_PUBLIC_KEYS=-----BEGIN PUBLIC KEY-----abc-----END PUBLIC KEY----------BEGIN PUBLIC KEY-----def-----END PUBLIC KEY-----`,
	)

	res := checkHardeningJWTPublicKeys(dir)
	if res.Status != "pass" {
		t.Errorf("two keys: status=%q, want pass; msg=%s", res.Status, res.Message)
	}
}

func TestCountJWTKeys_CommaList(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"key1,key2,key3", 3},
		{"key1", 1},
		{"key1,", 1}, // trailing comma produces one empty part
		{"", 0},
	}
	for _, c := range cases {
		got := countJWTKeys(c.input)
		if got != c.want {
			t.Errorf("countJWTKeys(%q) = %d, want %d", c.input, got, c.want)
		}
	}
}

func TestCountJWTKeys_JSONArray(t *testing.T) {
	val := `[{"kty":"RSA","n":"abc"},{"kty":"RSA","n":"def"}]`
	got := countJWTKeys(val)
	if got != 2 {
		t.Errorf("JSON array with 2 JWKs: got %d, want 2", got)
	}
}

func TestCountJWTKeys_PEMBlocks(t *testing.T) {
	val := "-----BEGIN PUBLIC KEY-----abc-----END PUBLIC KEY----------BEGIN PUBLIC KEY-----def-----END PUBLIC KEY-----"
	got := countJWTKeys(val)
	if got != 2 {
		t.Errorf("two PEM blocks: got %d, want 2", got)
	}
}

// ─── SEC-HARDENING-06: Nginx rate zones ─────────────────────────────────────

func TestCheckHardeningNginxRateZones_BothPresent(t *testing.T) {
	dir := t.TempDir()
	confDir := filepath.Join(dir, "nginx", "conf.d")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `
limit_req_zone $binary_remote_addr zone=auth_login:10m rate=5r/m;
location /auth/login { limit_req zone=auth_login; }
limit_req_zone $binary_remote_addr zone=api:10m rate=30r/s;
location /api/ { limit_req zone=api; }
`
	if err := os.WriteFile(filepath.Join(confDir, "ratelimit.conf"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	res := checkHardeningNginxRateZones(ctx, dir)
	if res.Status != "pass" {
		t.Errorf("both zones present: status=%q, want pass; msg=%s", res.Status, res.Message)
	}
}

func TestCheckHardeningNginxRateZones_NonePresent(t *testing.T) {
	dir := t.TempDir()
	confDir := filepath.Join(dir, "nginx", "conf.d")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "default.conf"), []byte("# empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	res := checkHardeningNginxRateZones(ctx, dir)
	// Without Docker running the fallback will also return nothing.
	if res.Status != "fail" && res.Status != "warn" {
		t.Errorf("no zones: status=%q, want fail or warn", res.Status)
	}
}

// TestCheckHardeningNginxRateZones_RealGeneratorOutputPasses is the closed-
// loop regression for P6-E2-W2-S3-T20: before this ticket, `nself build`'s
// nginx generator emitted only server-wide RateZone locations (no
// path-scoped /auth/login or /api/ locations at all), so this check would
// have failed against real generator output even though the two synthetic
// fixture tests above pass with hand-written confs. This test writes the
// REAL internal/nginx.Generate() output to disk (the exact files/paths
// `nself build` writes) and asserts checkHardeningNginxRateZones passes
// against them — proving the generator and the doctor check actually agree,
// not just that each independently does what its own author expected.
func TestCheckHardeningNginxRateZones_RealGeneratorOutputPasses(t *testing.T) {
	dir := t.TempDir()

	cfg := &config.Config{BaseDomain: "example.com"}
	cfg, err := config.ApplyDefaults(cfg)
	if err != nil {
		t.Fatalf("ApplyDefaults() error: %v", err)
	}
	gen := nginx.NewGenerator(cfg, dir)
	files, err := gen.Generate()
	if err != nil {
		t.Fatalf("nginx.Generate() error: %v", err)
	}
	for relPath, content := range files {
		abs := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", relPath, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", relPath, err)
		}
	}

	// checkHardeningNginxRateZones only scans nginx/conf.d and nginx/sites
	// (plus nginx/nginx.conf) — the generator writes site confs under
	// nginx/sites/, which the sites-dir branch already covers.
	ctx := context.Background()
	res := checkHardeningNginxRateZones(ctx, dir)
	if res.Status != "pass" {
		t.Errorf("real nginx.Generate() output: status=%q, want pass; msg=%s", res.Status, res.Message)
	}
}

// TestCheckHardeningNginxRateZones_ServiceIdentity is the cli#379
// regression: detect rate limiting by server_name/service identity, not
// just the literal /auth/login and /api/ path substrings, while keeping
// the literal-path match as a fallback and never weakening the check
// (no limit_req anywhere must still fail; a limit_req in an unrelated
// server block must still fail).
func TestCheckHardeningNginxRateZones_ServiceIdentity(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string
		wantStatus string
	}{
		{
			// The exact shape cli#379 reported: one server{} block per
			// service, named by server_name, with limit_req only inside
			// the server-wide `location /` — no literal /auth/login or
			// /api/ path anywhere. Pre-fix this always failed.
			name: "generated-shape config: server_name-per-service, limit_req in location /",
			files: map[string]string{
				"auth.conf": `
server {
    listen 443 ssl;
    server_name auth.stack-two.nself.org;
    location / {
        limit_req zone=auth burst=5 nodelay;
        set $up_auth auth:4000;
        proxy_pass $up_auth;
    }
}
`,
				"hasura.conf": `
server {
    listen 443 ssl;
    server_name api.stack-two.nself.org;
    location / {
        limit_req zone=graphql_api burst=20 nodelay;
        set $up_hasura hasura:8080;
        proxy_pass $up_hasura;
    }
}
`,
			},
			wantStatus: "pass",
		},
		{
			// Hand-written gateway that routes by literal path instead of
			// by server_name — the original detection signal, kept as a
			// fallback and must keep passing.
			name: "literal-path legacy gateway config",
			files: map[string]string{
				"gateway.conf": `
limit_req_zone $binary_remote_addr zone=auth_login:10m rate=5r/m;
location /auth/login { limit_req zone=auth_login; }
limit_req_zone $binary_remote_addr zone=api:10m rate=30r/s;
location /api/ { limit_req zone=api; }
`,
			},
			wantStatus: "pass",
		},
		{
			name: "no limit_req anywhere",
			files: map[string]string{
				"default.conf": "server {\n    listen 80;\n    server_name example.com;\n    location / { proxy_pass http://app:3000; }\n}\n",
			},
			wantStatus: "fail",
		},
		{
			// limit_req exists, but only inside a server block whose
			// server_name identifies neither the auth nor the API
			// service, and no literal path fallback applies. Must not be
			// misread as satisfying either half.
			name: "limit_req only in an unrelated server block",
			files: map[string]string{
				"static.conf": `
server {
    listen 443 ssl;
    server_name static.stack-two.nself.org;
    location / {
        limit_req zone=static burst=10 nodelay;
        set $up_static frontend:3000;
        proxy_pass $up_static;
    }
}
`,
			},
			wantStatus: "fail",
		},
		{
			// Auth half satisfied by identity, API half present but not
			// actually rate-limited — must land on warn, not pass.
			name: "auth satisfied by identity, api block present but unlimited",
			files: map[string]string{
				"auth.conf": `
server {
    server_name auth.stack-two.nself.org;
    location / {
        limit_req zone=auth burst=5 nodelay;
        proxy_pass http://auth:4000;
    }
}
`,
				"hasura.conf": `
server {
    server_name api.stack-two.nself.org;
    location / {
        proxy_pass http://hasura:8080;
    }
}
`,
			},
			wantStatus: "warn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			confDir := filepath.Join(dir, "nginx", "sites")
			if err := os.MkdirAll(confDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for fname, content := range tt.files {
				if err := os.WriteFile(filepath.Join(confDir, fname), []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", fname, err)
				}
			}

			ctx := context.Background()
			res := checkHardeningNginxRateZones(ctx, dir)
			if res.Status != tt.wantStatus {
				t.Errorf("status=%q, want %q; msg=%s", res.Status, tt.wantStatus, res.Message)
			}
		})
	}
}

// ─── SEC-HARDENING-08: Encryption at rest ───────────────────────────────────

func TestCheckHardeningEncryptionAtRest_EnvSet(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.prod", "POSTGRES_DATA_ENCRYPTED=true")

	res := checkHardeningEncryptionAtRest(dir)
	if res.Status != "pass" {
		t.Errorf("POSTGRES_DATA_ENCRYPTED=true: status=%q, want pass; msg=%s", res.Status, res.Message)
	}
}

func TestCheckHardeningEncryptionAtRest_NotSet(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.dev", "# no encryption flag")

	res := checkHardeningEncryptionAtRest(dir)
	// On a CI runner without LUKS/dm-crypt, this will be warn.
	if res.Status == "fail" {
		t.Errorf("encryption not set: status=fail, want warn (fail is too strict for missing config)")
	}
	if res.Status != "warn" && res.Status != "pass" {
		t.Errorf("unexpected status %q", res.Status)
	}
}

func TestCheckHardeningEncryptionAtRest_AltEnvKey(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.prod", "NSELF_ENCRYPTION_AT_REST=yes")

	res := checkHardeningEncryptionAtRest(dir)
	if res.Status != "pass" {
		t.Errorf("NSELF_ENCRYPTION_AT_REST=yes: status=%q, want pass", res.Status)
	}
}

// ─── HardeningChecks aggregate ───────────────────────────────────────────────

func TestHardeningChecks_ReturnsEightResults(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	results := HardeningChecks(ctx, dir)
	// 8 SEC-HARDENING-* + 1 SEC-CORS-01 + 1 SEC-OFFLINE-01 + 1 SEC-DEVMODE-01 + 1 SEC-METRICS-01 = 12 total.
	if len(results) != 12 {
		t.Errorf("HardeningChecks returned %d results, want 12", len(results))
	}

	for _, r := range results {
		if r.Section != hardeningSection {
			t.Errorf("result %q: Section=%q, want %q", r.Name, r.Section, hardeningSection)
		}
		validStatuses := map[string]bool{"pass": true, "warn": true, "fail": true}
		if !validStatuses[r.Status] {
			t.Errorf("result %q: invalid Status=%q", r.Name, r.Status)
		}
		if r.Name == "" {
			t.Error("result has empty Name")
		}
		if r.Message == "" {
			t.Error("result has empty Message")
		}
	}
}

func TestHardeningChecks_CheckIDsDistinct(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	results := HardeningChecks(ctx, dir)
	seen := map[string]bool{}
	for _, r := range results {
		if seen[r.Name] {
			t.Errorf("duplicate check ID: %q", r.Name)
		}
		seen[r.Name] = true
	}
}

// ─── SEC-CORS-01: CORS domain ────────────────────────────────────────────────

func TestCheckCORSDomain_NonProd(t *testing.T) {
	dir := t.TempDir()
	// No NSELF_ENV / NODE_ENV set → non-prod → skip check → pass.
	writeEnvFile(t, dir, ".env.dev", "HASURA_GRAPHQL_CORS_DOMAIN=*")

	res := checkCORSDomain(dir)
	if res.Status != "pass" {
		t.Errorf("non-prod with wildcard: status=%q, want pass (skip); msg=%s", res.Status, res.Message)
	}
}

func TestCheckCORSDomain_ProdBareWildcard(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.prod",
		"NSELF_ENV=production",
		"HASURA_GRAPHQL_CORS_DOMAIN=*",
	)

	res := checkCORSDomain(dir)
	if res.Status != "fail" {
		t.Errorf("prod bare wildcard: status=%q, want fail; msg=%s", res.Status, res.Message)
	}
	if !strings.Contains(res.Message, "SEC-CORS-01") {
		t.Errorf("message missing SEC-CORS-01: %s", res.Message)
	}
}

func TestCheckCORSDomain_ProdSubdomainWildcard(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.prod",
		"NSELF_ENV=production",
		`HASURA_GRAPHQL_CORS_DOMAIN=https://*.example.com`,
	)

	res := checkCORSDomain(dir)
	if res.Status != "warn" {
		t.Errorf("prod subdomain wildcard: status=%q, want warn; msg=%s", res.Status, res.Message)
	}
}

func TestCheckCORSDomain_ProdExplicit(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.prod",
		"NSELF_ENV=staging",
		`HASURA_GRAPHQL_CORS_DOMAIN=https://app.example.com`,
	)

	res := checkCORSDomain(dir)
	if res.Status != "pass" {
		t.Errorf("prod explicit domain: status=%q, want pass; msg=%s", res.Status, res.Message)
	}
}

func TestCheckCORSDomain_ProdNotSet(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.prod", "NSELF_ENV=prod")

	res := checkCORSDomain(dir)
	if res.Status != "warn" {
		t.Errorf("prod no CORS domain: status=%q, want warn; msg=%s", res.Status, res.Message)
	}
}

// ─── SEC-OFFLINE-01: license cache freshness ─────────────────────────────────

// TestCheckLicenseOffline_NoCache verifies that a missing cache is a pass (fresh install).
func TestCheckLicenseOffline_NoCache(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(tmp, "does-not-exist.json"))

	res := checkLicenseOffline()
	if res.Status != "pass" {
		t.Errorf("no cache: status=%q, want pass; msg=%s", res.Status, res.Message)
	}
	if res.Name != "SEC-OFFLINE-01" {
		t.Errorf("Name=%q, want SEC-OFFLINE-01", res.Name)
	}
}

// TestCheckLicenseOffline_FreshCache verifies that a recently-fetched cache is a pass.
func TestCheckLicenseOffline_FreshCache(t *testing.T) {
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)

	// fetched_at = 1 hour ago
	fetchedAt := timeNowForTest().Add(-1 * time.Hour).Unix()
	content := []byte(`{"fetched_at":` + itoa(fetchedAt) + `}`)
	if err := os.WriteFile(cachePath, content, 0o600); err != nil {
		t.Fatalf("writing cache: %v", err)
	}

	res := checkLicenseOffline()
	if res.Status != "pass" {
		t.Errorf("fresh cache (1h): status=%q, want pass; msg=%s", res.Status, res.Message)
	}
}

// TestCheckLicenseOffline_StaleCache verifies that a cache >24h old is a warn.
func TestCheckLicenseOffline_StaleCache(t *testing.T) {
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)

	// fetched_at = 36 hours ago
	fetchedAt := timeNowForTest().Add(-36 * time.Hour).Unix()
	content := []byte(`{"fetched_at":` + itoa(fetchedAt) + `}`)
	if err := os.WriteFile(cachePath, content, 0o600); err != nil {
		t.Fatalf("writing cache: %v", err)
	}

	res := checkLicenseOffline()
	if res.Status != "warn" {
		t.Errorf("stale cache (36h): status=%q, want warn; msg=%s", res.Status, res.Message)
	}
	if !strings.Contains(res.Message, "unreachable") {
		t.Errorf("message missing 'unreachable': %s", res.Message)
	}
}

// TestCheckLicenseOffline_CorruptCache verifies that an unreadable cache is a warn.
func TestCheckLicenseOffline_CorruptCache(t *testing.T) {
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)

	if err := os.WriteFile(cachePath, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("writing cache: %v", err)
	}

	res := checkLicenseOffline()
	if res.Status != "warn" {
		t.Errorf("corrupt cache: status=%q, want warn; msg=%s", res.Status, res.Message)
	}
}

// timeNowForTest returns time.Now — a thin wrapper so test helpers are readable.
func timeNowForTest() time.Time { return time.Now() }

// itoa converts an int64 to its decimal string representation without importing strconv.
func itoa(n int64) string { return fmt.Sprintf("%d", n) }

// ─── SEC-DEVMODE-01: Hasura dev-mode not in staging/prod ─────────────────────

func TestCheckHasuraDevMode_NonProd_Skipped(t *testing.T) {
	dir := t.TempDir()
	// No NSELF_ENV set → non-prod → skip.
	res := checkHasuraDevMode(dir)
	if res.Status != "pass" {
		t.Errorf("no env set (non-prod): status=%q, want pass; msg=%s", res.Status, res.Message)
	}
	if res.Name != "SEC-DEVMODE-01" {
		t.Errorf("Name=%q, want SEC-DEVMODE-01", res.Name)
	}
}

func TestCheckHasuraDevMode_DevEnv_Skipped(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.dev", "ENV=dev", "HASURA_GRAPHQL_DEV_MODE=true")
	res := checkHasuraDevMode(dir)
	// dev env → skipped, even if dev-mode=true.
	if res.Status != "pass" {
		t.Errorf("dev env with dev-mode=true: status=%q, want pass; msg=%s", res.Status, res.Message)
	}
}

func TestCheckHasuraDevMode_ProdDevModeTrue_Fails(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.prod",
		"ENV=prod",
		"HASURA_GRAPHQL_DEV_MODE=true",
	)
	res := checkHasuraDevMode(dir)
	if res.Status != "fail" {
		t.Errorf("prod dev-mode=true: status=%q, want fail; msg=%s", res.Status, res.Message)
	}
	if !strings.Contains(res.Message, "SEC-DEVMODE-01") {
		t.Errorf("message missing SEC-DEVMODE-01: %s", res.Message)
	}
	if res.FixCmd == "" {
		t.Error("FixCmd should be non-empty")
	}
}

func TestCheckHasuraDevMode_ProdDevModeFalse_Passes(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.prod",
		"ENV=prod",
		"HASURA_GRAPHQL_DEV_MODE=false",
	)
	res := checkHasuraDevMode(dir)
	if res.Status != "pass" {
		t.Errorf("prod dev-mode=false: status=%q, want pass; msg=%s", res.Status, res.Message)
	}
}

func TestCheckHasuraDevMode_StagingDevModeTrue_Fails(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, ".env.staging",
		"ENV=staging",
		"HASURA_GRAPHQL_DEV_MODE=true",
	)
	res := checkHasuraDevMode(dir)
	if res.Status != "fail" {
		t.Errorf("staging dev-mode=true: status=%q, want fail; msg=%s", res.Status, res.Message)
	}
}

// ─── SEC-METRICS-01: Prometheus port binding ─────────────────────────────────

// TestCheckMetricsPortBinding_LoopbackInCompose verifies that a docker-compose.yml
// with 127.0.0.1:9090 returns pass.
func TestCheckMetricsPortBinding_LoopbackInCompose(t *testing.T) {
	dir := t.TempDir()
	content := `services:
  prometheus:
    image: prom/prometheus:v2.53.0
    ports:
      - "127.0.0.1:9090:9090"
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	res := checkMetricsPortBinding(dir)
	if res.Status != "pass" {
		t.Errorf("loopback binding: status=%q, want pass; msg=%s", res.Status, res.Message)
	}
	if res.Name != "SEC-METRICS-01" {
		t.Errorf("Name=%q, want SEC-METRICS-01", res.Name)
	}
}

// TestCheckMetricsPortBinding_WildcardInCompose verifies that a docker-compose.yml
// with 0.0.0.0:9090 returns fail.
func TestCheckMetricsPortBinding_WildcardInCompose(t *testing.T) {
	dir := t.TempDir()
	content := `services:
  prometheus:
    image: prom/prometheus:v2.53.0
    ports:
      - "0.0.0.0:9090:9090"
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	res := checkMetricsPortBinding(dir)
	if res.Status != "fail" {
		t.Errorf("wildcard binding: status=%q, want fail; msg=%s", res.Status, res.Message)
	}
	if !strings.Contains(res.Message, "0.0.0.0") {
		t.Errorf("message missing '0.0.0.0': %s", res.Message)
	}
	if res.FixCmd == "" {
		t.Error("FixCmd should be non-empty for fail status")
	}
}

// TestCheckMetricsPortBinding_NoPrometheus verifies that a compose file without
// a prometheus service does not incorrectly fail.
func TestCheckMetricsPortBinding_NoPrometheus(t *testing.T) {
	dir := t.TempDir()
	content := `services:
  postgres:
    image: postgres:16
    ports:
      - "5432:5432"
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	res := checkMetricsPortBinding(dir)
	// No prometheus block → falls through to docker exec check (will fail to connect)
	// and then returns pass (monitoring not deployed).
	validStatuses := map[string]bool{"pass": true, "warn": true}
	if !validStatuses[res.Status] {
		t.Errorf("no prometheus: status=%q, want pass or warn; msg=%s", res.Status, res.Message)
	}
}

// TestCheckMetricsPortBinding_NoComposeFile verifies graceful handling when
// docker-compose.yml is absent (monitoring not deployed).
func TestCheckMetricsPortBinding_NoComposeFile(t *testing.T) {
	dir := t.TempDir()
	// No docker-compose.yml — monitoring stack not present.
	res := checkMetricsPortBinding(dir)
	// Without compose file and without docker running, result should be pass.
	validStatuses := map[string]bool{"pass": true, "warn": true}
	if !validStatuses[res.Status] {
		t.Errorf("no compose file: status=%q, want pass or warn; msg=%s", res.Status, res.Message)
	}
}
