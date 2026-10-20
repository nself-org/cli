package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// D-1396: a caller without HOME (cron, a bare systemd unit, a scrubbed child)
// used to resolve plugins under the world-writable /tmp/.nself/plugins/bin and
// exec whatever sat there with its own environment. With no HOME there is no
// plugin directory: the router refuses instead of guessing one.
func TestPluginRouterRefusesWithoutHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME semantics are unix only")
	}
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	if got := pluginBinDir(); got != "" {
		t.Fatalf("pluginBinDir() = %q with no HOME, want empty (no /tmp fallback)", got)
	}
	var ce *errs.CLIError
	err := ProxyCommand("demo", nil)
	if !errors.As(err, &ce) || ce.Code != "E406" || !strings.Contains(err.Error(), "HOME") {
		t.Fatalf("ProxyCommand without HOME = %v, want E406 naming HOME", err)
	}
	err = ProxyBinary("nself-demo", "demo", nil)
	if !errors.As(err, &ce) || ce.Code != "E406" || !strings.Contains(err.Error(), "HOME") {
		t.Fatalf("ProxyBinary without HOME = %v, want E406 naming HOME", err)
	}
	if got := pluginEnvForCommand("demo"); got != nil {
		t.Fatalf("pluginEnvForCommand without HOME = %v, want nil (no manifest read from the cwd)", got)
	}
}

// The router must not exec a plugin binary or run it from a directory that
// another local user could have replaced.
func TestProxyBinaryAtRefusesUntrustedModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are unix only")
	}
	cases := []struct {
		name     string
		fileMode os.FileMode
		binMode  os.FileMode
		rootMode os.FileMode
	}{
		{"group-writable binary", 0o775, 0o755, 0o755},
		{"world-writable binary", 0o777, 0o755, 0o755},
		{"world-writable bin dir", 0o755, 0o777, 0o755},
		{"group-writable plugins dir", 0o755, 0o755, 0o775},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "plugins")
			bin := filepath.Join(root, "bin")
			if err := os.MkdirAll(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(bin, "nself-demo")
			if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			for p, m := range map[string]os.FileMode{path: tc.fileMode, bin: tc.binMode, root: tc.rootMode} {
				if err := os.Chmod(p, m); err != nil {
					t.Fatal(err)
				}
			}
			var ce *errs.CLIError
			if err := ProxyBinaryAt(path, root, "demo", nil); !errors.As(err, &ce) || ce.Code != "E406" {
				t.Fatalf("ProxyBinaryAt = %v, want E406", err)
			}
		})
	}
}

// A binary owned by another user is refused even when its mode looks safe.
func TestExecTrustProblemOwner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ownership is unix only")
	}
	root := t.TempDir()
	path := filepath.Join(root, "nself-demo")
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolvedRoot, _ := filepath.EvalSymlinks(root)
	resolved, _ := filepath.EvalSymlinks(path)
	uid := os.Geteuid()
	if p := execTrustProblem(resolved, resolvedRoot, uid); p != "" {
		t.Fatalf("own file refused: %s", p)
	}
	if uid == 0 {
		t.Skip("root owns everything it may exec")
	}
	if p := execTrustProblem(resolved, resolvedRoot, uid+1); p == "" {
		t.Fatal("file owned by another uid was trusted")
	}
}
