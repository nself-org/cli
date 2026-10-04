package compat

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// runMarkers runs scripts/ci/compat-markers.sh in a scratch git repo holding
// files (path -> content) and returns stdout, stderr and the exit code.
func runMarkers(t *testing.T, files map[string]string, args ...string) (string, string, int) {
	out, errOut, rc, _ := runMarkersDir(t, files, args...)
	return out, errOut, rc
}

// runMarkersDir is runMarkers plus the scratch repo directory.
func runMarkersDir(t *testing.T, files map[string]string, args ...string) (string, string, int, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script test; the script runs on the Unix CI legs")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "ci", "compat-markers.sh"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for p, c := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	init := exec.Command("git", "init", "-q")
	init.Dir = dir
	if out, err := init.CombinedOutput(); err != nil {
		t.Skipf("git init failed: %v %s", err, out)
	}
	cmd := exec.Command(bash, append([]string{script}, args...)...)
	cmd.Dir = dir
	cmd.Env = filterGitEnv(os.Environ())
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	rc := 0
	if err := cmd.Run(); err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		rc = ee.ExitCode()
	}
	return so.String(), se.String(), rc, dir
}

func filterGitEnv(env []string) []string {
	out := env[:0:0]
	for _, e := range env {
		if strings.HasPrefix(e, "GIT_DIR=") || strings.HasPrefix(e, "GIT_WORK_TREE=") || strings.HasPrefix(e, "GIT_INDEX_FILE=") {
			continue
		}
		out = append(out, e)
	}
	return out
}

// TestMarkersIgnoreNonCalls: compat.V15() inside strings, trailing comments
// and block comments is not a call and never needs a marker.
func TestMarkersIgnoreNonCalls(t *testing.T) {
	src := "package x\n\n" +
		"var a = \"compat.V15()\"\n" +
		"var b = `raw compat.V15()`\n" +
		"var c = '\"' + \"esc \\\" compat.V15()\"\n" +
		"func f() { g() } // see compat.V15() docs\n" +
		"/* block compat.V15() */\n" +
		"/*\n * compat.V15() in a star line\n compat.V15() in a bare block body\n*/\n" +
		"var d = `multi\nline compat.V15()\nraw`\n" +
		"// compat.V15() in a line comment\n"
	out, errOut, rc := runMarkers(t, map[string]string{"cmd/x/a.go": src})
	if rc != 0 || out != "" {
		t.Fatalf("rc=%d stdout=%q stderr=%q, want clean", rc, out, errOut)
	}
}

// TestMarkersRealCallStillChecked: a real call after a string, or after a
// closed block comment, is flagged unless marked.
func TestMarkersRealCallStillChecked(t *testing.T) {
	bad := "package x\n\nfunc f() bool { s := \"x\"; _ = s; return compat.V15() }\n" +
		"func g() bool { /* c */ return compat.V15() }\n"
	_, errOut, rc := runMarkers(t, map[string]string{"cmd/x/a.go": bad})
	if rc != 1 || strings.Count(errOut, "has no marker") != 2 {
		t.Fatalf("rc=%d stderr=%q, want 2 unmarked", rc, errOut)
	}
	ok := "package x\n\nfunc f() bool { return compat.V15() } // compat.V15(T-1): a -> b\n" +
		"func g() bool {\n\t// compat.V15(T-2): c -> d\n\treturn compat.V15()\n}\n"
	out, errOut, rc := runMarkers(t, map[string]string{"cmd/x/a.go": ok})
	if rc != 0 || out != "cmd/x/a.go:3 T-1\ncmd/x/a.go:6 T-2\n" {
		t.Fatalf("rc=%d stdout=%q stderr=%q", rc, out, errOut)
	}
}

// TestMarkersRejectEmptyFields: an empty old or new behaviour is malformed.
func TestMarkersRejectEmptyFields(t *testing.T) {
	for _, m := range []string{"a -> ", "a ->  ", "a ->", " -> b", "a"} {
		src := "package x\n\nfunc f() bool {\n\t// compat.V15(T-1): " + m + "\n\treturn compat.V15()\n}\n"
		out, errOut, rc := runMarkers(t, map[string]string{"cmd/x/a.go": src})
		if rc != 1 || out != "" || !strings.Contains(errOut, "non-empty") {
			t.Fatalf("marker %q: rc=%d stdout=%q stderr=%q", m, rc, out, errOut)
		}
	}
}

// TestWriteWikiRefusesUnmarked: --write-wiki leaves the page untouched and
// fails while any call is unmarked.
func TestWriteWikiRefusesUnmarked(t *testing.T) {
	wiki := "x\n<!-- BEGIN GENERATED:gated -->\nOLD\n<!-- END GENERATED:gated -->\n"
	files := map[string]string{
		".github/wiki/Compat-V15.md": wiki,
		"cmd/x/a.go":                 "package x\n\nfunc f() bool { return compat.V15() }\n",
	}
	_, errOut, rc, dir := runMarkersDir(t, files, "--write-wiki")
	if rc == 0 || !strings.Contains(errOut, "no marker") {
		t.Fatalf("rc=%d stderr=%q, want failure", rc, errOut)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".github", "wiki", "Compat-V15.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != wiki {
		t.Fatalf("page was rewritten:\n%s", got)
	}
}
