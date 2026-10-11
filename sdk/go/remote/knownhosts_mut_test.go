package remote

// Purpose: boundary and exact-outcome tests that pin the pinned known_hosts
// code against mutation (D-0253): field-count edges, trailing comments, the
// empty-file write, and no-op operations leaving the file byte-identical.
// No network; synthetic keys only.

import (
	"os"
	"path/filepath"
	"testing"
)

func writePin(t *testing.T, p PinnedHostKeys, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readPin(t *testing.T, p PinnedHostKeys) string {
	t.Helper()
	raw, err := os.ReadFile(p.Path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

const (
	blobA = "AAAAC3NzaC1lZDI1NTE5AAAAIAEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEB"
	blobB = "AAAAC3NzaC1lZDI1NTE5AAAAIAICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgIC"
)

func TestMut_ParseKeyLineFieldCounts(t *testing.T) {
	for _, tc := range []struct {
		name, line, typ, b64 string
		ok                   bool
	}{
		{"one field", "ssh-ed25519", "", "", false},
		{"two fields type+blob", "ssh-ed25519 " + blobA, "ssh-ed25519", blobA, true},
		{"two fields host+type", "n1 ssh-ed25519", "", "", false},
		{"three fields host+type+blob", "n1 ssh-ed25519 " + blobA, "ssh-ed25519", blobA, true},
		{"three fields type+blob+comment", "ssh-ed25519 " + blobA + " note", "ssh-ed25519", blobA, true},
		{"four fields host+type+blob+comment", "n1 ssh-ed25519 " + blobB + " note", "ssh-ed25519", blobB, true},
		{"three fields no key type", "a b c", "", "", false},
		{"empty", "", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typ, b64, err := parseKeyLine(tc.line)
			if (err == nil) != tc.ok || typ != tc.typ || b64 != tc.b64 {
				t.Fatalf("parseKeyLine(%q) = %q, %q, %v", tc.line, typ, b64, err)
			}
		})
	}
}

func TestMut_LookupReturnsTypeAndBlobOnly(t *testing.T) {
	p, _ := pinFile(t)
	writePin(t, p, "n1 ssh-ed25519 "+blobA+" trailing comment\n"+
		"n1\n"+
		"n1 ssh-rsa\n"+
		"n2 ssh-ed25519 "+blobB+"\n")
	got, err := p.Lookup("n1")
	if err != nil || len(got) != 1 || got[0] != "ssh-ed25519 "+blobA {
		t.Fatalf("Lookup(n1) = %q, %v", got, err)
	}
	got, err = p.Lookup("n2")
	if err != nil || len(got) != 1 || got[0] != "ssh-ed25519 "+blobB {
		t.Fatalf("Lookup(n2) = %q, %v", got, err)
	}
	if got, err := p.Lookup("n3"); err != nil || len(got) != 0 {
		t.Fatalf("Lookup(n3) = %q, %v", got, err)
	}
}

func TestMut_AddSkipsShortLines(t *testing.T) {
	p, _ := pinFile(t)
	// Two-field and one-field lines for the same alias are not pins.
	writePin(t, p, "n1 ssh-ed25519\nn1\n")
	if err := p.Add("n1", "ssh-ed25519 "+blobA); err != nil {
		t.Fatalf("Add beside short lines: %v", err)
	}
	want := "n1 ssh-ed25519\nn1\nn1 ssh-ed25519 " + blobA + "\n"
	if got := readPin(t, p); got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
	// The same key again is a no-op; a different key of that type is refused.
	if err := p.Add("n1", "ssh-ed25519 "+blobA); err != nil || readPin(t, p) != want {
		t.Fatalf("idempotent Add changed things: %v %q", err, readPin(t, p))
	}
	if err := p.Add("n1", "ssh-ed25519 "+blobB); err == nil {
		t.Fatal("changed key accepted by Add")
	}
}

func TestMut_RemoveLastKeyLeavesEmptyFile(t *testing.T) {
	p, _ := pinFile(t)
	if err := p.Add("n1", keyA); err != nil {
		t.Fatal(err)
	}
	if err := p.Remove("n1"); err != nil {
		t.Fatal(err)
	}
	if got := readPin(t, p); got != "" {
		t.Fatalf("file after removing the last key = %q, want empty", got)
	}
}

func TestMut_RemoveDropsSingleFieldLine(t *testing.T) {
	p, _ := pinFile(t)
	writePin(t, p, "n1\nn2 "+keyA+"\n")
	if err := p.Remove("n1"); err != nil {
		t.Fatal(err)
	}
	if got, want := readPin(t, p), "n2 "+keyA+"\n"; got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
}

func TestMut_RemoveAbsentAliasDoesNotRewrite(t *testing.T) {
	p, _ := pinFile(t)
	// A rewrite would drop the blank line, so byte identity proves no write.
	content := "n1 " + keyA + "\n\n# keep me\n"
	writePin(t, p, content)
	if err := p.Remove("absent"); err != nil {
		t.Fatal(err)
	}
	if got := readPin(t, p); got != content {
		t.Fatalf("Remove of an absent alias rewrote the file: %q", got)
	}
	// Removing a present alias does rewrite (blank dropped, comment kept).
	if err := p.Remove("n1"); err != nil {
		t.Fatal(err)
	}
	if got, want := readPin(t, p), "# keep me\n"; got != want {
		t.Fatalf("after Remove = %q, want %q", got, want)
	}
}

func TestMut_ReplaceCreatesMissingFile(t *testing.T) {
	p, _ := pinFile(t)
	if err := p.Replace("n1", keyA, keyR); err != nil {
		t.Fatal(err)
	}
	if got, want := readPin(t, p), "n1 "+keyA+"\nn1 "+keyR+"\n"; got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
}

func TestMut_ReplaceKeepsOtherAliasesAndOrder(t *testing.T) {
	p, _ := pinFile(t)
	writePin(t, p, "a "+keyA+"\nn1 "+keyA+"\nn1 "+keyR+"\nb "+keyB+"\n")
	if err := p.Replace("n1", keyB); err != nil {
		t.Fatal(err)
	}
	if got, want := readPin(t, p), "a "+keyA+"\nb "+keyB+"\nn1 "+keyB+"\n"; got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
}
