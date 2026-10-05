package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	keyA = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEB"
	keyB = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgIC"
	keyR = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAQQABAgMEBQYHCAkKCwwNDg8QERITFBUWFxgZGhscHR4fICEiIyQlJicoKSorLC0uLzAxMjM0NTY3ODk6Ozw9Pj9A"
)

func pinFile(t *testing.T) (PinnedHostKeys, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home) // proves ~/.ssh/known_hosts is never touched
	p := filepath.Join(home, "ci", "known_hosts")
	return PinnedHostKeys{Path: p}, home
}

func TestPinned_AddLookup(t *testing.T) {
	p, home := pinFile(t)
	if err := p.Add("nself-ci-n1", keyA); err != nil {
		t.Fatal(err)
	}
	got, err := p.Lookup("nself-ci-n1")
	if err != nil || len(got) != 1 || got[0] != keyA {
		t.Fatalf("Lookup = %q, %v", got, err)
	}
	raw, _ := os.ReadFile(p.Path)
	if string(raw) != "nself-ci-n1 "+keyA+"\n" {
		t.Fatalf("file = %q", raw)
	}
	if err := p.Add("nself-ci-n1", keyA); err != nil { // idempotent
		t.Fatal(err)
	}
	if err := p.Add("nself-ci-n1", keyR); err != nil { // other type: allowed
		t.Fatal(err)
	}
	if got, _ := p.Lookup("nself-ci-n1"); len(got) != 2 {
		t.Fatalf("keys = %q", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "known_hosts")); !os.IsNotExist(err) {
		t.Fatal("~/.ssh/known_hosts was touched")
	}
}

func TestPinned_ChangedKeyNeedsReplace(t *testing.T) {
	p, _ := pinFile(t)
	_ = p.Add("n1", keyA)
	if err := p.Add("n1", keyB); err == nil || !strings.Contains(err.Error(), "Replace") {
		t.Fatalf("changed key accepted by Add: %v", err)
	}
	if err := p.Replace("n1", keyB); err != nil {
		t.Fatal(err)
	}
	if got, _ := p.Lookup("n1"); len(got) != 1 || got[0] != keyB {
		t.Fatalf("after Replace = %q", got)
	}
	if err := p.Remove("n1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := p.Lookup("n1"); len(got) != 0 {
		t.Fatalf("after Remove = %q", got)
	}
}

func TestPinned_LookupNeverMatchesByIP(t *testing.T) {
	p, _ := pinFile(t)
	_ = p.Add("nself-ci-n1", keyA)
	if _, err := p.Lookup("203.0.113.7"); err == nil {
		t.Fatal("IP lookup did not error")
	}
	if err := p.Add("203.0.113.7", keyA); err == nil {
		t.Fatal("IP alias accepted by Add")
	}
	// A hand-edited line keyed by an address must still not match a lookup
	// by that address.
	raw, _ := os.ReadFile(p.Path)
	_ = os.WriteFile(p.Path, append(raw, []byte("203.0.113.7 "+keyB+"\n")...), 0o600)
	if keys, err := p.Lookup("203.0.113.7"); err == nil || len(keys) != 0 {
		t.Fatalf("lookup by IP = %q, %v", keys, err)
	}
}

func TestPinned_ModesAndAtomicity(t *testing.T) {
	p, _ := pinFile(t)
	if err := p.Add("n1", keyA); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p.Path)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v", fi.Mode().Perm())
	}
	di, _ := os.Stat(filepath.Dir(p.Path))
	if di.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v", di.Mode().Perm())
	}
	_ = p.Replace("n1", keyB)
	ents, _ := os.ReadDir(filepath.Dir(p.Path))
	if len(ents) != 1 {
		t.Errorf("temp files left behind: %v", ents)
	}
}

func TestPinned_RefusesBadInput(t *testing.T) {
	p, _ := pinFile(t)
	for _, alias := range []string{"", "-x", "a b", "a\nb", "a,b", "*", "a;b"} {
		if p.Add(alias, keyA) == nil {
			t.Errorf("alias %q accepted", alias)
		}
	}
	for _, line := range []string{"", "garbage", "ssh-ed25519", "ssh-ed25519 !!!notbase64", "n1 ssh-ed25519\nx y"} {
		if p.Add("n1", line) == nil {
			t.Errorf("key line %q accepted", line)
		}
	}
	if (PinnedHostKeys{}).Add("n1", keyA) == nil {
		t.Error("empty Path accepted")
	}
	if p.Remove("absent") != nil {
		t.Error("Remove on a missing alias errored")
	}
	if _, err := os.Stat(p.Path); !os.IsNotExist(err) {
		t.Error("file created by a no-op Remove or by refused input")
	}
}

func TestFingerprint(t *testing.T) {
	fp, err := Fingerprint(keyA)
	if err != nil || !strings.HasPrefix(fp, "SHA256:") || strings.Contains(fp, "=") {
		t.Fatalf("Fingerprint = %q, %v", fp, err)
	}
	fp2, _ := Fingerprint("host1 " + keyA + " comment")
	if fp != fp2 {
		t.Error("host prefix changed the fingerprint")
	}
	if _, err := Fingerprint("nope"); err == nil {
		t.Error("garbage fingerprinted")
	}
}
