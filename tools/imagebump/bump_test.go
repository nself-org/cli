package main

// bump_test.go — end-to-end tests of run/execute. No network, no docker.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGoldenFixture is the ticket acceptance: the fixture run prints exactly
// testdata/expected.out.
func TestGoldenFixture(t *testing.T) {
	want, err := os.ReadFile("testdata/expected.out")
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := run([]string{"--dry-run", "--policy", "testdata/policy.yaml", "--lock", "testdata/lock.fixture"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if out.String() != string(want) {
		t.Fatalf("stdout mismatch\n got: %q\nwant: %q", out.String(), string(want))
	}
}

// writeTemp writes name under a fresh temp dir and returns its path.
func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const testPolicy = "schema_version: 1\nservices:\n  a: {regex: '^v(\\d+)\\.(\\d+)\\.(\\d+)$', track: patch}\n"

type stubLister map[string][]string

func (s stubLister) Tags(r string) ([]string, error) { return s[r], nil }

func TestExecuteErrors(t *testing.T) {
	good := "schema_version: 1\nimages:\n  - {name: a, role: core, repository: r/a, version: v1.0.0}\n"
	cases := []struct {
		name, lock, policy string
		args               []string
		code               int
		want               string
	}{
		{"missing policy entry", good, "schema_version: 1\nservices:\n  b: {regex: '^x$', track: patch}\n", nil, 1, "a: no entry in the bump policy"},
		{"version outside regex", strings.Replace(good, "v1.0.0", "weird", 1), testPolicy, nil, 1, "a: current version does not match"},
		{"fixture without dry-run", good + "fixture_tags: {r/a: [v1.0.1]}\n", testPolicy, nil, 2, "refused without --dry-run"},
		{"unknown flag", good, testPolicy, []string{"--bogus"}, 2, ""},
		{"extra argument", good, testPolicy, []string{"stray"}, 2, ""},
		{"bad track", good, "schema_version: 1\nservices:\n  a: {regex: '^x$', track: major}\n", nil, 1, "want patch|minor|manual"},
		{"unanchored regex", good, "schema_version: 1\nservices:\n  a: {regex: 'v1', track: patch}\n", nil, 1, "must be anchored"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"--policy", writeTemp(t, dir, "p.yaml", c.policy), "--lock", writeTemp(t, dir, "l.yaml", c.lock)}, c.args...)
			var out, errb bytes.Buffer
			d := deps{lister: stubLister{}, resolve: func(string, io.Writer) int { return 0 }}
			if got := execute(args, &out, &errb, d); got != c.code {
				t.Fatalf("exit %d, want %d; stderr: %s", got, c.code, errb.String())
			}
			if !strings.Contains(errb.String(), c.want) {
				t.Fatalf("stderr %q lacks %q", errb.String(), c.want)
			}
			if out.Len() != 0 {
				t.Fatalf("stdout should be empty, got %q", out.String())
			}
		})
	}
}

// TestApplyRewritesAndResolves covers the non-dry path with a stub lister and
// a stub generator: only the version changes, the resolver is always called.
func TestApplyRewritesAndResolves(t *testing.T) {
	lock := "# keep me\nschema_version: 1\nimages:\n  - {name: a, role: core, repository: r/a, version: v1.0.0, license: MIT}\n"
	dir := t.TempDir()
	lp := writeTemp(t, dir, "l.yaml", lock)
	pp := writeTemp(t, dir, "p.yaml", testPolicy)
	for _, tags := range [][]string{{"v1.0.0", "v1.0.4", "v1.1.0"}, {"v1.0.0"}} {
		resolved := 0
		d := deps{lister: stubLister{"r/a": tags}, resolve: func(p string, _ io.Writer) int {
			resolved++
			if p != lp {
				t.Errorf("resolve path %q", p)
			}
			return 7
		}}
		var out, errb bytes.Buffer
		if got := execute([]string{"--policy", pp, "--lock", lp}, &out, &errb, d); got != 7 {
			t.Fatalf("exit %d, want the resolver's 7", got)
		}
		if resolved != 1 {
			t.Fatalf("resolver ran %d times", resolved)
		}
	}
	got, _ := os.ReadFile(lp)
	if want := strings.Replace(lock, "v1.0.0", "v1.0.4", 1); string(got) != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
}

// TestDryRunWritesNothing checks --dry-run leaves the file alone and skips the resolver.
func TestDryRunWritesNothing(t *testing.T) {
	lock := "schema_version: 1\nimages:\n  - {name: a, role: core, repository: r/a, version: v1.0.0}\n"
	dir := t.TempDir()
	lp := writeTemp(t, dir, "l.yaml", lock)
	d := deps{lister: stubLister{"r/a": {"v1.0.9"}}, resolve: func(string, io.Writer) int {
		t.Error("resolver must not run on --dry-run")
		return 1
	}}
	var out, errb bytes.Buffer
	args := []string{"--dry-run", "--policy", writeTemp(t, dir, "p.yaml", testPolicy), "--lock", lp}
	if got := execute(args, &out, &errb, d); got != 0 {
		t.Fatalf("exit %d: %s", got, errb.String())
	}
	if out.String() != "BUMP a v1.0.0 -> v1.0.9\n" {
		t.Fatalf("stdout %q", out.String())
	}
	if b, _ := os.ReadFile(lp); string(b) != lock {
		t.Fatal("dry-run modified the file")
	}
}
