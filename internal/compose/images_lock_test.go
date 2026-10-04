package compose

// images_lock_test.go — tests for the image lock (P7-LIVE-17, ADR 0030).
//
// Purpose: prove the embedded lock is sound, that legacy mode emits the exact
// pre-lock strings (the expectations below were captured from origin/main
// before the lock existed), that lock mode emits repository:version@digest
// equal to the lock entry, and that a user-set *_VERSION is honoured.
// Constraints: no network; the lock is the committed file.

import (
	"os"
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
		if r.Role != RolePlugin && r.LegacyRepository()+":"+r.Version != r.LegacyRef {
			t.Errorf("%s: legacy_ref %q is not <repo>:<version> (%s:%s)", r.Name, r.LegacyRef, r.LegacyRepository(), r.Version)
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
// captured before the lock existed (sorted).
var legacyExpected = map[string][]string{
	"dev-minimal":           {"hasura/graphql-engine:v2.44.0", "nginx:alpine", "nhost/hasura-auth:0.36.0", "postgres:16-alpine"},
	"prod":                  {"hasura/graphql-engine:v2.44.0", "nginx:alpine", "nhost/hasura-auth:0.36.0", "postgres:16-alpine", "redis:7-alpine"},
	"python-pgvector":       {"hasura/graphql-engine:v2.44.0", "nginx:alpine", "nhost/hasura-auth:0.36.0", "pgvector/pgvector:pg16", "python:3.12-slim"},
	"search-typesense-deno": {"denoland/deno:alpine", "hasura/graphql-engine:v2.44.0", "nginx:alpine", "nhost/hasura-auth:0.36.0", "postgres:16-alpine", "typesense/typesense:27.1"},
	"all-optional": {"axllent/mailpit:latest", "busybox:1.36", "docker.io/pgsty/minio:latest", "getmeili/meilisearch:v1.6", "hasura/graphql-engine:v2.44.0",
		"nginx:alpine", "nhost/functions:latest", "nhost/hasura-auth:0.36.0", "nself/nself-admin:latest", "postgres:16-alpine", "redis:7-alpine"},
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
		cfg.Redis = config.RedisConfig{Enabled: true, Version: tc.version, Port: 6379}
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
