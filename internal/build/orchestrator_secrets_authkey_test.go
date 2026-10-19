package build

// D-0660: a new v1.5 project gets AUTH_ENCRYPTION_KEY once, in .env.secrets;
// nothing else ever gets one generated. All values are test fixtures.

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// authKeyLines returns every AUTH_ENCRYPTION_KEY=<value> line of .env.secrets.
func authKeyLines(t *testing.T, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".env.secrets"))
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(l, compose.AuthEncryptionKeyEnv+"="); ok {
			out = append(out, v)
		}
	}
	return out
}

func TestAuthEncryptionKeyGeneration(t *testing.T) {
	const given = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		name      string
		v15       bool
		setup     func(t *testing.T, dir string)
		wantNew   bool   // a key is generated and persisted
		wantValue string // the value that must stay (when not generated)
	}{
		{name: "new v1.5 project", v15: true, wantNew: true},
		{name: "v1.4 project", v15: false},
		{name: "key already in .env", v15: true, setup: func(t *testing.T, dir string) {
			testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\nAUTH_ENCRYPTION_KEY="+given+"\n", 0600)
		}},
		{name: "key already in .env.secrets", v15: true, wantValue: given, setup: func(t *testing.T, dir string) {
			testWriteFile(t, filepath.Join(dir, ".env.secrets"), "AUTH_ENCRYPTION_KEY="+given+"\n", 0600)
		}},
		{name: "malformed key in .env is left alone", v15: true, setup: func(t *testing.T, dir string) {
			testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\nAUTH_ENCRYPTION_KEY=short\n", 0600)
		}},
		{name: "key exported in the process environment", v15: true, setup: func(t *testing.T, dir string) {
			t.Setenv("AUTH_ENCRYPTION_KEY", given)
		}},
		{name: "project built before keeps its image", v15: true, setup: func(t *testing.T, dir string) {
			testWriteFile(t, filepath.Join(dir, ".nself", "compose.env"), "PROJECT_NAME=test\n", 0600)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("file modes")
			}
			clearSecretEnv(t)
			t.Setenv("AUTH_ENCRYPTION_KEY", "")
			_ = os.Unsetenv("AUTH_ENCRYPTION_KEY")
			compattest.Set(t, tc.v15)
			dir := t.TempDir()
			testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\n", 0600)
			if tc.setup != nil {
				tc.setup(t, dir)
			}
			first := buildOnce(t, dir)
			lines := authKeyLines(t, dir)
			if !tc.wantNew {
				if tc.wantValue == "" && len(lines) != 0 {
					t.Fatalf("a key was generated: %d line(s) in .env.secrets", len(lines))
				}
				if tc.wantValue != "" && (len(lines) != 1 || lines[0] != tc.wantValue) {
					t.Fatalf(".env.secrets key changed: %v", lines)
				}
				return
			}
			if len(lines) != 1 || !hex64.MatchString(lines[0]) {
				t.Fatalf("want one 64-hex key in .env.secrets, got %d line(s)", len(lines))
			}
			if got := first.Passthrough[compose.AuthEncryptionKeyEnv]; got != lines[0] {
				t.Error("this build does not render with the key it persisted")
			}
			if entry, missing := compose.AuthEntry(first.Passthrough); entry != "auth" || missing {
				t.Errorf("AuthEntry = %q, keyMissing %v; want the current release", entry, missing)
			}
			if info, err := os.Stat(filepath.Join(dir, ".env.secrets")); err != nil || info.Mode().Perm() != 0600 {
				t.Errorf(".env.secrets mode: %v %v", info, err)
			}
			// A rebuild reads the key back: same value, no second line.
			second := buildOnce(t, dir)
			again := authKeyLines(t, dir)
			if len(again) != 1 || again[0] != lines[0] || second.Passthrough[compose.AuthEncryptionKeyEnv] != lines[0] {
				t.Errorf("the key changed or was duplicated on rebuild: %d line(s)", len(again))
			}
		})
	}
}
