package commands

import "testing"

// Purpose: flag tests for `nself backup stream` (P7-PROD-71).
// Inputs: the package-level backupStreamCmd.
// Outputs: none (t.Error on a missing flag or a wrong default).

// TestBackupStreamFlags checks the heartbeat flags next to the existing ones:
// --heartbeat-to takes a remote with no default, --heartbeat-required is off
// by default (a heartbeat outage must not fail a backup), --recipient repeats.
func TestBackupStreamFlags(t *testing.T) {
	fs := backupStreamCmd.Flags()
	for name, spec := range map[string][2]string{
		"to":                 {"string", ""},
		"recipient":          {"stringArray", "[]"},
		"dry-run":            {"bool", "false"},
		"no-encrypt":         {"bool", "false"},
		"heartbeat-to":       {"string", ""},
		"heartbeat-required": {"bool", "false"},
	} {
		f := fs.Lookup(name)
		if f == nil {
			t.Errorf("flag --%s is not registered on `backup stream`", name)
			continue
		}
		if f.Value.Type() != spec[0] {
			t.Errorf("--%s has type %s, want %s", name, f.Value.Type(), spec[0])
		}
		if f.DefValue != spec[1] {
			t.Errorf("--%s default = %q, want %q", name, f.DefValue, spec[1])
		}
	}
}
