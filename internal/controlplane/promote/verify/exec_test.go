package verify

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fakePlugin(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the plugin")
	}
	path := filepath.Join(t.TempDir(), "nself-ci")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExecEvidenceRunsTheVerifyArgv(t *testing.T) {
	repo := t.TempDir()
	bin := fakePlugin(t, `printf '%s|' "$@"; pwd; exit 10`)
	out, exit, err := ExecEvidence(bin, filepath.Dir(bin), repo)(context.Background(), "/x/evidence.json", testRev)
	if err != nil || exit != 10 {
		t.Fatalf("exit %d err %v, want exit 10 and no error", exit, err)
	}
	want := "evidence|verify|--file|/x/evidence.json|--at-revision|" + testRev + "|--json|"
	if !strings.HasPrefix(string(out), want) {
		t.Fatalf("argv = %q, want prefix %q", out, want)
	}
	if real, _ := filepath.EvalSymlinks(repo); !strings.Contains(string(out), real) {
		t.Fatalf("plugin ran outside the project root: %q", out)
	}
}

func TestExecEvidenceExitZeroAndMissingBinary(t *testing.T) {
	ok := fakePlugin(t, `printf ok`)
	out, exit, err := ExecEvidence(ok, filepath.Dir(ok), t.TempDir())(context.Background(), "f", testRev)
	if err != nil || exit != 0 || string(out) != "ok" {
		t.Fatalf("out %q exit %d err %v", out, exit, err)
	}
	dir := t.TempDir()
	if _, _, err := ExecEvidence(filepath.Join(dir, "absent"), dir, t.TempDir())(context.Background(), "f", testRev); err == nil {
		t.Fatal("a missing binary reported no error")
	}
	// A binary outside the plugins directory, or not executable, is refused.
	if _, _, err := ExecEvidence(ok, t.TempDir(), t.TempDir())(context.Background(), "f", testRev); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("outside binary: %v", err)
	}
	plain := filepath.Join(filepath.Dir(ok), "plain")
	if err := os.WriteFile(plain, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ExecEvidence(plain, filepath.Dir(ok), t.TempDir())(context.Background(), "f", testRev); err == nil || !strings.Contains(err.Error(), "executable") {
		t.Fatalf("non-executable binary: %v", err)
	}
}

func TestLimitWriterDropsTheExcess(t *testing.T) {
	var buf bytes.Buffer
	w := &limitWriter{w: &buf, left: 5}
	if n, err := w.Write([]byte("abcdefgh")); n != 8 || err != nil {
		t.Fatalf("first write n=%d err=%v, want the full length reported", n, err)
	}
	if n, _ := w.Write([]byte("zz")); n != 2 {
		t.Fatalf("second write n=%d", n)
	}
	if buf.String() != "abcde" {
		t.Fatalf("buffer = %q, want abcde", buf.String())
	}
}
