package commands

// Purpose: resolve access selectors before any SSH or authorized_keys write.
// Inputs: explicit --host or inventory --env/--tier/--server selectors.
// Outputs: ordered, validated transports and a production confirmation gate.
// Constraints: explicit --host retains its existing transport and output path.

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/access"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/spf13/cobra"
)

type accessTarget struct {
	Env, Server, Host string
	Transport         access.Transport
	Prod              bool
}

var newAccessTargetTransport = func(host, identity, env, server string, tier controlplane.Tier, shipping bool) access.Transport {
	return &access.SSHTransport{Host: host, IdentityPath: identity, HostKeyOptions: func(ctx context.Context) ([]string, error) {
		return controlplane.HostKeyOptions(ctx, env, server, tier, host, shipping)
	}}
}

func resolveAccessTargets(cmd *cobra.Command) ([]accessTarget, bool, error) {
	host, _ := cmd.Flags().GetString("host")
	env, _ := cmd.Flags().GetString("env")
	tier, _ := cmd.Flags().GetString("tier")
	server, _ := cmd.Flags().GetString("server")
	selected := env != "" || tier != "" || server != ""
	if host != "" && selected {
		return nil, false, fmt.Errorf("--host is exclusive with --env, --tier and --server")
	}
	if !selected {
		t, err := newAccessTransport(cmd)
		if err != nil {
			return nil, false, err
		}
		return []accessTarget{{Host: host, Transport: t}}, false, nil
	}
	root, err := projectRoot()
	if err != nil {
		return nil, true, err
	}
	inv, err := controlplane.Load(root)
	if err != nil {
		return nil, true, fmt.Errorf("access: load inventory: %w", err)
	}
	targets, err := controlplane.ResolveTargets(inv, controlplane.Selector{Env: env, Tier: controlplane.Tier(tier), Server: server})
	if err != nil {
		return nil, true, err
	}
	identity, _ := cmd.Flags().GetString("identity")
	shipping := cmd.Name() != "list"
	out := make([]accessTarget, 0, len(targets))
	for _, target := range targets {
		if target.Server.Host == "" {
			return nil, true, errs.New("E486", fmt.Sprintf("%s/%s has no SSH host", target.Env, target.Server.Name))
		}
		if _, err := remote.ParseHostSpec(target.Server.Host); err != nil {
			return nil, true, errs.New("E484", fmt.Sprintf("%s/%s: %v", target.Env, target.Server.Name, err))
		}
		hostIdentity := identity
		if hostIdentity == "" && target.Server.SSHKeyRef != "" {
			hostIdentity = os.Getenv(target.Server.SSHKeyRef)
		}
		if hostIdentity == "" {
			hostIdentity = defaultIdentityPath()
		}
		out = append(out, accessTarget{
			Env: target.Env, Server: target.Server.Name, Host: target.Server.Host,
			Transport: newAccessTargetTransport(target.Server.Host, hostIdentity, target.Env, target.Server.Name, target.Tier, shipping),
			Prod:      controlplane.IsProdClass(inv, target.Env),
		})
	}
	return out, true, nil
}

func confirmAccessTargets(cmd *cobra.Command, targets []accessTarget, dryRun bool) error {
	// compat.V15(P7-DEPL-14): unrestricted inventory writes -> prod-class confirmation.
	if dryRun || !compat.V15() {
		return nil
	}
	var prod []string
	for _, target := range targets {
		if target.Prod {
			prod = append(prod, target.Env+"/"+target.Server)
		}
	}
	if len(prod) == 0 {
		return nil
	}
	yes, _ := cmd.Flags().GetBool("yes")
	if yes {
		return nil
	}
	confirm := interactiveConfirm(cmd)
	if confirm != nil && confirm("Change SSH access on production hosts "+strings.Join(prod, ", ")+"?") {
		return nil
	}
	return errs.New("E403", "production SSH access change requires --yes or terminal confirmation")
}

func printAccessFailure(cmd *cobra.Command, target accessTarget, err error) {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s/%s host=%s status=failed reason=%q\n", target.Env, target.Server, target.Host, err.Error())
}
