package main

// imagelock_test.go — offline tests for tools/imagelock (no network).
//
// Purpose: cover Resolve (fake indexer), Check, MergePlugins and the committed
// lock against the committed images.yaml.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
)

// D-0264: re-resolving these images must leave a reviewable release in the
// diff, while legacy mode continues emitting the original floating references.
func TestPreviouslyFloatingImagesUseReleaseTags(t *testing.T) {
	raw, err := os.ReadFile("../../internal/compose/images.yaml")
	if err != nil {
		t.Fatal(err)
	}
	sources, err := LoadSources(raw)
	if err != nil {
		t.Fatal(err)
	}
	legacy := map[string]string{
		"admin":          "nself/nself-admin:latest",
		"functions":      "nhost/functions:latest",
		"mailpit":        "axllent/mailpit:latest",
		"minio":          "docker.io/pgsty/minio:latest",
		"otel-collector": "otel/opentelemetry-collector-contrib:latest",
		"tempo":          "grafana/tempo:latest",
		"python":         "python:3.12-slim",
	}
	release := regexp.MustCompile(`^(v?[0-9]+\.[0-9]+\.[0-9]+|[0-9]+-[0-9]+\.[0-9]+\.[0-9]+|RELEASE\.[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}-[0-9]{2}-[0-9]{2}Z)$`)
	python := regexp.MustCompile(`^3\.12\.[0-9]+-slim$`)
	for _, s := range sources {
		wantLegacy, ok := legacy[s.Name]
		if !ok {
			continue
		}
		pattern := release
		if s.Name == "python" {
			pattern = python
		}
		if !pattern.MatchString(s.Version) {
			t.Errorf("%s: version %q must name a concrete release", s.Name, s.Version)
		}
		if s.LegacyRef != wantLegacy {
			t.Errorf("%s: legacy_ref = %q, want %q", s.Name, s.LegacyRef, wantLegacy)
		}
		delete(legacy, s.Name)
	}
	for name := range legacy {
		t.Errorf("missing image %s", name)
	}
}

// fakeIndexer serves one canned index per repository, for any tag or digest.
// The registry digest it reports is digestOf(raw) unless digest overrides it
// for that repository. rawSuffix is appended to the served bytes (as buildx
// does with a trailing newline). failBack makes digest-addressed reads fail.
type fakeIndexer struct {
	raw       map[string]string
	fail      map[string]bool
	digest    map[string]string
	rawSuffix string
	failBack  bool
}

func (f fakeIndexer) Index(_ context.Context, ref string) ([]byte, string, error) {
	repo := ref
	if i := strings.Index(repo, "@"); i >= 0 {
		repo = repo[:i]
		if f.failBack {
			return nil, "", fmt.Errorf("registry says no such manifest")
		}
	}
	if i := strings.LastIndex(repo, ":"); i > strings.LastIndex(repo, "/") {
		repo = repo[:i]
	}
	if f.fail[repo] {
		return nil, "", fmt.Errorf("registry says no such manifest")
	}
	dig := f.digest[repo]
	if dig == "" {
		dig = digestOf(f.raw[repo])
	}
	return []byte(f.raw[repo] + f.rawSuffix), dig, nil
}

func indexJSON(archs ...string) string {
	var ms []string
	for i, a := range archs {
		ms = append(ms, fmt.Sprintf(`{"digest":"sha256:%064x","platform":{"os":"linux","architecture":%q}}`, i+1, a))
	}
	return `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[` + strings.Join(ms, ",") + `]}`
}

func digestOf(raw string) string {
	s := sha256.Sum256([]byte(raw))
	return "sha256:" + hex.EncodeToString(s[:])
}

func TestResolveBuildsSortedLock(t *testing.T) {
	src := []Source{
		{Name: "zeta", Role: "optional", Repository: "docker.io/lib/zeta", Version: "1", LegacyRef: "zeta:1", Platforms: []string{"linux/amd64", "linux/arm64"}},
		{Name: "alpha", Role: "core", Repository: "docker.io/lib/alpha", Version: "2", LegacyRef: "alpha:2"},
	}
	both, one := indexJSON("amd64", "arm64"), indexJSON("amd64")
	lf, err := Resolve(context.Background(), src, fakeIndexer{raw: map[string]string{"docker.io/lib/zeta": both, "docker.io/lib/alpha": one}})
	if err != nil {
		t.Fatal(err)
	}
	if lf.Images[0].Name != "alpha" || lf.Images[1].Name != "zeta" {
		t.Errorf("lock entries must be sorted by name, got %s, %s", lf.Images[0].Name, lf.Images[1].Name)
	}
	for _, r := range lf.Images {
		want := digestOf(map[string]string{"zeta": both, "alpha": one}[r.Name])
		if r.IndexDigest != want {
			t.Errorf("%s digest = %s, want %s", r.Name, r.IndexDigest, want)
		}
		if r.Mirror == nil || *r.Mirror != "docker.io/nself/"+r.Name {
			t.Errorf("%s mirror = %v", r.Name, r.Mirror)
		}
	}
}

// buildx prints the raw index with a trailing newline and the registry digest
// is not always the hash of those bytes: the lock must record the registry's.
func TestResolveRecordsRegistryDigest(t *testing.T) {
	const repo = "docker.io/lib/x"
	s := Source{Name: "x", Role: "core", Repository: repo, Version: "1", LegacyRef: "x:1"}
	raw := indexJSON("amd64")
	registry := "sha256:" + strings.Repeat("4b", 32)
	lf, err := Resolve(context.Background(), []Source{s}, fakeIndexer{
		raw: map[string]string{repo: raw}, digest: map[string]string{repo: registry}, rawSuffix: "\n"})
	if err != nil {
		t.Fatal(err)
	}
	if got := lf.Images[0].IndexDigest; got != registry {
		t.Errorf("IndexDigest = %s, want the registry digest %s", got, registry)
	}
	if lf.Images[0].IndexDigest == digestOf(raw) {
		t.Error("the lock must not record a hash of the printed bytes")
	}
}

func TestResolveFailsWhenDigestReadBackFails(t *testing.T) {
	const repo = "docker.io/lib/x"
	s := Source{Name: "x", Role: "core", Repository: repo, Version: "1", LegacyRef: "x:1"}
	_, err := Resolve(context.Background(), []Source{s}, fakeIndexer{raw: map[string]string{repo: indexJSON("amd64")}, failBack: true})
	if err == nil || !strings.Contains(err.Error(), "does not address the index") {
		t.Errorf("a digest that cannot be read back must fail, got %v", err)
	}
}

func TestResolveErrors(t *testing.T) {
	ctx := context.Background()
	s := Source{Name: "x", Role: "core", Repository: "docker.io/lib/x", Version: "1", LegacyRef: "x:1", Platforms: []string{"linux/arm64"}}
	if _, err := Resolve(ctx, []Source{s}, fakeIndexer{raw: map[string]string{"docker.io/lib/x": indexJSON("amd64")}}); err == nil || !strings.Contains(err.Error(), "linux/arm64") {
		t.Errorf("a missing required platform must fail naming it, got %v", err)
	}
	single := `{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json"}`
	if _, err := Resolve(ctx, []Source{s}, fakeIndexer{raw: map[string]string{"docker.io/lib/x": single}}); err == nil || !strings.Contains(err.Error(), "index") {
		t.Errorf("a single manifest must fail as not an index, got %v", err)
	}
	if _, err := Resolve(ctx, []Source{s}, fakeIndexer{fail: map[string]bool{"docker.io/lib/x": true}}); err == nil {
		t.Error("a reference that no longer resolves must fail")
	}
	s.Digest = "sha256:" + strings.Repeat("9", 64)
	s.Platforms = nil
	if _, err := Resolve(ctx, []Source{s}, fakeIndexer{raw: map[string]string{"docker.io/lib/x": indexJSON("amd64")}}); err == nil || !strings.Contains(err.Error(), "pins") {
		t.Errorf("a pinned digest that differs from the registry must fail, got %v", err)
	}
}

func TestMergePluginsKinds(t *testing.T) {
	raw, err := os.ReadFile("testdata/images.json")
	if err != nil {
		t.Fatal(err)
	}
	merged, err := MergePlugins([]Source{{Name: "redis", Role: "optional", Repository: "docker.io/library/redis", Version: "7", LegacyRef: "redis:7"}}, raw)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Source{}
	for _, s := range merged {
		got[s.Name] = s
	}
	if len(merged) != 3 {
		t.Fatalf("merged %d entries (%v), want redis + plugin/claw + traefik; ci must be ignored", len(merged), got)
	}
	if p := got["plugin/claw"]; p.Role != "plugin" || p.Repository != "docker.io/nself/plugin-claw" || p.Version != "1.2.1" || !strings.HasPrefix(p.Digest, "sha256:1111") {
		t.Errorf("plugin entry = %+v", p)
	}
	if u := got["traefik"]; u.Role != "optional" || u.Version != "v3.1" {
		t.Errorf("upstream entry = %+v", u)
	}
	if _, ok := got["ci-runner"]; ok {
		t.Error("kind ci must be ignored")
	}
	if _, err := MergePlugins(merged, raw); err == nil {
		t.Error("merging the same names twice must fail")
	}
	if _, err := MergePlugins(nil, []byte(`{"schema":"nself.plugins.images/v1","images":{"a":{"kind":"plugin","image":"a:1"}}}`)); err == nil {
		t.Error("an image without @sha256 must fail")
	}
}

func TestPluginMergeResolvesToPluginRole(t *testing.T) {
	raw, _ := os.ReadFile("testdata/images.json")
	merged, err := MergePlugins(nil, raw)
	if err != nil {
		t.Fatal(err)
	}
	idx := map[string]string{}
	for _, s := range merged {
		idx[s.Repository] = indexJSON("amd64", "arm64")
		// the fixture digests are placeholders, so resolve without the pin check
	}
	for i := range merged {
		merged[i].Digest = ""
	}
	lf, err := Resolve(context.Background(), merged, fakeIndexer{raw: idx})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range lf.Images {
		if r.Name == "plugin/claw" && r.Mirror != nil {
			t.Error("a plugin entry has mirror null")
		}
		if r.Name == "traefik" && r.Mirror == nil {
			t.Error("an upstream entry has a mirror")
		}
	}
}

func TestCommittedLockMatchesImagesYAML(t *testing.T) {
	y, err := os.ReadFile("../../internal/compose/images.yaml")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := os.ReadFile("../../internal/compose/images.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	sources, err := LoadSources(y)
	if err != nil {
		t.Fatal(err)
	}
	if problems := Check(lock, sources); len(problems) > 0 {
		t.Fatalf("-check problems: %v", problems)
	}
	// An entry added to images.yaml without resolving must fail -check.
	extra := append(sources, Source{Name: "brand-new", Role: "optional", Repository: "docker.io/x/new", Version: "1", LegacyRef: "x/new:1"})
	if problems := Check(lock, extra); len(problems) != 1 || !strings.Contains(problems[0], "brand-new") {
		t.Errorf("an unresolved entry must fail -check, got %v", problems)
	}
	// A moved version without resolving is stale too.
	moved := append([]Source(nil), sources...)
	moved[0].Version += "-bumped"
	if problems := Check(lock, moved); len(problems) == 0 {
		t.Error("a version changed in images.yaml without -resolve must fail -check")
	}
	if problems := Check([]byte(`{"_generated":"x"}`), sources); len(problems) == 0 {
		t.Error("a malformed lock must fail -check")
	}
	if _, err := compose.ParseLock(lock); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSourcesStrict(t *testing.T) {
	if _, err := LoadSources([]byte("schema_version: 1\nimages:\n  - {name: a, role: core, repository: r, version: v, legacy_ref: r:v, bogus: 1}\n")); err == nil {
		t.Error("an unknown key must be rejected")
	}
	if _, err := LoadSources([]byte("schema_version: 1\nimages:\n  - {name: a, role: core, repository: r, version: v, legacy_ref: r:v}\n  - {name: a, role: core, repository: r, version: v, legacy_ref: r:v}\n")); err == nil {
		t.Error("a duplicate name must be rejected")
	}
}
