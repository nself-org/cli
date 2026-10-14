package deploy

// Purpose: the accept-new host-key policy and, in v1.5, nself's own TOFU file.
// Inputs:  none (the home directory decides the two file paths).
// Outputs: "-o ..." option tokens for a connection that carries no secret.
// Constraints: nself code never writes ~/.ssh/known_hosts. In v1.5 an
//              accept-new connection reads the operator file as the global
//              file (a changed key there still fails) and writes first-seen
//              keys to the TOFU file; the strict check never reads the TOFU
//              file. Each option is one token and passes checkTransportToken.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/nself-org/cli/internal/compat"
)

// tofuWarn is where the one-line home-path warning goes.
var tofuWarn io.Writer = os.Stderr

var tofuWarnOnce sync.Once

// DeployTofuHostsPath is nself's own first-seen key file (v1.5 accept-new).
func DeployTofuHostsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "nself", "deploy_tofu_hosts")
}

// acceptNewPolicy is the host-key policy of a connection that ships no secret.
func acceptNewPolicy() []string {
	plain := []string{"-o", "StrictHostKeyChecking=accept-new"}
	// compat.V15(P7-DEPL-29): accept-new seeds ~/.ssh/known_hosts -> accept-new seeds nself's TOFU file, operator file read-only
	if !compat.V15() {
		return plain
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return plain
	}
	tofu := filepath.ToSlash(DeployTofuHostsPath())
	operator := filepath.ToSlash(filepath.Join(home, ".ssh", "known_hosts"))
	opts := []string{"UserKnownHostsFile=" + tofu, "GlobalKnownHostsFile=" + operator}
	for _, o := range opts {
		if err := checkTransportToken(o); err != nil {
			tofuWarnOnce.Do(func() {
				_, _ = fmt.Fprintf(tofuWarn, "warning: home path %q cannot go into an ssh option; host keys of this connection are not kept in nself's own file\n", home)
			})
			return plain
		}
	}
	if err := ensureTofuFile(DeployTofuHostsPath()); err != nil {
		tofuWarnOnce.Do(func() {
			_, _ = fmt.Fprintf(tofuWarn, "warning: cannot create %s: %v; first-seen host keys of this connection are not recorded\n", DeployTofuHostsPath(), err)
		})
		// Never fall back to ssh's default file (the operator's ~/.ssh/known_hosts): record nothing.
		return append(plain, "-o", "UserKnownHostsFile="+filepath.ToSlash(os.DevNull), "-o", opts[1])
	}
	return append(plain, "-o", opts[0], "-o", opts[1])
}

// ensureTofuFile creates the TOFU directory (0700) and file (0600) when
// missing, because ssh does not create parents outside ~/.ssh. An existing
// file is left untouched.
func ensureTofuFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}
