// Purpose: shared validation for remote-path values that get interpolated
// into shell command strings executed on remote hosts over SSH (docker
// compose invocations built with fmt.Sprintf and handed to `ssh` as a single
// argv element, which the remote shell then parses).
// Inputs: a candidate remote-path string, from any input source: a CLI flag
// (`nself deploy --remote-path`, `nself env target add --remote-path`), a
// parsed .nself/control-plane.yaml inventory file, or an
// NSELF_REMOTE_PATH_<TARGET> environment variable.
// Outputs: nil when the path is empty or matches the allowed charset;
// otherwise a descriptive error naming the offending value.
// Constraints: this is the single source of truth for the remote-path
// charset enforced across cmd/commands (deploy, deploy_remote, env target
// add) and internal/controlplane (inventory Load/synthesize). Centralizing
// it here (rather than in cmd/commands, which internal/controlplane already
// imports transitively via internal/deploy) avoids an import cycle while
// keeping every enforcement point byte-identical. Deliberately an allowlist
// (reject anything outside a known-safe charset), not a blacklist of known-
// bad shell metacharacters, per the standing guidance that allowlists are
// harder to bypass via an overlooked special character.
package deploy

import "github.com/nself-org/cli/sdk/go/v2/remote"

// RemotePathRe is the shared remote-path allowlist. It lives in
// sdk/go/remote (the one implementation); this alias keeps every existing
// caller and cmd/commands' own alias pointing at the same compiled pattern.
var RemotePathRe = remote.RemotePathRe

// ValidateRemotePath returns an error when path is non-empty and contains
// characters outside RemotePathRe's allowed charset. An empty path is
// treated as valid here — callers that require a non-empty path (e.g. a
// remote deploy target) must check that separately; ssh.go's DeployViaSsh
// already falls back to /tmp when the recovered remote path is empty.
//
// One refusal is added to the charset check: a leading "-", which a remote
// rsync --server or scp would read as an option. ".." segments stay accepted,
// as they always were. The check itself lives in sdk/go/remote.
func ValidateRemotePath(path string) error {
	return remote.ValidateRemotePath(path)
}
