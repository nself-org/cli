package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin/requires"
)

// stubProbe answers requires.Check for installLocked tests.
type stubProbe struct {
	running bool
	have    map[string]bool
}

func (s stubProbe) Running(context.Context) (bool, error) { return s.running, nil }
func (s stubProbe) Available(context.Context) (map[string]bool, error) {
	return s.have, nil
}

// requiresFixture serves testdata/requires/registry.json and counts download
// requests (the tarball endpoint).
func requiresFixture(t *testing.T) *int32 {
	t.Helper()
	body, err := os.ReadFile("testdata/requires/registry.json")
	if err != nil {
		t.Fatal(err)
	}
	var downloads int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tarball") {
			atomic.AddInt32(&downloads, 1)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	return &downloads
}

// TestInstallLockedRequiresExtensions: a manifest requiring `vector` stops
// before download on an alpine cluster and proceeds on a pgvector cluster.
func TestInstallLockedRequiresExtensions(t *testing.T) {
	downloads := requiresFixture(t)
	cfg := &config.Config{ProjectName: "t"}
	t.Cleanup(func() { requires.DefaultProbe = nil })

	requires.DefaultProbe = stubProbe{running: true, have: map[string]bool{"plpgsql": true}}
	err := installLocked(context.Background(), cfg, "vecplug", t.TempDir())
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E507" || !strings.Contains(ce.Fix, "nself db image switch --to pgvector") {
		t.Fatalf("alpine fixture: want E507 with the switch fix, got %v", err)
	}
	if n := atomic.LoadInt32(downloads); n != 0 {
		t.Fatalf("download hit %d times before the refusal", n)
	}

	requires.DefaultProbe = stubProbe{running: true, have: map[string]bool{"vector": true}}
	err = installLocked(context.Background(), cfg, "vecplug", t.TempDir())
	if errors.As(err, &ce) && ce.Code == "E507" {
		t.Fatalf("pgvector fixture refused: %v", err)
	}
	if n := atomic.LoadInt32(downloads); n == 0 {
		t.Fatalf("pgvector fixture never reached the download step (err=%v)", err)
	}

	// A plugin with no requirement never consults the database.
	requires.DefaultProbe = stubProbe{running: true, have: nil}
	before := atomic.LoadInt32(downloads)
	_ = installLocked(context.Background(), cfg, "plainplug", t.TempDir())
	if atomic.LoadInt32(downloads) == before {
		t.Error("plainplug must proceed to download")
	}
}

// TestRegistryCarriesPostgresExtensions: catalog.requires reaches the runtime
// manifest, survives the cache round trip, and a registry without it is
// unchanged.
func TestRegistryCarriesPostgresExtensions(t *testing.T) {
	body, err := os.ReadFile("testdata/requires/registry.json")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := parseRegistryJSON(body)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for i := range reg.Plugins {
		got[reg.Plugins[i].Name] = reg.Plugins[i].RequiresPostgresExtensions()
	}
	if len(got["vecplug"]) != 1 || got["vecplug"][0] != "vector" || len(got["plainplug"]) != 0 {
		t.Fatalf("parsed = %v", got)
	}
	data, err := reg.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	again, err := parseRegistryJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	for i := range again.Plugins {
		if again.Plugins[i].Name == "vecplug" && len(again.Plugins[i].RequiresPostgresExtensions()) != 1 {
			t.Fatal("requirement lost in the cache round trip")
		}
	}
}

// TestLoadManifestV2PostgresExtensions: the v2 adapter copies
// requires.postgres_extensions onto the runtime manifest.
func TestLoadManifestV2PostgresExtensions(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(v2FixtureDir, "full.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	req, _ := doc["requires"].(map[string]any)
	if req == nil {
		req = map[string]any{}
	}
	req["postgres_extensions"] = []any{"vector"}
	doc["requires"] = req
	out, _ := json.Marshal(doc)
	m, err := LoadManifest(writeV2Manifest(t, out))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.RequiresPostgresExtensions(); len(got) != 1 || got[0] != "vector" {
		t.Fatalf("RequiresPostgresExtensions = %v", got)
	}
}
