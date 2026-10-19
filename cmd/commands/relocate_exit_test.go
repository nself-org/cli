package commands

// Purpose: regression for D-0909 — relocateBreakout must surface the plugin's
// own exit status, not flatten it. The shared stubScript always exits 0, so
// nothing else proves a failing break-out plugin fails `nself <old spelling>`.
// Inputs: a stub nself-ci that exits with the table's status.
// Constraints: POSIX shell stub (skipped on Windows by relocateSetup); throwaway
// HOME and project; no network.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin"
)

func TestRelocateBreakoutPropagatesPluginExit(t *testing.T) {
	for _, status := range []int{0, 1, 2, 3, 7, 64} {
		t.Run(fmt.Sprintf("exit_%d", status), func(t *testing.T) {
			_, calls, _ := relocateSetup(t, "", nil)
			t.Setenv("HOME", t.TempDir())
			autoAddPlugin = recordOver(calls, func(slug, _ string) (string, error) {
				p := plugin.PublishedBinaryPath("nself-" + slug)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					return "", err
				}
				return p, os.WriteFile(p, []byte(fmt.Sprintf("#!/bin/sh\nexit %d\n", status)), 0o755)
			})
			handled, err := relocateBreakout("ci", []string{"--check"})
			if !handled {
				t.Fatalf("handled=false: the break-out row was not used")
			}
			if got := errs.ExitCodeFor(err); got != status {
				t.Fatalf("plugin exit %d surfaced as %d (err=%v)", status, got, err)
			}
			if (status == 0) != (err == nil) {
				t.Fatalf("plugin exit %d: err=%v", status, err)
			}
		})
	}
}
