package controlplane

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

var firstContact sync.Map

// DeployKnownHostsPath is the dedicated pin file, separate from ~/.ssh/known_hosts.
func DeployKnownHostsPath() string { return deploy.DeployKnownHostsPath() }

// TrustHostKey rescans a host and pins only the offered live fingerprint.
func TrustHostKey(ctx context.Context, host, offered string) error {
	return deploy.TrustHostKey(ctx, host, offered)
}

// HostKeyOptions selects the strict policy for secret shipping and prod hosts.
func HostKeyOptions(ctx context.Context, env, server string, tier Tier, host string, shipping bool) ([]string, error) {
	// compat.V15(P7-DEPL-13): quiet read-only first contact -> print the observed fingerprint once.
	if compat.V15() && !shipping && tier != TierProd {
		printFirstContact(ctx, host)
	}
	return deploy.HostKeyOptions(ctx, host, env, server, shipping || tier == TierProd)
}

func printFirstContact(ctx context.Context, host string) {
	spec, err := remote.ParseHostSpec(host)
	if err != nil {
		return
	}
	alias := spec.Host
	if spec.Port != 0 {
		alias = "[" + spec.Host + "]:" + strconv.Itoa(spec.Port)
	}
	if _, err := exec.CommandContext(ctx, "ssh-keygen", "-F", alias).Output(); err == nil {
		return
	}
	if _, seen := firstContact.LoadOrStore(alias, true); seen {
		return
	}
	if keys, err := deploy.ScanDeployKeys(ctx, spec); err == nil && len(keys) > 0 {
		fmt.Fprintf(os.Stderr, "first contact %s: host key %s\n", strings.TrimSpace(alias), keys[0].Fingerprint)
	}
}

func probeTier(projectRoot string, s Server) (Tier, string) {
	inv, err := Load(projectRoot)
	if err == nil {
		for name, env := range inv.Environments {
			for _, candidate := range env.Servers {
				if candidate.Name == s.Name && candidate.Host == s.Host {
					return normalizedTier(env), name
				}
			}
		}
	}
	return TierLocalServers, ""
}
