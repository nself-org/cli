package commands

// db_remote_plugin_test.go — D-0232: `--plugin` must reach the remote host.
// Without it a remote `db migrate up --plugin X --env staging` ran every
// migration, not just the plugin's.

import (
	"context"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestRemoteUp_PluginIsForwarded(t *testing.T) {
	sent, err := remoteUp(t, map[string]string{"plugin": "claw"})
	if err != nil {
		t.Fatal(err)
	}
	want := "cd '/opt/nself' && nself 'db' 'migrate' 'up' '--plugin' 'claw'"
	if len(sent) != 1 || sent[0] != want {
		t.Fatalf("remote command = %q, want exactly [%q]", sent, want)
	}
}

func TestRemoteUp_PluginWithDirAndDryRun_ArgvIsExact(t *testing.T) {
	sent, err := remoteUpWith(t, "1.4.12", capableRemote(), map[string]string{"plugin": "claw", "migration-dir": "migrations", "dry-run": "true"})
	if err != nil {
		t.Fatal(err)
	}
	wantRemote(t, sent, probeCaps, "cd '/opt/nself' && nself 'db' 'migrate' 'up' '--migration-dir' 'migrations' '--dry-run' '--plugin' 'claw'")
}

func TestRemoteUp_InvalidPluginSendsNoSSH(t *testing.T) {
	for _, bad := range []string{"../x", "a b", "claw; rm -rf /"} {
		sent, err := remoteUp(t, map[string]string{"plugin": bad})
		if err == nil {
			t.Errorf("--plugin %q: expected an error", bad)
		}
		if len(sent) != 0 {
			t.Errorf("--plugin %q: sent %d SSH command(s), want 0: %q", bad, len(sent), sent)
		}
	}
}

func TestRemoteUp_NoPlugin_ArgvUnchanged(t *testing.T) {
	sent, err := remoteUp(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "cd '/opt/nself' && nself 'db' 'migrate' 'up'"
	if len(sent) != 1 || sent[0] != want {
		t.Fatalf("remote command = %q, want exactly [%q]", sent, want)
	}
}

func TestRemoteStatus_PluginIsForwarded(t *testing.T) {
	withLocalVersion(t, "dev")
	writeRemoteInventory(t)
	var sent []string
	withStubbedSSH(t, func(ctx context.Context, sshArgs []string) (string, error) {
		sent = append(sent, sshArgs[len(sshArgs)-1])
		return "ok", nil
	})
	cmd := &cobra.Command{Use: "status"}
	addDBRemoteFlags(cmd)
	cmd.SetContext(context.Background())
	cmd.Flags().String("migration-dir", "", "")
	cmd.Flags().Bool("detect", false, "")
	cmd.Flags().String("plugin", "", "")
	for k, v := range map[string]string{"env": "staging", "plugin": "claw"} {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set --%s: %v", k, err)
		}
	}
	if err := runDBMigrateStatus(cmd, nil); err != nil {
		t.Fatal(err)
	}
	want := "cd '/opt/nself' && nself 'db' 'migrate' 'status' '--plugin' 'claw'"
	if len(sent) != 1 || sent[0] != want {
		t.Fatalf("remote command = %q, want exactly [%q]", sent, want)
	}
}

// A caller that already forwards --plugin is not given a second copy, and the
// caller's backing array is never written to.
func TestWithRemotePluginFlag_NoDuplicateNoMutation(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("plugin", "claw", "")
	in := make([]string, 3, 8)
	copy(in, []string{"db", "migrate", "up"})
	out, err := withRemotePluginFlag(cmd, in)
	if err != nil || len(out) != 5 || out[3] != "--plugin" || out[4] != "claw" {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if back := in[:5]; back[3] != "" || back[4] != "" {
		t.Errorf("caller backing array was mutated: %q", back)
	}
	again, err := withRemotePluginFlag(cmd, out)
	if err != nil || len(again) != 5 {
		t.Fatalf("duplicate forwarded: %q, err = %v", again, err)
	}
}

func TestWithRemotePluginFlag_DistinguishesFlagsFromValues(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		forwarded bool
	}{
		{"string value", []string{"--migration-dir", "--plugin"}, false},
		{"inline string value", []string{"--migration-dir=--plugin"}, false},
		{"boolean then plugin", []string{"--dry-run", "--plugin", "claw"}, true},
		{"inline plugin", []string{"--plugin=claw"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "up"}
			cmd.Flags().String("plugin", "claw", "")
			cmd.Flags().String("migration-dir", "", "")
			cmd.Flags().Bool("dry-run", false, "")
			out, err := withRemotePluginFlag(cmd, tc.args)
			want := append([]string(nil), tc.args...)
			if !tc.forwarded {
				want = append(want, "--plugin", "claw")
			}
			if err != nil || !reflect.DeepEqual(out, want) {
				t.Fatalf("out = %q, err = %v; want %q", out, err, want)
			}
		})
	}
}

func TestWithRemotePluginFlag_PositionalPluginIsNotAFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "up"}
	cmd.Flags().String("plugin", "claw", "")
	out, err := withRemotePluginFlag(cmd, []string{"db", "migrate", "up", "--", "--plugin"})
	want := []string{"db", "migrate", "up", "--plugin", "claw", "--", "--plugin"}
	if err != nil || !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %q, err = %v; want %q", out, err, want)
	}
}

func TestRemoteUp_PluginLikeDirectoryStillValidatesPlugin(t *testing.T) {
	sent, err := remoteUp(t, map[string]string{"migration-dir": "--plugin", "plugin": "../x"})
	if err == nil || len(sent) != 0 {
		t.Fatalf("err = %v, sent = %q; want validation error and no SSH", err, sent)
	}
}

func TestWithRemotePluginFlag_ValidatesAlreadyForwardedPlugin(t *testing.T) {
	cmd := &cobra.Command{Use: "up"}
	cmd.Flags().String("plugin", "../x", "")
	if _, err := withRemotePluginFlag(cmd, []string{"--plugin", "../x"}); err == nil {
		t.Fatal("already forwarded plugin bypassed local validation")
	}
}
