package bundle

// installer_helpers_ledger_test.go — D-0475: the one-pass plugin directory scan
// behind the bootstrap source. A manifest that parses but fails validation is
// recorded by name with a note (Sources.Invalid); only an unreadable or
// unparsable one, or one with no usable name, is Unreadable. LedgerSources is
// the same source.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle/ledger"
)

func writeRawManifest(t *testing.T, dir, sub, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sub, "plugin.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestScanPluginDirClassifies: valid, invalid-but-parsable, unparsable and
// bad-name manifests land in Plugins, Plugins+Invalid, Unreadable, Unreadable.
func TestScanPluginDirClassifies(t *testing.T) {
	dir := t.TempDir()
	writeMember(t, dir, "notifications", 3999, nil)
	writeRawManifest(t, dir, "nodesc", `{"name":"nodesc","version":"2.0.0","tier":"pro","checksum":"abc"}`)
	writeRawManifest(t, dir, "broken", "{not json")
	writeRawManifest(t, dir, "evil", `{"name":"../x","version":"1.0.0"}`)
	writeRawManifest(t, dir, "noname", `{"version":"1.0.0"}`)
	if err := os.MkdirAll(filepath.Join(dir, ".nself-state"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "empty-dir"), 0o755); err != nil {
		t.Fatal(err)
	}

	plugins, unreadable, invalid, err := scanPluginDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ledger.InstalledPlugin{}
	for _, p := range plugins {
		got[p.Name] = p
	}
	if len(got) != 2 || got["notifications"].Name == "" || got["nodesc"].Version != "2.0.0" || got["nodesc"].Tier != ledger.TierLicensed {
		t.Errorf("plugins = %+v, want notifications and nodesc (licensed, raw fields)", plugins)
	}
	if len(invalid) != 1 || !strings.HasPrefix(invalid[0], "nodesc: ") || !strings.HasSuffix(invalid[0], "; recorded by name") {
		t.Errorf("invalid = %q, want one nodesc note", invalid)
	}
	if len(unreadable) != 3 {
		t.Fatalf("unreadable = %q, want broken, evil and noname", unreadable)
	}
	for _, want := range []string{"broken: ", "evil: ", "noname: "} {
		found := false
		for _, u := range unreadable {
			found = found || strings.HasPrefix(u, want)
		}
		if !found {
			t.Errorf("unreadable %q lacks %s", unreadable, want)
		}
	}
}

// TestBootstrapParsableInvalidManifestPersists: a manifest without "description"
// does not block a persisted bootstrap; the plugin is recorded and the note is
// both in Sources.Invalid and surfaced through Store.Warn.
func TestBootstrapParsableInvalidManifestPersists(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	writeRawManifest(t, dir, "nodesc", `{"name":"nodesc","version":"1.0.0","category":"x","license":"MIT"}`)
	src, err := bootstrapSources(dir)()
	if err != nil {
		t.Fatal(err)
	}
	if len(src.Unreadable) != 0 || len(src.Invalid) != 1 {
		t.Fatalf("unreadable = %q invalid = %q, want none and one", src.Unreadable, src.Invalid)
	}
	st := newLedgerStore(dir)
	var warned []string
	st.Warn = func(m string) { warned = append(warned, m) }
	if _, err := st.EnsureBootstrapped(); err != nil {
		t.Fatalf("EnsureBootstrapped = %v, want success", err)
	}
	l := loadLedger(t, dir)
	if _, ok := l.Plugins["nodesc"]; !ok {
		t.Errorf("ledger plugins = %+v, want nodesc recorded", l.Plugins)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "fails validation and is recorded from its raw fields") {
		t.Errorf("warnings = %q, want the validation note", warned)
	}
}

// TestBootstrapUnparsableManifestStillRefused: unparsable JSON and a bad name
// keep blocking a persisted bootstrap.
func TestBootstrapUnparsableManifestStillRefused(t *testing.T) {
	for sub, body := range map[string]string{"broken": "{not json", "evil": `{"name":"../x"}`} {
		isolate(t)
		dir := t.TempDir()
		writeRawManifest(t, dir, sub, body)
		if _, err := newLedgerStore(dir).EnsureBootstrapped(); err == nil || !strings.Contains(err.Error(), sub) {
			t.Errorf("%s: EnsureBootstrapped = %v, want a refusal naming it", sub, err)
		}
	}
}

// TestLedgerSourcesIsStrict: LedgerSources reports a bad manifest in Unreadable,
// carries every catalogue bundle (Names, not only DisplayOrder), and the
// read-only Load still works and warns.
func TestLedgerSourcesIsStrict(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	writeMember(t, dir, "notifications", 3999, nil)
	writeRawManifest(t, dir, "broken", "{not json")
	src, err := LedgerSources(dir)()
	if err != nil {
		t.Fatal(err)
	}
	if len(src.Unreadable) != 1 || !strings.HasPrefix(src.Unreadable[0], "broken: ") {
		t.Errorf("unreadable = %q, want broken", src.Unreadable)
	}
	if len(src.Bundles) != len(Names()) {
		t.Errorf("bundles = %d, want the full catalogue (%d)", len(src.Bundles), len(Names()))
	}
	st := ledger.NewStore(dir)
	st.Sources = LedgerSources(dir)
	var warned []string
	st.Warn = func(m string) { warned = append(warned, m) }
	if _, persisted, err := st.Load(); err != nil || persisted {
		t.Fatalf("Load = persisted %v, %v; want a read-only view", persisted, err)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "broken") {
		t.Errorf("warnings = %q, want one naming broken", warned)
	}
}
