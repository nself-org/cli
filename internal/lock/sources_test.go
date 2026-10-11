package lock

// sources_test.go — tests for the compose source readers, LoadSources, the lock
// validation table and the small helpers (P7-LIVE-09). No network; files only
// under t.TempDir.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestComposeSourcesMultiDocument: Compose merges every YAML document of a file
// and ComposeSources reads only the first, so a second document marks the file
// opaque (adversarial r2 N1). An empty trailing document counts too: it is not
// worth a special case on a path that must fail closed.
func TestComposeSourcesMultiDocument(t *testing.T) {
	cases := []struct {
		name, yaml string
		multi      bool
	}{
		{"single", "services:\n  a:\n    image: x:1\n", false},
		{"empty file", "", false},
		{"comment only", "# nothing\n", false},
		{"leading marker only", "---\nservices:\n  a:\n    image: x:1\n", false},
		{"image in second document", "services: {}\n---\nservices:\n  hasura:\n    image: evil/hasura:1\n", true},
		{"build in second document", "services: {}\n---\nservices:\n  hasura:\n    build: ./evil\n", true},
		{"empty second document", "services:\n  a:\n    image: x:1\n---\n", true},
		{"two empty documents", "---\n---\nservices: {}\n", true},
	}
	for _, c := range cases {
		svcs, _, multi, err := ComposeSources([]byte(c.yaml))
		if err != nil || multi != c.multi {
			t.Errorf("%s: multi=%v err=%v, want multi=%v", c.name, multi, err, c.multi)
		}
		if c.name == "single" && svcs["a"].Image != "x:1" {
			t.Errorf("single: first document not read: %v", svcs)
		}
	}
	if _, _, _, err := ComposeSources([]byte("services: {}\n---\nservices: [\n")); err != nil {
		t.Errorf("a broken second document is reported as multi-document, not a parse error: %v", err)
	}
}

// TestMergeDetailedMultiDocument: a second document in any file is reported
// unverifiable and names the file, even when the service is not CLI-owned
// (the document could hold anything).
func TestMergeDetailedMultiDocument(t *testing.T) {
	owned := func(s string) bool { return s == "hasura" }
	for _, y := range []string{
		"services: {}\n---\nservices:\n  hasura:\n    image: evil/hasura:1\n",
		"services: {}\n---\nservices:\n  hasura:\n    build: ./evil\n",
	} {
		m, err := MergeDetailed([]SourceFile{
			{Name: "docker-compose.yml", Data: []byte("services:\n  hasura:\n    image: h/h:1\n")},
			{Name: "docker-compose.override.yml", Data: []byte(y)},
		}, owned, true)
		if err != nil || len(m.Unverifiable) != 1 ||
			!strings.Contains(m.Unverifiable[0], "more than one YAML document in docker-compose.override.yml") ||
			!strings.Contains(m.Unverifiable[0], "cannot be verified") {
			t.Errorf("multi-document override: %v %v", m.Unverifiable, err)
		}
	}
}

func TestCLIMinor(t *testing.T) {
	for in, want := range map[string]string{
		"1.5.0": "1.5", "v1.5.2": "1.5", " 2.10.3-rc1 ": "2.10", "1.5": "1.5",
		"": "", "dev": "", "v1": "", "x1.5": "",
	} {
		if got := CLIMinor(in); got != want {
			t.Errorf("CLIMinor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRefDigestForms(t *testing.T) {
	for in, want := range map[string]string{
		"r/x:1@" + digA: digA, "r/x@" + digB: digB, "r/x:1": "", "r/x:1@sha256:short": "", "": "",
	} {
		if got := RefDigest(in); got != want {
			t.Errorf("RefDigest(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestLoadSources: order, base replacement, skipped missing files, the mirror
// flag, and the errors.
func TestLoadSources(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("docker-compose.yml", "disk base")
	write("docker-compose.override.yml", "override")
	write("mirror.yml", "mirror")
	abs := filepath.Join(dir, "docker-compose.override.yml")

	files, err := LoadSources(dir, nil, []string{"docker-compose.override.yml", "missing.yml", "mirror.yml"}, "mirror.yml")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range files {
		got = append(got, f.Name+"="+string(f.Data))
		if f.Mirror != (f.Name == "mirror.yml") {
			t.Errorf("%s: Mirror = %v", f.Name, f.Mirror)
		}
	}
	if want := "docker-compose.yml=disk base,docker-compose.override.yml=override,mirror.yml=mirror"; strings.Join(got, ",") != want {
		t.Errorf("files = %v, want %s", got, want)
	}

	// base bytes replace the file on disk; the base listed again is not read twice;
	// an absolute path is honoured.
	files, err = LoadSources(dir, []byte("memory base"), []string{"docker-compose.yml", abs}, "")
	if err != nil || len(files) != 2 || string(files[0].Data) != "memory base" || string(files[1].Data) != "override" {
		t.Errorf("base replacement: %v %v", files, err)
	}

	// a missing base is an error naming the file; a missing base with bytes is not.
	empty := t.TempDir()
	if _, err := LoadSources(empty, nil, nil, ""); err == nil || !strings.Contains(err.Error(), "docker-compose.yml") {
		t.Errorf("missing base: %v", err)
	}
	if files, err := LoadSources(empty, []byte("x"), nil, ""); err != nil || len(files) != 1 {
		t.Errorf("base bytes without a file: %v %v", files, err)
	}
	// a listed path that cannot be read as a file (a directory) is an error, not a skip.
	if err := os.Mkdir(filepath.Join(dir, "adir.yml"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSources(dir, nil, []string{"adir.yml"}, ""); err == nil || !strings.Contains(err.Error(), "adir.yml") {
		t.Errorf("unreadable listed file: %v", err)
	}
}

// TestLockValidateRejects: every rule Validate states fails on its own.
func TestLockValidateRejects(t *testing.T) {
	cases := []struct {
		name string
		mut  func(l *Lock)
		want string
	}{
		{"schema", func(l *Lock) { l.SchemaVersion = 2 }, "unsupported schema_version"},
		{"cli", func(l *Lock) { l.CLI = "one.five" }, "not major.minor"},
		{"pinned ref", func(l *Lock) { l.Images["redis"] = "redis:7" }, "images["},
		{"pinned key", func(l *Lock) { l.Images["Bad Key"] = "r/x:1@" + digA }, "images["},
		{"unpinned ref", func(l *Lock) { l.Unpinned["minio"] = "m/m:1@" + digA }, "unpinned["},
		{"both", func(l *Lock) { l.Unpinned["redis"] = "docker.io/pgsty/minio:latest" }, "both pinned and unpinned"},
		{"plugin slug", func(l *Lock) { l.Plugins["Bad Slug"] = PluginPin{Tier: TierFree} }, "plugin slug"},
		{"plugin tier", func(l *Lock) { l.Plugins["x"] = PluginPin{Version: "1.0.0", Tier: "paid"} }, "tier"},
		{"plugin sha", func(l *Lock) { l.Plugins["x"] = PluginPin{Version: "1.0.0", Tier: TierFree, SHA256: "XYZ"} }, "sha256"},
		{"bundle twice", func(l *Lock) { l.Bundles = []string{"ai", "ai"} }, "bundle"},
		{"bundle malformed", func(l *Lock) { l.Bundles = []string{"Not Valid"} }, "bundle"},
	}
	for _, c := range cases {
		l := sample()
		c.mut(&l)
		if err := l.Validate(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Validate = %v, want %q", c.name, err, c.want)
		}
		if _, err := l.Marshal(); err == nil {
			t.Errorf("%s: Marshal must refuse an invalid lock", c.name)
		}
	}
	if err := sample().Validate(); err != nil {
		t.Errorf("the sample must be valid: %v", err)
	}
}

// TestLockNormalizeAndParseGuards: nil collections become empty, bundles sort,
// and Parse refuses an oversize, cut-short or empty file.
func TestLockNormalizeAndParseGuards(t *testing.T) {
	l := Lock{Bundles: []string{"b", "a"}}
	l.normalize()
	if l.SchemaVersion != SchemaVersion || l.Images == nil || l.Unpinned == nil || l.Plugins == nil || strings.Join(l.Bundles, "") != "ab" {
		t.Errorf("normalize = %+v", l)
	}
	raw, err := (Lock{CLI: "1.5"}).Marshal()
	if err != nil {
		t.Fatalf("a zero-collection lock must marshal: %v", err)
	}
	if _, err := Parse(raw); err != nil {
		t.Errorf("round trip: %v", err)
	}
	if _, err := Parse(append(raw, make([]byte, MaxBytes)...)); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Errorf("oversize: %v", err)
	}
	if _, err := Parse([]byte("schema_version: 1\n")); err == nil || !strings.Contains(err.Error(), "final") {
		t.Errorf("no trailer: %v", err)
	}
	if _, err := Parse([]byte("# only a comment\n" + Trailer + "\n")); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("empty document: %v", err)
	}
	if _, err := Parse([]byte("unknown_key: 1\n" + Trailer + "\n")); err == nil {
		t.Error("an unknown key must be refused")
	}
	if l := (*Lock)(nil); len(l.Pins()) != 0 {
		t.Error("nil lock has no pins")
	}
}
