package invoke

// Purpose: regression tests for the D-0367 post-merge findings in the invoke path.
// Constraints: the stub child and the gate fixture from the other tests; no network.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestChildEnvCaseInsensitive(t *testing.T) {
	parent := []string{"nself_mcp_token=x", "Nself_V15=0", "KEEP=1"}
	cases := []struct {
		name       string
		insensitve bool
		wantKept   []string
	}{
		{"windows drops any spelling", true, []string{"KEEP=1"}},
		{"case sensitive keeps lower-case spellings", false, []string{"nself_mcp_token=x", "Nself_V15=0", "KEEP=1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			old := envCaseInsensitive
			envCaseInsensitive = c.insensitve
			t.Cleanup(func() { envCaseInsensitive = old })
			env, err := childEnv(parent, TransportMCP, "", 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, kv := range []string{"nself_mcp_token=x", "Nself_V15=0", "KEEP=1"} {
				if got, want := contains(env, kv), contains(c.wantKept, kv); got != want {
					t.Errorf("%s present=%v, want %v in %v", kv, got, want, env)
				}
			}
		})
	}
}

func TestScrubQuotedForms(t *testing.T) {
	cases := []struct {
		name, text, secret string
		scrubbed           bool
	}{
		{"go-quoted control byte", `bad "a\ab" here`, "a\x07b", true},
		{"go-quoted non-ascii escapes", `v=caf\u00e9x`, "caféx", true},
		{"plain secret", "tok=a\x07b", "a\x07b", true},
		{"other text stays", `bad "a\ab" here`, "zzz", false},
	}
	for _, c := range cases {
		got := scrub(c.text, []string{c.secret})
		if has := strings.Contains(got, Redacted); has != c.scrubbed {
			t.Errorf("%s: scrub(%q) = %q, scrubbed=%v want %v", c.name, c.text, got, has, c.scrubbed)
		}
		if c.scrubbed && (strings.Contains(got, `\ab`) || strings.Contains(got, `\u00e9x`) || strings.Contains(got, `éx`)) {
			t.Errorf("%s: a spelling of the secret survived: %q", c.name, got)
		}
	}
}

func TestNotOneDocumentTrim(t *testing.T) {
	cases := []struct {
		name, stdout string
		ok           bool
	}{
		{"space and newline padding", " {}\n", true},
		{"vertical tab and form feed", "\v{}\f", false},
		{"next line padding", "\u0085{}\u0085", false},
		{"no-break space padding", " {} ", false},
	}
	for _, c := range cases {
		got := notOneDocument(Result{Stdout: []byte(c.stdout)})
		if (got == "") != c.ok {
			t.Errorf("%s: notOneDocument(%q) = %q, ok want %v", c.name, c.stdout, got, c.ok)
		}
	}
}

// TestRefusedReadIsAudited pins that a read whose child run is refused leaves
// an audit line (decision refuse, the error code) while a read that ran does not.
func TestRefusedReadIsAudited(t *testing.T) {
	e := newGateEnv(t, "STUBCLI_MODE", "text")
	res, err := Invoke(context.Background(), e.reg, "gx read", req(t, `{}`), Options{Dir: e.dir, Guard: e.guard})
	if err != nil {
		t.Fatal(err)
	}
	lines := e.auditLines()
	if res.ErrorCode != "E422" || len(lines) != 1 || lines[0]["decision"] != string(DecisionRefuse) || lines[0]["error_code"] != "E422" {
		t.Fatalf("a refused read must be audited: code=%q lines=%v", res.ErrorCode, lines)
	}
	if lines[0]["side_effect"] != "read" {
		t.Errorf("side_effect = %v", lines[0]["side_effect"])
	}
	// Positive control: a read that runs fine writes nothing.
	ok := newGateEnv(t)
	if r := ok.call("gx read", `{}`); r.ErrorCode != "" {
		t.Fatalf("control read refused: %s", r.Stdout)
	}
	if got := ok.auditLines(); len(got) != 0 {
		t.Errorf("a read that ran must not be audited: %v", got)
	}
}

// TestRefusedReadTimeoutIsAudited is the E423 case of the same rule.
func TestRefusedReadTimeoutIsAudited(t *testing.T) {
	e := newGateEnv(t, "STUBCLI_MODE", "sleep")
	_, err := Invoke(context.Background(), e.reg, "gx read", req(t, `{}`), Options{Dir: e.dir, Guard: e.guard, Timeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	lines := e.auditLines()
	if len(lines) != 1 || lines[0]["decision"] != string(DecisionRefuse) || lines[0]["error_code"] != "E423" {
		t.Fatalf("a timed-out read must be audited: %v", lines)
	}
}

func TestBuildArgvEmptySliceItemAndIntegerText(t *testing.T) {
	reg := fixtureRegistry(t)
	echo := mustCmd(t, reg, "fx echo")
	if _, err := BuildArgv(echo, req(t, `{"args":["w"],"flags":{"tags":["a",""]}}`)); err == nil || !strings.Contains(err.Error(), "tags") {
		t.Errorf("an empty stringSlice item must be refused, got %v", err)
	}
	if _, err := BuildArgv(echo, req(t, `{"args":["w"],"flags":{"items":[""]}}`)); err != nil {
		t.Errorf("an empty item of a plain string slice stays allowed: %v", err)
	}
	for in, want := range map[string]string{"-0": "0", "007": "7", "-4": "-4", "18446744073709551615": "18446744073709551615"} {
		got, ok := integerText(json.Number(in), false)
		if !ok || got != want {
			t.Errorf("integerText(%q) = %q, %v, want %q", in, got, ok, want)
		}
	}
	argv, err := BuildArgv(echo, req(t, `{"args":["w"],"flags":{"count":-0}}`))
	if err != nil || !contains(argv, "--count=0") {
		t.Errorf("-0 must render as 0: %v %v", argv, err)
	}
}
