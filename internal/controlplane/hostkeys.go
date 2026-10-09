package controlplane

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	env, server = hostKeyIdentity(env, server, host)
	prodClass := IsProdClass(&Inventory{Environments: map[string]Environment{env: {Name: env, Tier: tier}}}, env)
	// compat.V15(P7-DEPL-13): quiet read-only first contact -> print the observed fingerprint once.
	if compat.V15() && !shipping && !prodClass {
		printFirstContact(ctx, host)
	}
	return deploy.HostKeyOptions(ctx, host, env, server, shipping || prodClass)
}

// hostKeyIdentity maps callers with generic labels to the inventory entry so
// the E487 command can be pasted into `env target add` unchanged.
func hostKeyIdentity(env, server, host string) (string, string) {
	root, err := inventoryProjectRoot()
	if err != nil {
		return env, server
	}
	inv, err := Load(root)
	if err != nil {
		return env, server
	}
	for name, item := range inv.Environments {
		for _, candidate := range item.Servers {
			if candidate.Host == host && candidate.Name == server && name == env {
				return env, server
			}
			if candidate.Host == host && name == env && candidate.Primary && server == "primary" {
				return name, candidate.Name
			}
			if candidate.Host == host && candidate.Name == server {
				return name, candidate.Name
			}
		}
	}
	var matchEnv, matchServer string
	for name, item := range inv.Environments {
		for _, candidate := range item.Servers {
			if candidate.Host != host {
				continue
			}
			if matchServer != "" {
				return env, server
			}
			matchEnv, matchServer = name, candidate.Name
		}
	}
	if matchServer != "" {
		return matchEnv, matchServer
	}
	return env, server
}

// inventoryProjectRoot finds the inventory used by the active project.
func inventoryProjectRoot() (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(root, inventoryFileName)); err == nil {
			return root, nil
		}
		parent := filepath.Dir(root)
		if parent == root {
			return "", os.ErrNotExist
		}
		root = parent
	}
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
	return TierProd, ""
}
