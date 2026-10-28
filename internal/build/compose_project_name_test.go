package build

// Purpose: D-0448 — the compose project name is PROJECT_NAME, so a differing
// COMPOSE_PROJECT_NAME in .env gets a warning that names the volumes.

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeProjectNameWarning(t *testing.T) {
	for _, tc := range []struct {
		name, env string
		want      bool
	}{
		{"no .env", "", false},
		{"unset", "PROJECT_NAME=bar\n", false},
		{"empty", "PROJECT_NAME=bar\nCOMPOSE_PROJECT_NAME=\n", false},
		{"same", "PROJECT_NAME=bar\nCOMPOSE_PROJECT_NAME=bar\n", false},
		{"differs", "PROJECT_NAME=bar\nCOMPOSE_PROJECT_NAME=foo\n", true},
		{"differs quoted", "PROJECT_NAME=bar\nCOMPOSE_PROJECT_NAME=\"foo\"\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.env != "" {
				testWriteFile(t, filepath.Join(dir, ".env"), tc.env, 0600)
			}
			got := composeProjectNameWarning(dir, "bar")
			if (got != "") != tc.want {
				t.Fatalf("warning = %q, want warning: %v", got, tc.want)
			}
			if tc.want {
				for _, part := range []string{"foo_postgres_data", "bar_postgres_data", "PROJECT_NAME=bar"} {
					if !strings.Contains(got, part) {
						t.Errorf("warning lacks %q: %s", part, got)
					}
				}
			}
		})
	}
}
