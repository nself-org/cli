package controlplane

// Purpose: read and cache a remote host's CPU architecture through SSH.
// Inputs: a server and context; outputs: amd64 or arm64, or an error.
// Constraints: the command is read-only and follows the existing host-key policy.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

type archEntry struct {
	arch string
	at   time.Time
}

var archCaches sync.Map // project root and host -> archEntry

// Architecture returns the inventory architecture or one cached SSH uname -m.
func (sp *SSHProber) Architecture(ctx context.Context, s Server) (string, error) {
	if s.Arch != "" {
		return s.Arch, nil
	}
	cacheKey := sp.projectRoot + "\x00" + s.Host
	lock := sp.hostMutex(s.Host)
	lock.Lock()
	defer lock.Unlock()
	if !sp.refresh {
		if entryAny, ok := archCaches.Load(cacheKey); ok {
			entry := entryAny.(archEntry)
			if time.Since(entry.at) < probeCacheTTL {
				return entry.arch, nil
			}
		}
	}
	spec, err := remote.ParseHostSpec(s.Host)
	if err != nil {
		return "", err
	}
	key := os.Getenv(s.SSHKeyRef)
	if key == "" {
		key = os.Getenv("NSELF_DEPLOY_SSH_KEY")
	}
	if key == "" {
		home, _ := os.UserHomeDir()
		key = home + "/.ssh/id_ed25519"
	}
	tier, envName := probeTier(sp.projectRoot, s)
	policy, err := HostKeyOptions(ctx, envName, s.Name, tier, s.Host, false)
	if err != nil {
		return "", err
	}
	args := []string{"-i", key, "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "-o", "ForwardAgent=no"}
	args = append(args, policy...)
	args = append(args, spec.SSHArgs()...)
	args = append(args, "uname -m")
	out, err := exec.CommandContext(ctx, "ssh", args...).Output() //nolint:gosec // validated HostSpec and fixed remote command
	if err != nil {
		return "", fmt.Errorf("uname -m on %s: %w", s.Name, err)
	}
	var arch string
	switch strings.TrimSpace(string(out)) {
	case "x86_64", "amd64":
		arch = "amd64"
	case "aarch64", "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("uname -m on %s returned unsupported architecture %q", s.Name, strings.TrimSpace(string(out)))
	}
	archCaches.Store(cacheKey, archEntry{arch, time.Now()})
	return arch, nil
}
