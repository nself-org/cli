// Purpose: resolve an ssh alias to the facts the CLI needs, using ssh itself.
// Inputs:  a host alias or hostname.
// Outputs: Resolved{Hostname, Port, User, ProxyJump, HostKeyAlias}.
// Constraints: runs `ssh -G -- <alias>` (no connection is made) with a 5 s
//              timeout; only five keys are read, the rest is ignored; an alias
//              failing ValidateAlias is refused before any exec. Interim check
//              until P7-DEPL-23 switches this to ParseHostSpec.

package remote

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Resolved is what `ssh -G` reports for an alias.
type Resolved struct {
	Hostname     string
	Port         int
	User         string
	ProxyJump    string
	HostKeyAlias string
}

// ResolveSSHHost runs `ssh -G -- <alias>` and parses the effective hostname,
// port, user, proxyjump and hostkeyalias.
func ResolveSSHHost(ctx context.Context, alias string) (Resolved, error) {
	if err := ValidateAlias(alias); err != nil {
		return Resolved{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd, err := Command(ctx, "ssh", "-G", "--", alias)
	if err != nil {
		return Resolved{}, err
	}
	out, err := cmd.Output()
	if err != nil {
		return Resolved{}, fmt.Errorf("ssh -G %s: %w", alias, err)
	}
	return parseSSHG(string(out))
}

func parseSSHG(out string) (Resolved, error) {
	var r Resolved
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.ToLower(key) {
		case "hostname":
			r.Hostname = val
		case "port":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 || n > 65535 {
				return Resolved{}, fmt.Errorf("ssh -G reported invalid port %q", val)
			}
			r.Port = n
		case "user":
			r.User = val
		case "proxyjump":
			r.ProxyJump = val
		case "hostkeyalias":
			r.HostKeyAlias = val
		}
	}
	if r.Hostname == "" {
		return Resolved{}, fmt.Errorf("ssh -G reported no hostname")
	}
	return r, nil
}
