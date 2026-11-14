package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRunner records every call and answers from canned output.
type fakeRunner struct {
	calls []string
	out   map[string]string // joined "name args" prefix -> stdout
	fail  map[string]bool   // joined prefix -> error
	code  map[string]int    // joined prefix -> exit code of a RunError
	tty   io.Writer         // the "terminal" a tty RunIn writes to
	ttys  []string          // calls made on the terminal
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, line)
	for p, c := range f.code {
		if strings.HasPrefix(line, p) {
			return "", &RunError{Code: c, Stderr: "fake exit"}
		}
	}
	for p := range f.fail {
		if strings.HasPrefix(line, p) {
			return "", errors.New("fake failure")
		}
	}
	for p, o := range f.out {
		if strings.HasPrefix(line, p) {
			return o, nil
		}
	}
	return "", nil
}

// RunIn is DirRunner: a tty call writes its canned stdout to the terminal
// (f.tty) and returns nothing, as a child on the real terminal would.
func (f *fakeRunner) RunIn(dir string, tty bool, name string, args ...string) (string, error) {
	out, err := f.Run(name, args...)
	if !tty {
		return out, err
	}
	f.ttys = append(f.ttys, f.calls[len(f.calls)-1])
	if f.tty != nil {
		io.WriteString(f.tty, out)
	}
	return "", err
}

func (f *fakeRunner) called(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// env builds a Deps over a temp repo with captured output.
type env struct {
	d        Deps
	r        *fakeRunner
	out, err *bytes.Buffer
}

func newEnv(t *testing.T, stdin string) *env {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".github"), 0o755)
	os.WriteFile(filepath.Join(root, ".github", "release-denylist.json"), []byte("{}"), 0o644)
	r := &fakeRunner{
		out:  map[string]string{"gh run list": "success\n", "gh api": "closed\n", "git -C " + root + " rev-parse": "abc123\n"},
		fail: map[string]bool{},
	}
	e := &env{r: r, out: &bytes.Buffer{}, err: &bytes.Buffer{}}
	r.tty = e.out
	e.d = Deps{
		Runner: r, Stdout: e.out, Stderr: e.err, Stdin: bufio.NewReader(strings.NewReader(stdin)),
		Root: root, Env: func(k string) string { return map[string]string{"USER": "tester"}[k] },
		HaveGH: func() bool { return true },
		Now:    func() time.Time { return time.Date(2026, 11, 7, 1, 2, 3, 0, time.UTC) },
	}
	return e
}
