package commands

// Purpose: tests for the --env resolution of `db drift --metadata` and
// `db reconcile` (D-0165): empty/dir/local stay local, a remote name is
// dispatched with the right argv, an unknown name is refused.
// Constraints: ssh is stubbed (withStubbedSSH); no network.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

// resolveWithEnv runs resolveDBMetadataDir on a command whose --env is env.
func resolveWithEnv(t *testing.T, env string, args ...string) (string, bool, error) {
	t.Helper()
	cmd := newDBRemoteTestCmd()
	if env != "" {
		if err := cmd.Flags().Set("env", env); err != nil {
			t.Fatalf("set --env: %v", err)
		}
	}
	return resolveDBMetadataDir(cmd, args...)
}

func evalDir(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestResolveDBMetadataDir_EmptyIsCwd(t *testing.T) {
	proj := t.TempDir()
	t.Chdir(proj)
	dir, handled, err := resolveWithEnv(t, "", "db", "drift", "--metadata")
	if err != nil || handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if evalDir(t, dir) != evalDir(t, proj) {
		t.Errorf("dir = %q, want cwd %q", dir, proj)
	}
}

func TestResolveDBMetadataDir_ExistingDirIsUsed(t *testing.T) {
	t.Chdir(t.TempDir())
	other := t.TempDir()
	dir, handled, err := resolveWithEnv(t, other, "db", "drift", "--metadata")
	if err != nil || handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if dir != other {
		t.Errorf("dir = %q, want %q", dir, other)
	}
}

func TestResolveDBMetadataDir_LocalIsCwd(t *testing.T) {
	proj := t.TempDir()
	t.Chdir(proj)
	dir, handled, err := resolveWithEnv(t, "local", "db", "drift", "--metadata")
	if err != nil || handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if evalDir(t, dir) != evalDir(t, proj) {
		t.Errorf("dir = %q, want cwd %q", dir, proj)
	}
}

func TestResolveDBMetadataDir_UnknownNameErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	_, handled, err := resolveWithEnv(t, "nosuchenv", "db", "drift", "--metadata")
	if !handled || err == nil {
		t.Fatalf("want handled error for an unknown env, got handled=%v err=%v", handled, err)
	}
	if !strings.Contains(err.Error(), "E483") {
		t.Errorf("want E483, got %v", err)
	}
}

func TestResolveDBMetadataDir_RemoteNameDispatches(t *testing.T) {
	withLocalVersion(t, "1.2.0")
	writeRemoteInventory(t)
	var sent []string
	withStubbedSSH(t, func(_ context.Context, sshArgs []string) (string, error) {
		line := sshArgs[len(sshArgs)-1]
		sent = append(sent, line)
		if line == probeVersion {
			return "nself 1.2.0", nil
		}
		return "ok", nil
	})
	cmd := newDBRemoteTestCmd()
	_ = cmd.Flags().Set("env", "staging")
	dir, handled, err := resolveDBMetadataDir(cmd, "db", "reconcile", "--apply")
	if !handled || err != nil || dir != "" {
		t.Fatalf("dir=%q handled=%v err=%v", dir, handled, err)
	}
	last := sent[len(sent)-1]
	if !strings.Contains(last, "nself 'db' 'reconcile' '--apply'") {
		t.Errorf("remote command = %q, want it to run 'db reconcile --apply'", last)
	}
}

// A directory literally named like an env still wins (documented back-compat):
// the old meaning of --env is a project directory.
func TestResolveDBMetadataDir_DirNamedLikeEnvWins(t *testing.T) {
	proj := t.TempDir()
	t.Chdir(proj)
	if err := os.Mkdir(filepath.Join(proj, "staging"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir, handled, err := resolveWithEnv(t, "staging", "db", "drift", "--metadata")
	if err != nil || handled || dir != "staging" {
		t.Fatalf("dir=%q handled=%v err=%v", dir, handled, err)
	}
}

// envelopeTestCmd builds "nself db <name>" so invokedKey matches the registry
// row, with the db remote flags and --json registered.
func envelopeTestCmd(name string) *cobra.Command {
	root := &cobra.Command{Use: "nself"}
	db := &cobra.Command{Use: "db"}
	cmd := &cobra.Command{Use: name}
	addDBRemoteFlags(cmd)
	cmd.Flags().Bool("json", false, "")
	db.AddCommand(cmd)
	root.AddCommand(db)
	cmd.SetContext(context.Background())
	return cmd
}

// With the JSON envelope on, a remote --env is refused as a usage error before
// anything is dispatched; an unknown name keeps its E483; local still runs.
func TestResolveDBMetadataDir_JSONWithRemoteEnvRefused(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	for _, name := range []string{"reconcile", "drift"} {
		t.Run(name, func(t *testing.T) {
			writeRemoteInventory(t)
			dispatched := 0
			orig := dbEnvDispatchFn
			dbEnvDispatchFn = func(*cobra.Command, ...string) (bool, error) { dispatched++; return true, nil }
			t.Cleanup(func() { dbEnvDispatchFn = orig })
			sshCalls := 0
			withStubbedSSH(t, func(context.Context, []string) (string, error) { sshCalls++; return "", nil })

			cmd := envelopeTestCmd(name)
			_ = cmd.Flags().Set("env", "staging")
			_ = cmd.Flags().Set("json", "true")
			_, handled, err := resolveDBMetadataDir(cmd, "db", name)
			if !handled || err == nil || !strings.Contains(err.Error(), "--json is not supported with a remote --env") {
				t.Fatalf("handled=%v err=%v, want the usage refusal", handled, err)
			}
			if got := errs.ExitCodeFor(err); got != errs.ExitUserError {
				t.Errorf("exit code = %d, want %d", got, errs.ExitUserError)
			}
			if dispatched != 0 || sshCalls != 0 {
				t.Errorf("dispatch ran: dispatched=%d ssh=%d", dispatched, sshCalls)
			}

			// Unknown name: still E483, not the usage message.
			cmd2 := envelopeTestCmd(name)
			_ = cmd2.Flags().Set("env", "nosuchenv")
			_ = cmd2.Flags().Set("json", "true")
			if _, h, e := resolveDBMetadataDir(cmd2, "db", name); !h || e == nil || !strings.Contains(e.Error(), "E483") {
				t.Errorf("unknown env: handled=%v err=%v, want E483", h, e)
			}

			dbEnvDispatchFn = orig

			// local + json is allowed and resolves to cwd.
			cmd3 := envelopeTestCmd(name)
			_ = cmd3.Flags().Set("env", "local")
			_ = cmd3.Flags().Set("json", "true")
			if d, h, e := resolveDBMetadataDir(cmd3, "db", name); h || e != nil || d == "" {
				t.Errorf("local: dir=%q handled=%v err=%v", d, h, e)
			}
		})
	}
}
