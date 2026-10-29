package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/errs"
)

func TestMachinePluginExecutionWithoutSecretsRuns(t *testing.T) {
	// A machine-invoked plugin call that carries no restored secret bundle
	// must still run: the guard protects secrets, not the transport.
	t.Setenv("NSELF_INVOKED_BY", "mcp")
	// execPluginBinary will reach os/exec; use /bin/sh -c true as a portable
	// no-op that exists on every POSIX system (and skip on Windows).
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only test")
	}
	if err := execPluginBinary("/bin/sh", "demo", []string{"-c", "true"}); err != nil {
		t.Fatalf("machine-invoked plugin without secrets should run, got %v", err)
	}
}

func TestMachinePluginExecutionWithSecretsRefused(t *testing.T) {
	// When a secret bundle was restored, argv carries real values and the
	// plugin must be refused until it supports the handoff protocol (D-0651).
	t.Setenv("NSELF_INVOKED_BY", "mcp")
	// RestoreSecretArgv sets the package flag when it replaces placeholders;
	// simulate that state directly because constructing a real bundle here
	// would duplicate the secretenv tests.
	invoke.TestingSetSecretsRestored(true)
	t.Cleanup(func() { invoke.TestingSetSecretsRestored(false) })
	err := execPluginBinary("missing-plugin-binary", "demo", []string{"--token=fixture"})
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E421" {
		t.Fatalf("expected E421 when secrets were restored, got %v", err)
	}
}

func TestProxyBinaryAtRejectsEscape(t *testing.T) {
	// ProxyBinaryAt must refuse a candidate that resolves outside rootDir;
	// this is the containment check that prevents symlink escapes.
	root := t.TempDir()
	outside := filepath.Join(os.TempDir(), "nself-proxy-binary-at-test-outside")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write outside binary: %v", err)
	}
	t.Cleanup(func() { os.Remove(outside) })
	err := ProxyBinaryAt(outside, root, "demo", nil)
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E406" {
		t.Fatalf("expected E406 for escape, got %v", err)
	}
}
