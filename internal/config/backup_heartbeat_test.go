package config

import "testing"

// TestBackupHeartbeatRemote covers the environment-only heartbeat remote.
func TestBackupHeartbeatRemote(t *testing.T) {
	var b BackupConfig
	t.Setenv(EnvBackupHeartbeatRemote, "")
	if got := b.HeartbeatRemote(); got != "" {
		t.Errorf("unset: got %q, want empty", got)
	}
	t.Setenv(EnvBackupHeartbeatRemote, "  r2hb:hb \n")
	if got := b.HeartbeatRemote(); got != "r2hb:hb" {
		t.Errorf("set: got %q, want r2hb:hb", got)
	}
}

// TestBackupHeartbeatRemoteIsKnown keeps the variable out of the unknown-var warning.
func TestBackupHeartbeatRemoteIsKnown(t *testing.T) {
	for _, k := range knownEnvVars {
		if k == EnvBackupHeartbeatRemote {
			return
		}
	}
	t.Errorf("%s is not in knownEnvVars", EnvBackupHeartbeatRemote)
}
