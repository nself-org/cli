package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorCompatCheck(t *testing.T) {
	for _, tc := range []struct{ name, yaml, running, status, contains string }{
		{"older", "cli_min_version: 99.0.0\n", "1.4.99", "warn", "[E060] project needs nself ≥ 99.0.0 (running 1.4.99); Fix: nself update"},
		{"newer", "cli_min_version: 0.0.1\n", "1.4.99", "pass", "0.0.1"},
		{"equal", "cli_min_version: v1.4.99\n", "v1.4.99", "pass", "v1.4.99"},
		{"release-after-rc", "cli_min_version: 1.4.99-rc.1\n", "1.4.99", "pass", "1.4.99-rc.1"},
		{"rc-before-release", "cli_min_version: 1.4.99\n", "1.4.99-rc.1", "warn", "[E060]"},
		{"dev", "cli_min_version: 99.0.0\n", "dev", "skip", "development build"},
		{"pseudo", "cli_min_version: 99.0.0\n", "v0.0.0-20261009-abcdef", "skip", "development build"},
		{"absent", "app: test\n", "1.4.99", "", ""},
		{"invalid", "cli_min_version: soon\n", "1.4.99", "warn", "[E061]"},
		{"no-file", "", "1.4.99", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.yaml != "" {
				if err := os.WriteFile(filepath.Join(dir, "nself.yaml"), []byte(tc.yaml), 0600); err != nil {
					t.Fatal(err)
				}
			}
			results := checkDoctorCompatVersion(dir, tc.running, false)
			if tc.status == "" {
				if len(results) != 0 {
					t.Fatalf("expected no row, got %+v", results)
				}
				return
			}
			if len(results) != 1 || results[0].Status != tc.status || !strings.Contains(results[0].Message, tc.contains) {
				t.Fatalf("got %+v; want status %s and %q", results, tc.status, tc.contains)
			}
		})
	}
}
