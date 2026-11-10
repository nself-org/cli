package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRunner records every call and answers from canned output.
type fakeRunner struct {
	calls []string
	out   map[string]string // "name args" prefix -> stdout
	fail  string            // prefix that fails
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	line := strings.TrimSpace(name + " " + strings.Join(args, " "))
	f.calls = append(f.calls, line)
	if f.fail != "" && strings.HasPrefix(line, f.fail) {
		return "", errors.New("fake failure")
	}
	for p, o := range f.out {
		if strings.HasPrefix(line, p) {
			return o, nil
		}
	}
	return "", nil
}

type env struct {
	d        Deps
	r        *fakeRunner
	out, err *bytes.Buffer
	vars     map[string]string
}

// repoRoot is the real repo tree the committed files are read from.
const repoRoot = "../.."

func readRepo(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// newEnv builds Deps over a temp root with .github/wiki present.
func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".github", "wiki"), 0o755); err != nil {
		t.Fatal(err)
	}
	e := &env{r: &fakeRunner{out: map[string]string{}}, out: &bytes.Buffer{}, err: &bytes.Buffer{}, vars: map[string]string{}}
	e.d = Deps{Runner: e.r, Stdout: e.out, Stderr: e.err, Root: root,
		Env: func(k string) string { return e.vars[k] },
		Now: func() time.Time { return time.Date(2026, 11, 7, 1, 2, 3, 0, time.UTC) }}
	return e
}

func (e *env) read(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(e.d.Root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func wantEq(t *testing.T, what, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s mismatch\n got: %q\nwant: %q", what, got, want)
	}
}
