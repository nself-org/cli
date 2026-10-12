package compose

// images_lock_test.go — tests for the image lock (P7-LIVE-17, ADR 0030).
//
// Purpose: prove the embedded lock is sound, that legacy mode emits the exact
// pre-lock strings (the expectations below were captured from origin/main
// before the lock existed), that lock mode emits repository:version@digest
// equal to the lock entry, and that a user-set *_VERSION is honoured.
// Constraints: no network; the lock is the committed file.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

// init pins the package's tests to legacy image strings, so the many generator
// tests that assert pre-lock strings stay valid when the suite runs with
// NSELF_V15=1 (where the mode default is lock). Tests of the mode itself set
// IMAGE_PINNING explicitly.
func init() { _ = os.Setenv(EnvImagePinning, PinningLegacy) }

var lockedRefRE = regexp.MustCompile(`^[^@\s]+:[^@\s:]+@sha256:[0-9a-f]{64}$`)

func setMode(t *testing.T, mode string) { t.Helper(); t.Setenv(EnvImagePinning, mode) }

func TestLockIsSound(t *testing.T) {
	if err := LockError(); err != nil {
		t.Fatal(err)
	}
	imgs := LockedImages()
	names := make([]string, len(imgs))
	for i, r := range imgs {
		names[i] = r.Name
		// legacy_ref is frozen at the pre-lock string (a pin bump moves only
		// version), so it must name the same repository and carry a tag.
		if r.Role != RolePlugin && (r.LegacyVersion() == "" || shortRepo(r.LegacyRepository()) != shortRepo(r.Repository)) {
			t.Errorf("%s: legacy_ref %q is not <repository>:<tag> for %s", r.Name, r.LegacyRef, r.Repository)
		}
		if !lockedRefRE.MatchString(r.String()) {
			t.Errorf("%s: %q is not repository:version@sha256:<64 hex>", r.Name, r.String())
		}
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("LockedImages not sorted by name: %v", names)
	}
	for _, want := range strings.Fields("postgres pgvector hasura auth nginx redis minio storage functions mailpit admin mlflow meilisearch meilisearch-init typesense elasticsearch deno python lego tempo otel-collector seaweedfs prometheus grafana loki promtail alertmanager cadvisor node-exporter postgres-exporter redis-exporter otel-collector-contrib") {
		if _, ok := LockedRef(want); !ok {
			t.Errorf("lock has no %q entry", want)
		}
	}
	if r, _ := LockedRef("minio"); r.Repository != "docker.io/pgsty/minio" {
		t.Errorf("minio repository = %q, want docker.io/pgsty/minio (ADR 0028)", r.Repository)
	}
}

// shortRepo drops the docker.io/ and library/ prefixes Docker implies.
func shortRepo(repo string) string {
	return strings.TrimPrefix(strings.TrimPrefix(repo, "docker.io/"), "library/")
}

// TestImageForAfterPinBump: once a bump moves version past the frozen
// legacy_ref, the pre-lock default (what config defaults and .env files carry)
// must still resolve to the locked digest in lock mode and to legacy_ref in
// legacy mode; only another version is an unpinned override.
func TestImageForAfterPinBump(t *testing.T) {
	d := "sha256:" + strings.Repeat("b", 64)
	r := Ref{Name: "hasura", Repository: "docker.io/hasura/graphql-engine", Version: "v2.44.1", IndexDigest: d,
		LegacyRef: "hasura/graphql-engine:v2.44.0"}
	locked := "docker.io/hasura/graphql-engine:v2.44.1@" + d
	for _, tc := range []struct{ mode, version, want string }{
		{PinningLock, "", locked},
		{PinningLock, "v2.44.0", locked},
		{PinningLock, " v2.44.1 ", locked},
		{PinningLock, "v2.43.0", "hasura/graphql-engine:v2.43.0"},
		{PinningLegacy, "", "hasura/graphql-engine:v2.44.0"},
		{PinningLegacy, "v2.44.0", "hasura/graphql-engine:v2.44.0"},
		{PinningLegacy, "v2.44.1", "hasura/graphql-engine:v2.44.1"},
	} {
		if got := r.imageFor(tc.mode, tc.version); got != tc.want {
			t.Errorf("%s %q -> %q, want %q", tc.mode, tc.version, got, tc.want)
		}
	}
	if v := (Ref{LegacyRef: "busybox"}).LegacyVersion(); v != "" {
		t.Errorf("untagged legacy_ref has version %q", v)
	}
}

// TestConfigDefaultVersionsStayPinned: a documented *_VERSION default that is
// neither the lock version nor the legacy_ref version would make ImageRef emit
// an unpinned tag in lock mode. Guards the config literals against pin bumps.
func TestConfigDefaultVersionsStayPinned(t *testing.T) {
	setMode(t, PinningLock)
	keys := map[string]string{"elasticsearch": "ELASTICSEARCH_VERSION"}
	for _, e := range serviceCatalog {
		if e.VersionEnv != "" {
			name := e.Name
			if e.ImageKey != "" {
				name = e.ImageKey
			}
			keys[name] = e.VersionEnv
		}
	}
	for name, key := range keys {
		if name == "auth" { // auth renders the entry its mode picks (P7-LIVE-20)
			name = AuthEntry(nil)
		}
		r, ok := LockedRef(name)
		if !ok {
			continue
		}
		if def := config.DefaultFor(key); def != "" && ImageRef(name, def) != r.String() {
			t.Errorf("%s=%q (config default) is unpinned in lock mode: %s", key, def, ImageRef(name, def))
		}
	}
	for name, cfg := range fixtureConfigs(t) {
		for svc, v := range map[string]string{"hasura": cfg.Hasura.Version, AuthEntry(cfg.Passthrough): cfg.Auth.Version,
			"elasticsearch": cfg.Search.Elasticsearch.Version} {
			if r, _ := LockedRef(svc); ImageRef(svc, v) != r.String() {
				t.Errorf("%s: ApplyDefaults %s version %q is unpinned in lock mode", name, svc, v)
			}
		}
	}
}

func TestLockedImagesIsACopy(t *testing.T) {
	a := LockedImages()
	a[0].Name = "mutated"
	if LockedImages()[0].Name == "mutated" {
		t.Error("LockedImages must return a copy")
	}
}

func TestParseLockRejectsDrift(t *testing.T) {
	good, err := ParseLock(lockData)
	if err != nil {
		t.Fatal(err)
	}
	bad := *good
	bad.Generated = "hand edited"
	raw, _ := MarshalLock(&bad)
	if _, err := ParseLock(raw); err == nil {
		t.Error("a wrong _generated header must be rejected")
	}
	bad = *good
	bad.Images = append([]Ref(nil), good.Images...)
	bad.Images[0], bad.Images[1] = bad.Images[1], bad.Images[0]
	raw, _ = MarshalLock(&bad)
	if _, err := ParseLock(raw); err == nil {
		t.Error("unsorted entries must be rejected")
	}
	raw, _ = MarshalLock(good)
	if string(raw) != string(lockData) {
		t.Error("MarshalLock(ParseLock(lock)) must reproduce the committed lock byte for byte")
	}
}

func TestImageRefModes(t *testing.T) {
	r, _ := LockedRef("redis")
	for _, tc := range []struct{ mode, version, want string }{
		{PinningLegacy, "", "redis:7-alpine"},
		{PinningLegacy, "7-alpine", "redis:7-alpine"},
		{PinningLegacy, "6-alpine", "redis:6-alpine"},
		{PinningLock, "", r.String()},
		{PinningLock, "7-alpine", r.String()},       // equals the lock version: locked ref with digest
		{PinningLock, "6-alpine", "redis:6-alpine"}, // explicit override: used as given, unpinned
	} {
		setMode(t, tc.mode)
		if got := ImageRef("redis", tc.version); got != tc.want {
			t.Errorf("%s ImageRef(redis, %q) = %q, want %q", tc.mode, tc.version, got, tc.want)
		}
	}
	if ImageRef("no-such-image", "") != "" {
		t.Error("an unknown entry must yield the empty string")
	}
}

func TestPinningModeDefaultFollowsCompat(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		t.Setenv(EnvImagePinning, "")
		want := PinningLegacy
		if compat.V15() {
			want = PinningLock
		}
		if got := PinningMode(); got != want {
			t.Errorf("default mode = %s, want %s", got, want)
		}
		t.Setenv(EnvImagePinning, " LOCK ")
		if PinningMode() != PinningLock {
			t.Error("IMAGE_PINNING=lock must win in either compat mode")
		}
		t.Setenv(EnvImagePinning, "legacy")
		if PinningMode() != PinningLegacy {
			t.Error("IMAGE_PINNING=legacy must win in either compat mode")
		}
		t.Setenv(EnvImagePinning, "bogus")
		if got := PinningMode(); got != want {
			t.Errorf("invalid IMAGE_PINNING fell to %s, want the default %s", got, want)
		}
	})
}

func fixtureConfigs(t *testing.T) map[string]*config.Config {
	t.Helper()
	mk := func(mut func(c *config.Config)) *config.Config {
		c := minimalCfg()
		if mut != nil {
			mut(c)
		}
		// These fixtures assert the redis image strings; the cache engine default
		// is v1.5-dependent (P7-LIVE-19), so pin the engine and stay mode-independent.
		c.Redis.Engine = "redis"
		out, err := config.ApplyDefaults(c)
		if err != nil {
			t.Fatalf("ApplyDefaults: %v", err)
		}
		return out
	}
	prod := mk(func(c *config.Config) { c.Redis.Enabled = true })
	prod.Env = "prod"
	return map[string]*config.Config{
		"dev-minimal": mk(nil),
		"all-optional": mk(func(c *config.Config) {
			c.Redis.Enabled, c.Minio.Enabled, c.Mailpit.Enabled = true, true, true
			c.Functions.Enabled, c.Admin.Enabled, c.Search.Enabled = true, true, true
			c.Search.Engine = "meilisearch"
		}),
		"search-typesense-deno": mk(func(c *config.Config) {
			c.Search.Enabled, c.Search.Engine = true, "typesense"
			c.Functions.Enabled, c.Functions.Runtime = true, "deno"
		}),
		"python-pgvector": mk(func(c *config.Config) {
			c.Functions.Enabled, c.Functions.Runtime = true, "python"
			c.Postgres.Extensions = []string{"pgvector"}
		}),
		"prod": prod,
	}
}

// legacyExpected are the image strings origin/main emitted for each fixture,
// captured before the lock existed (sorted). One deliberate change since: auth
// is nhost/auth:0.36.0, the same index digest as nhost/hasura-auth:0.36.0 (the
// ungated rename, EPIC D17, P7-LIVE-20).
var legacyExpected = map[string][]string{
	"dev-minimal":           {"hasura/graphql-engine:v2.44.0", "nginx:alpine", "nhost/auth:0.36.0", "postgres:16-alpine"},
	"prod":                  {"hasura/graphql-engine:v2.44.0", "nginx:alpine", "nhost/auth:0.36.0", "postgres:16-alpine", "redis:7-alpine"},
	"python-pgvector":       {"hasura/graphql-engine:v2.44.0", "nginx:alpine", "nhost/auth:0.36.0", "pgvector/pgvector:pg16", "python:3.12-slim"},
	"search-typesense-deno": {"denoland/deno:alpine", "hasura/graphql-engine:v2.44.0", "nginx:alpine", "nhost/auth:0.36.0", "postgres:16-alpine", "typesense/typesense:27.1"},
	"all-optional": {"axllent/mailpit:latest", "busybox:1.36", "docker.io/pgsty/minio:latest", "getmeili/meilisearch:v1.6", "hasura/graphql-engine:v2.44.0",
		"nginx:alpine", "nhost/auth:0.36.0", "nhost/functions:latest", "nself/nself-admin:latest", "postgres:16-alpine", "redis:7-alpine"},
}

var imageLineRE = regexp.MustCompile(`(?m)^\s+image:\s*(\S+)`)

func generatedImages(t *testing.T, cfg *config.Config) []string {
	t.Helper()
	raw, err := NewGenerator(cfg).Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var out []string
	for _, m := range imageLineRE.FindAllStringSubmatch(string(raw), -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

func TestGeneratedImagesLegacyAreByteIdentical(t *testing.T) {
	setMode(t, PinningLegacy)
	for name, cfg := range fixtureConfigs(t) {
		if got, want := strings.Join(generatedImages(t, cfg), " "), strings.Join(legacyExpected[name], " "); got != want {
			t.Errorf("%s (legacy):\n got  %s\n want %s", name, got, want)
		}
	}
}

func TestGeneratedImagesLockModeEqualsLockedRef(t *testing.T) {
	setMode(t, PinningLock)
	byLegacy := map[string]Ref{}
	for _, r := range LockedImages() {
		byLegacy[r.LegacyRef] = r
	}
	for name, cfg := range fixtureConfigs(t) {
		var want []string
		for _, legacy := range legacyExpected[name] {
			r, ok := byLegacy[strings.TrimPrefix(legacy, "docker.io/pgsty/")]
			if !ok {
				r, ok = byLegacy[legacy]
			}
			if !ok {
				t.Fatalf("%s: no lock entry has legacy_ref %q", name, legacy)
			}
			want = append(want, r.String())
		}
		sort.Strings(want)
		got := generatedImages(t, cfg)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("%s (lock):\n got  %v\n want %v", name, got, want)
		}
		for _, img := range got {
			if !lockedRefRE.MatchString(img) {
				t.Errorf("%s: %q is not repository:version@sha256", name, img)
			}
		}
	}
}

func TestUserVersionOverrideTable(t *testing.T) {
	lock, _ := LockedRef("redis")
	for _, tc := range []struct{ mode, version, want string }{
		{PinningLock, lock.Version, lock.String()}, // equals the lock version: locked ref with digest
		{PinningLock, "6.2-alpine", "redis:6.2-alpine"},
		{PinningLegacy, lock.Version, "redis:7-alpine"},
		{PinningLegacy, "6.2-alpine", "redis:6.2-alpine"},
	} {
		setMode(t, tc.mode)
		cfg := minimalCfg()
		cfg.Redis = config.RedisConfig{Enabled: true, Engine: "redis", Version: tc.version, Port: 6379}
		if got := NewGenerator(cfg).buildRedisService().Image; got != tc.want {
			t.Errorf("%s REDIS_VERSION=%q -> %q, want %q", tc.mode, tc.version, got, tc.want)
		}
	}
}

func TestResolveImageLeavesDigestRefsAlone(t *testing.T) {
	setMode(t, PinningLock)
	ImageDigests = map[string]string{"nginx": strings.Repeat("a", 64)}
	t.Cleanup(func() { ImageDigests = map[string]string{} })
	r, _ := LockedRef("nginx")
	if got := ResolveImage("nginx", ImageRef("nginx", "")); got != r.String() {
		t.Errorf("ResolveImage double-pinned a locked ref: %q", got)
	}
}

func TestDefaultImagePostgresIsPgvector(t *testing.T) {
	setMode(t, PinningLegacy)
	if got := DefaultImage("postgres"); got != "pgvector/pgvector:pg16" {
		t.Errorf("DefaultImage(postgres) = %q, want the pre-lock pgvector pin", got)
	}
}

// TestEveryImageNameLiteralIsLocked: ImageRef returns "" for an unknown name,
// so every name literal handed to ImageRef or LockedRef in non-test Go must
// exist in the lock, including paths no fixture exercises (tempo, otel).
func TestEveryImageNameLiteralIsLocked(t *testing.T) {
	re := regexp.MustCompile(`(?:ImageRef|LockedRef|DefaultImage|ResolveImage)\("([a-z0-9-]+)"`)
	seen := 0
	for _, root := range []string{"../../internal", "../../cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			var code []string
			for _, line := range strings.Split(string(b), "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "//") {
					code = append(code, line)
				}
			}
			for _, m := range re.FindAllStringSubmatch(strings.Join(code, "\n"), -1) {
				seen++
				name := m[1]
				if name == "postgres" && strings.Contains(m[0], "DefaultImage") {
					continue // aliased to the pgvector entry
				}
				if _, ok := LockedRef(name); !ok {
					t.Errorf("%s passes %q, which is not a lock entry", path, name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if seen < 15 {
		t.Errorf("found only %d name literals; the pattern drifted from the call sites", seen)
	}
}
