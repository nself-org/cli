package commands

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/spf13/cobra"
)

type deployTargetRow struct {
	Env                 string                  `json:"env"`
	Tier                controlplane.Tier       `json:"tier"`
	Server              string                  `json:"server"`
	Role                controlplane.ServerRole `json:"role"`
	Primary             bool                    `json:"primary"`
	User                *string                 `json:"user"`
	Host                string                  `json:"host"`
	Port                int                     `json:"port"`
	SSHHostname         string                  `json:"ssh_hostname"`
	Addresses           []string                `json:"addresses"`
	HostKeyFingerprints []string                `json:"host_key_fingerprints"`
	Arch                *string                 `json:"arch"`
	Source              string                  `json:"source"`
}

type deployTargetsData struct {
	Targets  []deployTargetRow `json:"targets"`
	Warnings []string          `json:"warnings"`
}

func init() {
	c := &cobra.Command{Use: "targets", Short: "List deploy targets without connecting to them", Args: cobra.NoArgs, RunE: runDeployTargets}
	c.Flags().Bool("json", false, "Emit the deploy targets document")
	deployCmd.AddCommand(c)
}

func runDeployTargets(cmd *cobra.Command, _ []string) error {
	root, err := projectRoot()
	if err != nil {
		return err
	}
	inv, err := controlplane.Load(root)
	if err != nil {
		return err
	}
	_, statErr := os.Stat(root + "/.nself/control-plane.yaml")
	source := "inventory"
	if os.IsNotExist(statErr) {
		source = "env"
	}
	data, err := collectDeployTargets(cmd.Context(), inv, source)
	if err != nil {
		return err
	}
	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		return output.EmitData(output.Default(), "deploy targets", data)
	}
	for _, row := range data.Targets {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", row.Env, row.Server, row.Tier, row.Host); err != nil {
			return err
		}
	}
	return nil
}

// collectDeployTargets runs ssh -G, DNS and ssh-keygen -F only; it never opens SSH.
func collectDeployTargets(ctx context.Context, inv *controlplane.Inventory, source string) (deployTargetsData, error) {
	data := deployTargetsData{Targets: []deployTargetRow{}, Warnings: []string{}}
	for envName, env := range inv.Environments {
		tier := env.Tier
		if tier == "" {
			tier = controlplane.DeriveTier(envName, env.Kind)
		}
		for _, srv := range env.Servers {
			row := deployTargetRow{Env: envName, Tier: tier, Server: srv.Name, Role: srv.Role, Primary: srv.Primary, Host: srv.Host, Port: 22, Addresses: []string{}, HostKeyFingerprints: []string{}, Source: source}
			if srv.Arch != "" {
				row.Arch = &srv.Arch
			}
			if srv.Host != "" {
				spec, err := remote.ParseHostSpec(srv.Host)
				if err != nil {
					return data, err
				}
				row.Host = spec.Host
				if spec.Port != 0 {
					row.Port = spec.Port
				}
				if spec.User != "" {
					row.User = &spec.User
				}
				resolved, err := remote.ResolveSSHHost(ctx, spec.String())
				if err != nil {
					return data, err
				}
				row.SSHHostname = resolved.Hostname
				if resolved.Port != 0 {
					row.Port = resolved.Port
				}
				if resolved.User != "" {
					row.User = &resolved.User
				}
				lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				ips, err := net.DefaultResolver.LookupHost(lookupCtx, row.SSHHostname)
				cancel()
				if err != nil {
					data.Warnings = append(data.Warnings, fmt.Sprintf("%s/%s: DNS: %v", envName, srv.Name, err))
				} else {
					row.Addresses = ips
					sort.Strings(row.Addresses)
				}
				row.HostKeyFingerprints = knownHostFingerprints(ctx, spec.Host, row.Port)
			}
			data.Targets = append(data.Targets, row)
		}
	}
	sort.Slice(data.Targets, func(i, j int) bool {
		a, b := data.Targets[i], data.Targets[j]
		if a.Env != b.Env {
			return a.Env < b.Env
		}
		return a.Server < b.Server
	})
	return data, nil
}

func knownHostFingerprints(ctx context.Context, host string, port int) []string {
	query := host
	if port != 22 {
		query = fmt.Sprintf("[%s]:%d", host, port)
	}
	cmd := exec.CommandContext(ctx, "ssh-keygen", "-F", query)
	out, err := cmd.Output()
	if err != nil {
		return []string{}
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || strings.HasPrefix(line, "#") {
			continue
		}
		blob, err := base64.StdEncoding.DecodeString(f[2])
		if err != nil {
			continue
		}
		sum := sha256.Sum256(blob)
		seen["SHA256:"+base64.RawStdEncoding.EncodeToString(sum[:])] = true
	}
	keys := make([]string, 0, len(seen))
	for fp := range seen {
		keys = append(keys, fp)
	}
	sort.Strings(keys)
	return keys
}
