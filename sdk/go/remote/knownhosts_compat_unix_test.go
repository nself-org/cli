//go:build unix && !aix && !solaris && !illumos

package remote

// Purpose: dual-mode tests for the ancestor walk gate (D-0675): v1.4 warns
// once per pin dir and goes on as v1.4.13 did, compat.V15 refuses. No
// network, no root needed.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/compat/compattest"
)

// captureWarn sends the v1.4 ancestor warning to a buffer for the rest of
// the test.
func captureWarn(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := ancestorWarnOut
	ancestorWarnOut = &buf
	t.Cleanup(func() { ancestorWarnOut = old })
	return &buf
}

// D-0675: on v1.4 a 0775 ancestor (Debian's ~/.config) warns once per pin
// dir and every operation goes on, as on v1.4.13; under compat.V15 the same
// tree is refused and the pin file is left alone.
func TestAncestor_V14WarnsOnceV15Refuses(t *testing.T) {
	skipWindows(t)
	compattest.Set(t, false)
	buf := captureWarn(t)
	p, shared := nestedPin(t)
	if err := p.Add("n1", keyA); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o775); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("warned before the ancestor was writable: %q", buf)
	}
	if err := p.Add("n2", keyB); err != nil {
		t.Fatalf("v1.4 Add under a 0775 ancestor: %v", err)
	}
	if got, err := p.Lookup("n2"); err != nil || len(got) != 1 {
		t.Fatalf("v1.4 Lookup under a 0775 ancestor = %q, %v", got, err)
	}
	if err := p.Remove("n1"); err != nil {
		t.Fatalf("v1.4 Remove under a 0775 ancestor: %v", err)
	}
	w := buf.String()
	if strings.Count(w, "\n") != 1 || !strings.HasPrefix(w, "warning: ") ||
		!strings.Contains(w, "ancestor "+shared+" has mode 0775") || !strings.Contains(w, "nself v1.5 refuses it") {
		t.Fatalf("v1.4 warning = %q, want one line naming %s", w, shared)
	}
	before := readPin(t, p)
	compattest.Set(t, true)
	_, lerr := p.Lookup("n2")
	aerr := p.Add("n3", keyA)
	for op, err := range map[string]error{"Lookup": lerr, "Add": aerr} {
		if err == nil || !strings.Contains(err.Error(), "ancestor "+shared+" ") {
			t.Errorf("v1.5 %s under a 0775 ancestor: %v", op, err)
		}
	}
	if readPin(t, p) != before {
		t.Error("v1.5 refusal changed the pin file")
	}
	if buf.String() != w {
		t.Errorf("v1.5 refusal also warned: %q", buf.String()[len(w):])
	}
}

// D-0675: on v1.4 a new pin dir under a writable ancestor is created, as on
// v1.4.13, with one warning; the pin dir itself is still made 0700.
func TestAncestor_V14CreatesUnderWritableAncestor(t *testing.T) {
	skipWindows(t)
	compattest.Set(t, false)
	buf := captureWarn(t)
	_, home := pinFile(t)
	open := filepath.Join(home, "open")
	if err := os.Mkdir(open, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(open, 0o777); err != nil {
		t.Fatal(err)
	}
	p := PinnedHostKeys{Path: filepath.Join(open, "new", "ci", "known_hosts")}
	if err := p.Add("n1", keyA); err != nil {
		t.Fatalf("v1.4 Add under a 0777 ancestor: %v", err)
	}
	fi, err := os.Stat(filepath.Dir(p.Path))
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("pin dir = %v, %v; want 0700", fi, err)
	}
	if w := buf.String(); strings.Count(w, "\n") != 1 || !strings.Contains(w, "ancestor "+open+" has mode 0777") {
		t.Errorf("v1.4 warning = %q", w)
	}
}

// D-0675 keeps the pin dir's own check ungated: a 0775 pin dir is refused on
// v1.4 too, and that refusal is not turned into a warning.
func TestAncestor_V14StillRefusesWritablePinDir(t *testing.T) {
	skipWindows(t)
	compattest.Set(t, false)
	buf := captureWarn(t)
	p, _ := pinFile(t)
	if err := os.MkdirAll(filepath.Dir(p.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(p.Path), 0o775); err != nil {
		t.Fatal(err)
	}
	if err := p.Add("n1", keyA); err == nil || !strings.Contains(err.Error(), "chmod it 0700") {
		t.Errorf("v1.4 Add into a 0775 pin dir: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("pin dir refusal warned: %q", buf)
	}
}

// ancestorGate passes nil through on both modes without a warning.
func TestAncestorGate_NilIsSilent(t *testing.T) {
	buf := captureWarn(t)
	compattest.Set(t, false)
	if err := ancestorGate("/pin", nil); err != nil {
		t.Errorf("v1.4 ancestorGate(nil) = %v", err)
	}
	compattest.Set(t, true)
	if err := ancestorGate("/pin", nil); err != nil {
		t.Errorf("v1.5 ancestorGate(nil) = %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("ancestorGate(nil) warned: %q", buf)
	}
}

// The warn-once key is the pin dir: a second pin dir under a writable
// ancestor gets its own warning, a repeat call on either gets none.
func TestAncestor_V14WarnsOncePerPinDir(t *testing.T) {
	skipWindows(t)
	compattest.Set(t, false)
	buf := captureWarn(t)
	p1, shared1 := nestedPin(t)
	p2, shared2 := nestedPin(t)
	for _, d := range []string{shared1, shared2} {
		if err := os.Chmod(d, 0o775); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []PinnedHostKeys{p1, p2, p1, p2} {
		if err := p.Add("n1", keyA); err != nil {
			t.Fatalf("v1.4 Add %s: %v", p.Path, err)
		}
	}
	w := buf.String()
	if strings.Count(w, "\n") != 2 || !strings.Contains(w, "ancestor "+shared1+" ") || !strings.Contains(w, "ancestor "+shared2+" ") {
		t.Errorf("warnings = %q, want one per pin dir", w)
	}
}
