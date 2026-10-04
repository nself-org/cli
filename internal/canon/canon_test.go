package canon

import (
	"errors"
	"strings"
	"testing"
)

const goodDoc = `
schema_version: 1
verbs: [status, doctor]
commands:
  status:
    canon: core
    side_effect: read
    exit_codes: {"2": "unhealthy"}
    exit_codes_v15: {"10": "unhealthy"}
  doctor:
    canon: core
    side_effect: read
    flags:
      fix: {side_effect: write}
      deploy: {output: stream}
  config get: {side_effect: read}
  oldstatus: {canon: deprecated-shim, target: status}
`

var adrVerbs = []string{"init", "start", "stop", "restart", "status", "logs", "doctor", "build", "reset", "clean", "add", "remove", "config", "db", "backup", "deploy", "update", "license", "mcp", "exec"}

func TestLoadEmbedded(t *testing.T) {
	f, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := strings.Join(f.Verbs, " "), strings.Join(adrVerbs, " "); got != want {
		t.Errorf("verbs = %s, want ADR 0016 order %s", got, want)
	}
	if len(f.Verbs) > MaxVerbs {
		t.Errorf("%d verbs exceeds ceiling %d", len(f.Verbs), MaxVerbs)
	}
	if f.SchemaVersion != 1 {
		t.Errorf("schema_version = %d", f.SchemaVersion)
	}
	f2, _ := Load()
	if f != f2 {
		t.Error("Load is not memoised")
	}
}

func TestParseGood(t *testing.T) {
	f, err := Parse([]byte(goodDoc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := f.Commands["status"].ExitCodesV15["10"]; got != "unhealthy" {
		t.Errorf("exit_codes_v15 = %q", got)
	}
	if got := f.Commands["doctor"].Flags["deploy"].Output; got != OutputStream {
		t.Errorf("flag output = %q", got)
	}
}

func TestParseStrictUnknownKey(t *testing.T) {
	for name, doc := range map[string]string{
		"top level":   "schema_version: 1\nverbs: [a]\nbogus: 1\ncommands: {}\n",
		"entry":       "schema_version: 1\nverbs: [a]\ncommands:\n  a: {canon: core, side_efect: read}\n",
		"flag":        "schema_version: 1\nverbs: [a]\ncommands:\n  a: {canon: core, flags: {x: {nope: 1}}}\n",
		"empty":       "",
		"duplicate":   "schema_version: 1\nverbs: [a]\ncommands:\n  a: {canon: core}\n  a: {canon: core}\n",
		"wrong shape": "schema_version: 1\nverbs: a\ncommands: {}\n",
	} {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: Parse accepted an invalid document", name)
		}
	}
}

func TestRank(t *testing.T) {
	order := []string{SideEffectRead, SideEffectWrite, SideEffectRemote, SideEffectDestructive}
	for i, s := range order {
		if Rank(s) != i {
			t.Errorf("Rank(%q) = %d, want %d", s, Rank(s), i)
		}
	}
	if Rank("bogus") != -1 || Rank("") != -1 {
		t.Error("unknown side effect must rank -1")
	}
}

func TestValidationErrorAggregates(t *testing.T) {
	_, err := Parse([]byte("schema_version: 2\nverbs: []\ncommands:\n  a: {}\n  b c: {canon: core}\n"))
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *ValidationError, got %T %v", err, err)
	}
	if len(ve.Problems) < 4 {
		t.Errorf("want every problem aggregated, got %d: %v", len(ve.Problems), ve.Problems)
	}
	if NewValidationError(nil) != nil {
		t.Error("NewValidationError(nil) must be a nil error")
	}
}
