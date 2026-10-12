package plugin

// Tests for the exported strict helpers `nself update` uses (P7-LIVE-10).

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// ParseRegistry parses a good body and rejects a bad one without any network.
func TestParseRegistryPublic(t *testing.T) {
	reg, err := ParseRegistry([]byte(`{"version":"1","plugins":[{"name":"ai","version":"2.0.0"}]}`))
	if err != nil || len(reg.Plugins) != 1 || reg.Plugins[0].Version != "2.0.0" {
		t.Fatalf("good body: reg=%+v err=%v", reg, err)
	}
	if reg, err := ParseRegistry([]byte(`{"plugins": [oops`)); err == nil {
		t.Fatalf("a malformed body parsed: %+v", reg)
	}
}

// WriteRegistryCache writes a 0600 registry.json that FetchRegistry then reads
// as a fresh cache, ahead of an older file that was there before.
func TestWriteRegistryCacheOverridesStale(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "registry.json")
	if err := os.WriteFile(stale, []byte(`{"version":"1","plugins":[{"name":"ai","version":"1.0.0"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(stale, old, old)
	want := &Registry{Plugins: []PluginManifest{{Name: "ai", Version: "2.0.0"}}}
	if err := WriteRegistryCache(dir, want); err != nil {
		t.Fatal(err)
	}
	// Windows has no POSIX permission bits (it reads back 0666).
	if runtime.GOOS != "windows" {
		if st, err := os.Stat(stale); err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("cache mode = %v err=%v, want 0600", st.Mode().Perm(), err)
		}
	}
	got, err := FetchRegistry(context.Background(), "", dir) // fresh cache: no network
	if err != nil || len(got.Plugins) != 1 || got.Plugins[0].Version != "2.0.0" {
		t.Fatalf("FetchRegistry after prime: %+v err=%v", got, err)
	}
	if err := WriteRegistryCache("", want); err == nil {
		t.Fatal("empty cache dir must be an error")
	}
	if err := WriteRegistryCache(dir, nil); err == nil {
		t.Fatal("nil registry must be an error")
	}
}
