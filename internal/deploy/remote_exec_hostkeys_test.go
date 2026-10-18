package deploy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Runner provisioning must keep first-seen keys out of the operator's trust
// file, including when nself's own TOFU file cannot be opened.
func TestRunRemoteCommandUsesDeployTofuV15(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		name := "available"
		if unavailable {
			name = "unavailable"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("NSELF_V15", "1")
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			tofu := filepath.Join(home, ".config", "nself", "deploy_tofu_hosts")
			if unavailable {
				if err := os.MkdirAll(tofu, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			operator := filepath.Join(home, ".ssh", "known_hosts")
			if err := os.MkdirAll(filepath.Dir(operator), 0o700); err != nil {
				t.Fatal(err)
			}
			const trusted = "operator's existing trusted key\n"
			if err := os.WriteFile(operator, []byte(trusted), 0o600); err != nil {
				t.Fatal(err)
			}
			read := installFakeSSHTools(t)
			rt := RemoteTarget{SSHTarget: "deploy@203.0.113.7:2222", KeyPath: "/k/id"}
			if _, err := RunRemoteCommand(context.Background(), rt, "provision runner"); err != nil {
				t.Fatal(err)
			}
			userHosts := tofu
			if unavailable {
				userHosts = os.DevNull
			}
			assertCalls(t, read(), []recordedCall{{
				tool: "ssh", probe: "inherited",
				args: []string{"-i", "/k/id", "-o", "StrictHostKeyChecking=accept-new",
					"-o", "UserKnownHostsFile=" + filepath.ToSlash(userHosts),
					"-o", "GlobalKnownHostsFile=" + filepath.ToSlash(operator),
					"-o", "ForwardAgent=no", "-p", "2222", "--", "deploy@203.0.113.7", "provision runner"},
			}})
			if !unavailable {
				info, err := os.Stat(tofu)
				if err != nil {
					t.Fatalf("TOFU file was not created: %v", err)
				}
				if info.Mode().Perm() != 0o600 {
					t.Errorf("TOFU file permissions = %o, want 600", info.Mode().Perm())
				}
			}
			got, err := os.ReadFile(operator)
			if err != nil || string(got) != trusted {
				t.Fatalf("operator trust file changed: %q, %v", got, err)
			}
		})
	}
}
