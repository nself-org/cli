package canon

import (
	"os"
	"strings"
	"testing"
)

func TestSurfaceFieldsLoad(t *testing.T) {
	b, err := os.ReadFile("testdata/surface/valid.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if f.Commands["reset"].Confirm == nil || f.Commands["config export"].Surface != "cli-only" || f.Commands["config set"].SecretArgs[0] != "value" || !f.Commands["logs"].Flags["reveal"].CLIOnly {
		t.Fatalf("surface fields lost: %+v", f.Commands)
	}
}

func TestSurfaceFieldsInvalid(t *testing.T) {
	for _, tc := range []struct{ name, row, want string }{
		{"unknown confirm key", "confirm: {flogs: [yes]}", "flogs"},
		{"bad cli only", "flags: {yes: {cli_only: maybe}}", "maybe"},
		{"bad surface", "surface: sometimes", "surface"},
		{"duplicate confirm", "confirm: {flags: [yes, yes]}", "confirm.flags"},
		{"empty plan", "confirm: {plan: {flag: plan}}", "confirm.plan"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := []byte("schema_version: 1\nverbs: [reset]\ncommands:\n  reset: {canon: core, side_effect: destructive, " + tc.row + "}\n")
			_, err := Parse(b)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s, got %v", tc.want, err)
			}
		})
	}
}
