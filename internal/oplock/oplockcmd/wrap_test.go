//go:build !windows

package oplockcmd

// Tests for the cobra binding: the lock is held while the body runs and gone
// after, only inside a project, only for a command with a registry entry, and
// flag escalation decides the class.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/oplock"
	"github.com/spf13/cobra"
)

func str(s string) *string { return &s }

func newTree(entry *cmdregistry.Command, body func(*cobra.Command) error) (*cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "nself"}
	c := &cobra.Command{Use: "thing"}
	c.Flags().Bool("apply", false, "x")
	c.Flags().Bool("watch", false, "x")
	c.Flags().String("name", "", "x")
	c.RunE = Wrap(func(*cobra.Command) (*cmdregistry.Command, error) {
		if entry == nil {
			return nil, errors.New("no entry")
		}
		return entry, nil
	}, func(cmd *cobra.Command, _ []string) error { return body(cmd) })
	root.AddCommand(c)
	return root, c
}

func projectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("ENV=dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

func write() *cmdregistry.Command {
	return &cmdregistry.Command{SideEffect: canon.SideEffectWrite, Output: canon.OutputDocument}
}

func TestOpLockWrapHoldsDuringBodyOnly(t *testing.T) {
	compattest.Set(t, true)
	dir := projectDir(t)
	t.Setenv(oplock.EnvToken, "")
	_ = os.Unsetenv(oplock.EnvToken)
	var during error
	root, _ := newTree(write(), func(cmd *cobra.Command) error {
		if oplock.FromContext(cmd.Context()) == nil {
			t.Error("no lock in the command context")
		}
		_ = os.Unsetenv(oplock.EnvToken) // another process has no token
		_, during = oplock.Acquire(context.Background(), dir, oplock.Opts{Command: "other"})
		return nil
	})
	root.SetArgs([]string{"thing"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(during, oplock.ErrHeld) {
		t.Fatalf("body must run under the lock, got %v", during)
	}
	l, err := oplock.Acquire(context.Background(), dir, oplock.Opts{Command: "after"})
	if err != nil {
		t.Fatalf("lock must be gone after the body: %v", err)
	}
	l.Release()
}

func TestOpLockWrapTakesNothingWithoutProjectOrEntry(t *testing.T) {
	compattest.Set(t, true)
	bare := t.TempDir()
	t.Chdir(bare)
	ran := 0
	root, _ := newTree(write(), func(*cobra.Command) error { ran++; return nil })
	root.SetArgs([]string{"thing"})
	if err := root.Execute(); err != nil || ran != 1 {
		t.Fatalf("outside a project: err=%v ran=%d", err, ran)
	}
	if _, err := os.Stat(filepath.Join(bare, ".nself")); err == nil {
		t.Fatal("outside a project .nself was created")
	}
	dir := projectDir(t)
	root, _ = newTree(nil, func(*cobra.Command) error { ran++; return nil })
	root.SetArgs([]string{"thing"})
	if err := root.Execute(); err != nil || ran != 2 {
		t.Fatalf("no registry entry: err=%v ran=%d", err, ran)
	}
	if _, err := os.Stat(filepath.Join(dir, ".nself")); err == nil {
		t.Fatal("a command without an entry took the lock")
	}
	// The root command itself has no entry and never locks.
	rootOnly := &cobra.Command{Use: "nself", RunE: Wrap(func(*cobra.Command) (*cmdregistry.Command, error) { return write(), nil },
		func(*cobra.Command, []string) error { ran++; return nil })}
	rootOnly.SetArgs(nil)
	if err := rootOnly.Execute(); err != nil || ran != 3 {
		t.Fatalf("root: err=%v ran=%d", err, ran)
	}
}

func TestOpLockEffectiveClass(t *testing.T) {
	e := &cmdregistry.Command{SideEffect: canon.SideEffectRead, Output: canon.OutputDocument, Flags: []cmdregistry.Flag{
		{Name: "apply", SideEffect: str(canon.SideEffectDestructive)},
		{Name: "watch", Output: str(canon.OutputStream)},
		{Name: "name", SideEffect: str(canon.SideEffectRemote)},
		{Name: "absent", SideEffect: str(canon.SideEffectWrite)},
	}}
	cases := []struct {
		args      []string
		side, out string
	}{
		{nil, "read", "document"},
		{[]string{"--apply"}, "destructive", "document"},
		{[]string{"--apply=false"}, "read", "document"}, // a false bool is not given
		{[]string{"--watch"}, "read", "stream"},
		{[]string{"--name", "x"}, "remote", "document"},
		{[]string{"--apply", "--name", "x"}, "destructive", "document"}, // a flag only escalates
	}
	for _, c := range cases {
		_, cmd := newTree(e, nil)
		if err := cmd.ParseFlags(c.args); err != nil {
			t.Fatal(err)
		}
		if side, out := Effective(cmd, e); side != c.side || out != c.out {
			t.Errorf("%v: got %s/%s want %s/%s", c.args, side, out, c.side, c.out)
		}
	}
}
