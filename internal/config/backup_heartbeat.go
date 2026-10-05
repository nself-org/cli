package config

import (
	"os"
	"strings"
)

// Purpose: the heartbeat remote of the backup stream (P7-PROD-71). After a
// successful `nself backup stream` upload, a small `<project>/backup.json`
// object is written to this rclone remote so freshness can be checked off the
// box (contract:cli.backup-heartbeat).
// Inputs: the NSELF_BACKUP_HEARTBEAT_REMOTE environment variable (the project
// .env is already loaded into the process environment by Load).
// Outputs: the remote string, "" when none is set.
// Constraints: environment only. nself.yaml carries no config keys (P7-SURF
// D6), so there is no yaml form. A method on BackupConfig, not a field, so
// the struct and the known-variable table stay untouched; the --heartbeat-to
// flag of `backup stream` and `backup schedule` wins over this value.

// EnvBackupHeartbeatRemote names the variable that selects the heartbeat remote.
const EnvBackupHeartbeatRemote = "NSELF_BACKUP_HEARTBEAT_REMOTE"

// The variable is declared known so a project .env that sets it does not print
// an "unknown env var" warning on every run (cron logs stay quiet).
func init() {
	knownEnvVars = append(knownEnvVars, EnvBackupHeartbeatRemote)
}

// HeartbeatRemote returns the heartbeat remote from the environment, trimmed,
// or "" when it is not set.
func (BackupConfig) HeartbeatRemote() string {
	return strings.TrimSpace(os.Getenv(EnvBackupHeartbeatRemote))
}
