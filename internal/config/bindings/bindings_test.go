package bindings

import (
	"strings"
	"testing"
)

var testKnown = []string{"BASE_DOMAIN", "ENV", "NSELF_NO_MONOREPO"}

const okFlags = "schema_version: 1\nbindings:\n  - {command: init, flag: domain, key: BASE_DOMAIN}\n"
const okExempt = "schema_version: 1\nexempt:\n  - {command: build, flag: no-monorepo, reason: unread}\n"

func TestBindingsLoad(t *testing.T) {
	b, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := b.Lookup("init", "domain")
	if !ok || got.Key != "BASE_DOMAIN" {
		t.Fatalf("init --domain = %+v, %v; want BASE_DOMAIN", got, ok)
	}
	if fe := b.FlagEnv()["init"]["domain"]; fe != "BASE_DOMAIN" {
		t.Errorf("FlagEnv init domain = %q", fe)
	}
	if rows := b.ForKey("ENV"); len(rows) < 2 {
		t.Errorf("ENV bound by %d flags, want several", len(rows))
	}
	if _, ok := b.Exemption("logs", "no-color"); !ok {
		t.Error("logs --no-color exemption missing")
	}
	for i := 1; i < len(b.Flags); i++ {
		p, c := b.Flags[i-1], b.Flags[i]
		if less(c.Command, c.Flag, p.Command, p.Flag) {
			t.Errorf("bindings not sorted at %d", i)
		}
	}
	if _, err := Parse([]byte(okFlags), []byte(okExempt), testKnown); err != nil {
		t.Errorf("minimal document: %v", err)
	}
}

func TestBindingsValidate(t *testing.T) {
	parseCases := []struct {
		name, flags, exempt, want string
	}{
		{"unknown key", "schema_version: 1\nbindings:\n  - {command: init, flag: domain, key: NOT_A_KEY}\n", okExempt, "not a known configuration key"},
		{"lower case key", "schema_version: 1\nbindings:\n  - {command: init, flag: domain, key: base_domain}\n", okExempt, "not UPPER_SNAKE"},
		{"missing field", "schema_version: 1\nbindings:\n  - {command: init, key: ENV}\n", okExempt, "command, flag and key are required"},
		{"duplicate", okFlags + "  - {command: init, flag: domain, key: ENV}\n", okExempt, "already declared"},
		{"bound and exempt", okFlags, "schema_version: 1\nexempt:\n  - {command: init, flag: domain, reason: x}\n", "bound or exempt, not both"},
		{"exempt without reason", okFlags, "schema_version: 1\nexempt:\n  - {command: build, flag: debug}\n", "command, flag and reason are required"},
		{"unknown yaml field", "schema_version: 1\nbindings:\n  - {command: init, flag: domain, key: ENV, extra: 1}\n", okExempt, "field extra not found"},
		{"bad version", "schema_version: 2\nbindings: []\n", okExempt, "schema_version 2"},
	}
	for _, tt := range parseCases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.flags), []byte(tt.exempt), testKnown)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v; want %q", err, tt.want)
			}
		})
	}

	b, err := Parse([]byte(okFlags), []byte(okExempt), testKnown)
	if err != nil {
		t.Fatal(err)
	}
	tree := []FlagRef{{"init", "domain"}, {"build", "no-monorepo"}}
	if err := b.Check(tree); err != nil {
		t.Errorf("clean tree: %v", err)
	}
	// A fixture command gains --base-domain (BASE_DOMAIN is a known key): the gate fails.
	err = b.Check(append(append([]FlagRef{}, tree...), FlagRef{"fixture", "base-domain"}))
	if err == nil || !strings.Contains(err.Error(), "nself fixture --base-domain looks like configuration key BASE_DOMAIN") {
		t.Errorf("unbound candidate: %v", err)
	}
	// NSELF_ prefix candidate.
	if err := b.Check(append(append([]FlagRef{}, tree...), FlagRef{"fixture", "no-monorepo"})); err == nil {
		t.Error("NSELF_NO_MONOREPO candidate passed the gate")
	}
	// A binding whose flag is missing.
	err = b.Check([]FlagRef{{"build", "no-monorepo"}})
	if err == nil || !strings.Contains(err.Error(), "binding init --domain -> BASE_DOMAIN: no such flag") {
		t.Errorf("missing bound flag: %v", err)
	}
	// An exemption whose flag is missing.
	err = b.Check([]FlagRef{{"init", "domain"}})
	if err == nil || !strings.Contains(err.Error(), "exemption build --no-monorepo: no such flag") {
		t.Errorf("missing exempt flag: %v", err)
	}
	if _, ok := CandidateKey("color", testKnown); ok {
		t.Error("color is not a known key")
	}
}

func TestBindingsRenderMarkdown(t *testing.T) {
	b, err := Parse([]byte(okFlags), []byte(okExempt), []string{"BASE_DOMAIN", "NSELF_NO_MONOREPO"})
	if err != nil {
		t.Fatal(err)
	}
	md := b.RenderMarkdown()
	for _, want := range []string{"| `nself init` | `--domain` | `BASE_DOMAIN` | local.nself.org |", "| `nself build` | `--no-monorepo` | unread |"} {
		if !strings.Contains(md, want) {
			t.Errorf("render missing %q in:\n%s", want, md)
		}
	}
	if !strings.HasSuffix(md, "\n") {
		t.Error("render must end with a newline")
	}
}
