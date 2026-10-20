package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommandProseLinks(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"private", "[plan](https://github.com/nself-org/cli/blob/main/.claude/plan.md)", false},
		{"private-relative", "[plan](../../.claude/plan.md)", false},
		{"private-local", "[plan](.claude/plan.md)", false},
		{"private-reference", "[plan]: ../.claude/plan.md", false},
		{"private-root", "[plan](https://github.com/nself-org/cli/blob/main/.claude)", false},
		{"missing", "https://nself.org/docs/missing/page", false},
		{"existing", "[start](https://nself.org/docs/commands/start)", true},
		{"root", "https://nself.org/docs", true},
		{"placeholder", "http://127.0.0.1:MINIO_PORT", true},
		{"config-example", "`.claude/settings.json`", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "cmd-start.md"), []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			err := validateProseLinks(dir, tc.body)
			if (err == nil) != tc.valid {
				t.Fatalf("error=%v valid=%v", err, tc.valid)
			}
		})
	}
}
