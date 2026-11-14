package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// f17Env points the generator at the real tree's VERSION and a temp plugin root.
type f17Env struct {
	*env
	good, outDir, version string
}

func newF17(t *testing.T) *f17Env {
	e := newEnv(t)
	e.d.Root = repoRoot
	f := &f17Env{env: e, good: t.TempDir(), outDir: t.TempDir()}
	f.version = strings.Join(strings.Fields(readRepo(t, ".github/VERSION")), "")
	os.MkdirAll(filepath.Join(f.good, "plugins"), 0o755)
	os.WriteFile(filepath.Join(f.good, "plugins", "registry.json"),
		[]byte(`{"plugins":{"demo":{"status":"stable","minNselfVersion":"1.0.0"}}}`+"\n"), 0o644)
	return f
}

func (f *f17Env) run(root, out string) int {
	f.out.Reset()
	f.err.Reset()
	f.vars["F17_NSELF_ROOT"], f.vars["F17_OUT_FILE"] = root, out
	return RunWith(f.d, []string{"f17"})
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// Case 1: happy path names the version and lists the plugin.
func TestSportGenCoreF02F17F17HappyPath(t *testing.T) {
	f := newF17(t)
	out := filepath.Join(f.outDir, "f17.md")
	if code := f.run(f.good, out); code != 0 {
		t.Fatalf("code %d: %s", code, f.err)
	}
	b, _ := os.ReadFile(out)
	want := f17Header + "## v" + f.version + "\n\n| Plugin | Status | Notes |\n|--------|--------|-------|\n" +
		"| demo                           | stable       |  |\n\n\n## Summary\n\n- **CLI versions covered:** 1\n- **Generated:** 2026-11-07T01:02:03Z\n"
	wantEq(t, "file", string(b), want)
	wantEq(t, "stdout", f.out.String(), "F17 generated for CLI v"+f.version+": "+out+"\n")
	wantEq(t, "stderr", f.err.String(), "")
	if len(f.r.calls) != 0 {
		t.Errorf("runner used: %v", f.r.calls)
	}
}

// Case 2: no registries at all fails closed, no file.
func TestSportGenCoreF02F17F17MissingRegistries(t *testing.T) {
	f := newF17(t)
	empty := t.TempDir()
	out := filepath.Join(f.outDir, "none.md")
	if code := f.run(empty, out); code != 1 {
		t.Errorf("code %d", code)
	}
	wantEq(t, "stderr", f.err.String(), "error: no plugin rows read from "+empty+"/plugins/registry.json or "+
		empty+"/plugins-pro/registry.json (registries missing, unreadable or empty)\n")
	if exists(out) {
		t.Error("file written")
	}
}

// Case 3: registry present with an empty plugin set fails closed.
func TestSportGenCoreF02F17F17EmptyPluginSet(t *testing.T) {
	f := newF17(t)
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "plugins"), 0o755)
	os.WriteFile(filepath.Join(root, "plugins", "registry.json"), []byte(`{"plugins":{}}`+"\n"), 0o644)
	out := filepath.Join(f.outDir, "emptyreg.md")
	if code := f.run(root, out); code != 1 {
		t.Errorf("code %d", code)
	}
	if !strings.Contains(f.err.String(), "error: no plugin rows read from") || exists(out) {
		t.Errorf("stderr %q exists %v", f.err, exists(out))
	}
}

// Case 4: the output directory must exist and is never created.
func TestSportGenCoreF02F17F17AbsentOutputDir(t *testing.T) {
	f := newF17(t)
	absent := filepath.Join(f.outDir, "absent")
	if code := f.run(f.good, filepath.Join(absent, "dir", "f17.md")); code != 1 {
		t.Errorf("code %d", code)
	}
	wantEq(t, "stderr", f.err.String(), "error: output directory does not exist: "+filepath.Join(absent, "dir")+"\n")
	if exists(absent) {
		t.Error("directory created")
	}
}

// Case 5: the retired .claude/docs/sport path is refused and not created.
func TestSportGenCoreF02F17F17RetiredPath(t *testing.T) {
	f := newF17(t)
	proj := filepath.Join(f.outDir, "proj")
	os.MkdirAll(filepath.Join(proj, ".claude", "docs"), 0o755)
	out := filepath.Join(proj, ".claude", "docs", "sport", "F17-CLI-PLUGIN-COMPAT.md")
	if code := f.run(f.good, out); code != 1 {
		t.Errorf("code %d", code)
	}
	wantEq(t, "stderr", f.err.String(), "error: refusing to write to the retired .claude/docs/sport/ path: "+out+"\n")
	if exists(filepath.Join(proj, ".claude", "docs", "sport")) {
		t.Error("retired dir created")
	}
}

// Case 6: default output (no F17_OUT_FILE) goes under TMPDIR.
func TestSportGenCoreF02F17F17DefaultOutputUnderTmpdir(t *testing.T) {
	f := newF17(t)
	f.vars["F17_NSELF_ROOT"], f.vars["TMPDIR"] = f.good, f.outDir
	if code := RunWith(f.d, []string{"f17"}); code != 0 {
		t.Fatalf("code %d: %s", code, f.err)
	}
	want := filepath.Join(f.outDir, "F17-CLI-PLUGIN-COMPAT.md")
	if !exists(want) || strings.Contains(f.out.String(), ".claude/docs/sport") {
		t.Errorf("stdout %q", f.out)
	}
	wantEq(t, "stdout", f.out.String(), "F17 generated for CLI v"+f.version+": "+want+"\n")
}

// Case 7: a good run prints nothing on stderr (no integer-expression noise).
func TestSportGenCoreF02F17F17NoNoise(t *testing.T) {
	f := newF17(t)
	f.run(f.good, filepath.Join(f.outDir, "quiet.md"))
	wantEq(t, "stderr", f.err.String(), "")
	if strings.Contains(f.out.String(), "integer") {
		t.Errorf("noise: %q", f.out)
	}
}

// Labels, version range filtering and registry order across free and pro registries.
func TestSportGenCoreF02F17F17LabelsRangeAndOrder(t *testing.T) {
	f := newF17(t)
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "plugins"), 0o755)
	os.MkdirAll(filepath.Join(root, "plugins-pro"), 0o755)
	os.WriteFile(filepath.Join(root, "plugins", "registry.json"), []byte(`{"plugins":{
 "zeta":{"status":"beta","min_nself_version":"0.1.0"},
 "alpha":{"status":"eol","minNselfVersion":"0.1.0"},
 "old":{"minNselfVersion":"0.1.0","maxNselfVersion":"0.2.0"},
 "future":{"minNselfVersion":"99.0.0"}}}`), 0o644)
	os.WriteFile(filepath.Join(root, "plugins-pro", "registry.json"),
		[]byte(`{"plugins":{"pro1":{"status":"planned"}}}`), 0o644)
	out := filepath.Join(f.outDir, "l.md")
	if code := f.run(root, out); code != 0 {
		t.Fatalf("code %d: %s", code, f.err)
	}
	b, _ := os.ReadFile(out)
	rows := "| zeta                           | beta         |  *(beta)* |\n" +
		"| alpha                          | eol          |  **[EOL]** |\n" +
		"| pro1                           | planned      |  *(planned)* |\n"
	if !strings.Contains(string(b), "|--------|--------|-------|\n"+rows+"\n\n## Summary") {
		t.Errorf("rows wrong:\n%s", b)
	}
}

func TestSportGenCoreF02F17SemverLTE(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"1.0.0", "1.0.0", true}, {"1.4.9", "1.4.15", true}, {"1.5.0", "1.4.15", false},
		{"0.0.0", "99.99.99", true}, {"x", "1.0.0", false}} {
		if got := semverLTE(c.a, c.b); got != c.want {
			t.Errorf("semverLTE(%q,%q)=%v", c.a, c.b, got)
		}
	}
}
