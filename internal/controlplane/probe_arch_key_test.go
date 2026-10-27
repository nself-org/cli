package controlplane

// Purpose: the arch probe offers the same SSH identity the deploy uses (D-0614).

import (
	"path/filepath"
	"testing"
)

func TestArchProbeKeyMatchesDeployKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir on Windows
	t.Setenv("NSELF_DEPLOY_SSH_KEY", "")
	t.Setenv("NSELF_DEPLOY_KEY_PATH", "")
	t.Setenv("ARCH_PROBE_REF_KEY", "")

	if got, want := archProbeKey(Server{}), filepath.Join(home, ".ssh", "id_ed25519"); got != want {
		t.Errorf("default key = %q, want %q", got, want)
	}
	t.Setenv("NSELF_DEPLOY_KEY_PATH", "/keys/legacy-deploy")
	if got := archProbeKey(Server{}); got != "/keys/legacy-deploy" {
		t.Errorf("key set only via NSELF_DEPLOY_KEY_PATH: probe uses %q", got)
	}
	t.Setenv("NSELF_DEPLOY_SSH_KEY", "/keys/other")
	if got := archProbeKey(Server{}); got != "/keys/legacy-deploy" {
		t.Errorf("both set: probe uses %q, deploy prefers NSELF_DEPLOY_KEY_PATH", got)
	}
	t.Setenv("ARCH_PROBE_REF_KEY", "/keys/server-ref")
	if got := archProbeKey(Server{SSHKeyRef: "ARCH_PROBE_REF_KEY"}); got != "/keys/server-ref" {
		t.Errorf("server key ref: probe uses %q", got)
	}
}
