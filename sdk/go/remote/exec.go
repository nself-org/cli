// Purpose: the single exec funnel. Every process this package starts is built
//          here from an argv slice; nothing else in the package calls
//          os/exec constructors.
// Inputs:  a tool name from a fixed set and its argv.
// Outputs: an *exec.Cmd with the allowlisted environment, or an error.
// Constraints: no shell. argv elements are never joined, split or re-parsed
//              here. The tool set is closed: ssh, scp, rsync, ssh-keyscan.

package remote

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// commandContext is the package test hook: tests replace it to capture argv.
var commandContext = exec.CommandContext

// allowedTools is the closed set Command will start.
var allowedTools = map[string]bool{"ssh": true, "scp": true, "rsync": true, "ssh-keyscan": true}

// Command builds (does not start) the process for tool with args. The
// environment is EnvAllowlist(); a caller that must inherit more (cli deploy)
// replaces cmd.Env explicitly before starting it.
func Command(ctx context.Context, tool string, args ...string) (*exec.Cmd, error) {
	if !allowedTools[tool] {
		return nil, fmt.Errorf("remote: %q is not an allowed tool", tool)
	}
	for _, a := range args {
		if strings.IndexByte(a, 0) >= 0 {
			return nil, fmt.Errorf("remote: argument contains a NUL byte")
		}
	}
	cmd := commandContext(ctx, tool, args...)
	cmd.Env = EnvAllowlist()
	return cmd, nil
}

// EnvAllowlist returns the only environment ssh, scp and rsync run with:
// PATH HOME USER LOGNAME SSH_AUTH_SOCK (when set) and LANG=C. A config
// SendEnv therefore has only those variables to send. SetEnv is different: it
// sets literal values from the ssh configuration itself, not from the process
// environment, so the allowlist does not neutralize it (OpenSSH also rejects
// an empty "-o SetEnv=" override, so there is no option to cancel it). What
// reaches the remote process is still limited by the server's AcceptEnv.
func EnvAllowlist() []string {
	env := make([]string, 0, 6)
	for _, k := range []string{"PATH", "HOME", "USER", "LOGNAME", "SSH_AUTH_SOCK"} {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			env = append(env, k+"="+v)
		}
	}
	return append(env, "LANG=C")
}
