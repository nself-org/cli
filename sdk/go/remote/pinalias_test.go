package remote

import (
	"os"
	"path/filepath"
	"testing"
)

func mustSpec(t *testing.T, in string) HostSpec {
	t.Helper()
	spec, err := ParseHostSpec(in)
	if err != nil {
		t.Fatalf("ParseHostSpec(%q): %v", in, err)
	}
	return spec
}

func TestPinAlias(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a.test", "a.test"},
		{"A.Test", "a.test"},
		{"a.test:22", "a.test"}, // one port-22 rule: explicit 22 is the default endpoint
		{"a.test:2222", "nself+a.test+2222"},
		{"203.0.113.9", "nself+203.0.113.9+22"},
		{"203.0.113.9:22", "nself+203.0.113.9+22"},
		{"203.0.113.9:2222", "nself+203.0.113.9+2222"},
		{"[2001:db8::1]", "nself+2001:db8::1+22"},
		{"[2001:DB8::1]:2222", "nself+2001:db8::1+2222"},
		{"u@a.test:2222", "nself+a.test+2222"}, // the user is not part of the endpoint
	}
	for _, tc := range cases {
		if got := PinAlias(mustSpec(t, tc.in)); got != tc.want {
			t.Errorf("PinAlias(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestPinAliasNoCollision is D-0463: the old scheme mapped these pairs to one alias.
func TestPinAliasNoCollision(t *testing.T) {
	pairs := [][2]string{
		{"[2001:db8::1]:2222", "2001-db8--1:2222"}, // ':' folded to '-'
		{"nself-a.test-2222", "a.test:2222"},       // a host named like an alias, on 22
		{"nself-a.test-2222:22", "a.test:2222"},    // same, explicit 22
		{"a.test-2222", "a.test:2222"},             // port folded into the name
		{"a.test:2222", "a.test:22222"},            // port digits run into the host
		{"203.0.113.9:2222", "203.0.113.9:222"},
		{"[2001:db8::1]", "[2001:db8::1]:2222"},
	}
	for _, p := range pairs {
		a, b := PinAlias(mustSpec(t, p[0])), PinAlias(mustSpec(t, p[1]))
		if a == b {
			t.Errorf("%q and %q share the pin alias %q", p[0], p[1], a)
		}
	}
	// No host name can spell a synthetic alias: ParseHostSpec refuses '+'.
	if _, err := ParseHostSpec("nself+a.test+2222"); err == nil {
		t.Error("a host name holding '+' parsed, so a name could collide with a synthetic alias")
	}
}

func TestPinAliasIsAcceptedByThePinFile(t *testing.T) {
	for _, in := range []string{"a.test", "a.test:2222", "203.0.113.9", "[2001:db8::1]:2222"} {
		if _, err := checkPinAlias(PinAlias(mustSpec(t, in))); err != nil {
			t.Errorf("pin alias for %q refused: %v", in, err)
		}
	}
}

func TestLegacyPinAliases(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a.test", nil},
		{"a.test:22", []string{"nself-a.test-22"}}, // deploy pinned explicit 22 under this
		{"a.test:2222", []string{"nself-a.test-2222"}},
		{"203.0.113.9", []string{"nself-203.0.113.9-22"}},
		{"[2001:db8::1]:2222", []string{"nself-2001-db8--1-2222"}},
	}
	for _, tc := range cases {
		got := LegacyPinAliases(mustSpec(t, tc.in))
		if len(got) != len(tc.want) || (len(got) == 1 && got[0] != tc.want[0]) {
			t.Errorf("LegacyPinAliases(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func liveKey(t *testing.T, line string) HostKey {
	t.Helper()
	typ, _, err := parseKeyLine(line)
	if err != nil {
		t.Fatal(err)
	}
	fp, err := Fingerprint(line)
	if err != nil {
		t.Fatal(err)
	}
	return HostKey{Line: line, Type: typ, Fingerprint: fp}
}

func newPins(t *testing.T) PinnedHostKeys {
	t.Helper()
	return PinnedHostKeys{Path: filepath.Join(safeTempDir(t), "deploy_known_hosts")}
}

func lookup(t *testing.T, p PinnedHostKeys, alias string) []string {
	t.Helper()
	keys, err := p.Lookup(alias)
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func TestMigrateLegacyPinCopiesTheMatchingKey(t *testing.T) {
	cases := []struct{ host, legacy string }{
		{"a.test:22", "nself-a.test-22"}, // the explicit-22 pin admin could not see
		{"a.test:2222", "nself-a.test-2222"},
		{"203.0.113.9", "nself-203.0.113.9-22"},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			p := newPins(t)
			spec := mustSpec(t, tc.host)
			if err := p.Add(tc.legacy, keyA); err != nil {
				t.Fatal(err)
			}
			if err := p.MigrateLegacyPin(spec, []HostKey{liveKey(t, keyA)}); err != nil {
				t.Fatal(err)
			}
			if got := lookup(t, p, PinAlias(spec)); len(got) != 1 || got[0] != keyA {
				t.Fatalf("new alias holds %v, want [%s]", got, keyA)
			}
			if got := lookup(t, p, tc.legacy); len(got) != 1 {
				t.Fatalf("the legacy pin was removed: %v", got)
			}
			// A second run is a no-op.
			if err := p.MigrateLegacyPin(spec, []HostKey{liveKey(t, keyA)}); err != nil {
				t.Fatal(err)
			}
			if got := lookup(t, p, PinAlias(spec)); len(got) != 1 {
				t.Fatalf("second migration changed the pin: %v", got)
			}
		})
	}
}

func TestMigrateLegacyPinRefusals(t *testing.T) {
	spec := mustSpec(t, "a.test:2222")
	t.Run("a live key that is not the legacy pin copies nothing", func(t *testing.T) {
		p := newPins(t)
		if err := p.Add("nself-a.test-2222", keyA); err != nil {
			t.Fatal(err)
		}
		if err := p.MigrateLegacyPin(spec, []HostKey{liveKey(t, keyB)}); err != nil {
			t.Fatal(err)
		}
		if got := lookup(t, p, PinAlias(spec)); len(got) != 0 {
			t.Fatalf("an unpresented legacy key was migrated: %v", got)
		}
	})
	t.Run("a different key already pinned at the new alias is not replaced", func(t *testing.T) {
		p := newPins(t)
		if err := p.Add("nself-a.test-2222", keyA); err != nil {
			t.Fatal(err)
		}
		if err := p.Add(PinAlias(spec), keyB); err != nil {
			t.Fatal(err)
		}
		if err := p.MigrateLegacyPin(spec, []HostKey{liveKey(t, keyA)}); err != nil {
			t.Fatal(err)
		}
		if got := lookup(t, p, PinAlias(spec)); len(got) != 1 || got[0] != keyB {
			t.Fatalf("new alias holds %v, want only %s", got, keyB)
		}
	})
	t.Run("no legacy alias is a no-op that creates no file", func(t *testing.T) {
		p := newPins(t)
		if err := p.MigrateLegacyPin(mustSpec(t, "a.test"), []HostKey{liveKey(t, keyA)}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(p.Path); !os.IsNotExist(err) {
			t.Fatalf("pin file created: %v", err)
		}
	})
	t.Run("a refused pin file is an error", func(t *testing.T) {
		p := newPins(t)
		writePin(t, p, "*.test "+keyA+"\n")
		if err := p.MigrateLegacyPin(spec, []HostKey{liveKey(t, keyA)}); err == nil {
			t.Fatal("a foreign wildcard line did not fail closed")
		}
	})
}

func TestEffectivePinAlias(t *testing.T) {
	spec := mustSpec(t, "a.test:22")
	p := newPins(t)
	if got, err := p.EffectivePinAlias(spec); err != nil || got != "a.test" {
		t.Fatalf("empty file: %q, %v", got, err)
	}
	if err := p.Add("nself-a.test-22", keyA); err != nil {
		t.Fatal(err)
	}
	if got, err := p.EffectivePinAlias(spec); err != nil || got != "nself-a.test-22" {
		t.Fatalf("only the legacy alias pinned: %q, %v", got, err)
	}
	if err := p.Add("a.test", keyA); err != nil {
		t.Fatal(err)
	}
	if got, err := p.EffectivePinAlias(spec); err != nil || got != "a.test" {
		t.Fatalf("the current alias wins: %q, %v", got, err)
	}
	if got, err := p.EffectivePinAlias(mustSpec(t, "b.test")); err != nil || got != "b.test" {
		t.Fatalf("no legacy form: %q, %v", got, err)
	}
	bad := newPins(t)
	writePin(t, bad, "*.test "+keyA+"\n")
	if _, err := bad.EffectivePinAlias(spec); err == nil {
		t.Fatal("a refused pin file did not fail closed")
	}
}
