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
