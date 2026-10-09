package commands

// Purpose: `nself env target list/add/remove` RunE handlers split out of
// env_target.go (CLI-R12 Batch B mechanical file-size split). Reads/writes
// .nself/control-plane.yaml server entries.
// Inputs: cobra command flags/args as documented on the cobra.Command vars
// in env_target.go (--json, --all, --host, --role, --key-ref, --remote-path,
// --primary, --upstreams).
// Outputs: a table or JSON listing, or a confirmation message after
// add/remove; errors wrap controlplane package failures.
// Constraints: pure move, no behavior change. Security invariants (no
// inline secrets, 0600 writes) are documented in env_target.go's file
// header and enforced by the controlplane package, unchanged here.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/spf13/cobra"
)

// envTargetListRow is the JSON representation of one row in the list output.
type envTargetListRow struct {
	Env        string `json:"env"`
	Server     string `json:"server"`
	Role       string `json:"role"`
	Host       string `json:"host"`
	SSHKeyRef  string `json:"ssh_key_ref"`
	RemotePath string `json:"remote_path"`
	Primary    bool   `json:"primary"`
}

func runEnvTargetList(cmd *cobra.Command, args []string) error {
	root, err := projectRoot()
	if err != nil {
		return err
	}

	inv, err := controlplane.Load(root)
	if err != nil {
		return fmt.Errorf("env target list: %w", err)
	}

	jsonOut, _ := cmd.Flags().GetBool("json")
	showAll, _ := cmd.Flags().GetBool("all")

	filterEnvName := ""
	if len(args) == 1 {
		filterEnvName = args[0]
	}

	var rows []envTargetListRow
	envNames := sortedEnvNames(inv)
	for _, envName := range envNames {
		if filterEnvName != "" && envName != filterEnvName {
			continue
		}
		env := inv.Environments[envName]
		for _, srv := range env.Servers {
			if !showAll && srv.Host == "" && env.Kind != "local" {
				continue // hidden — omit by default
			}
			rows = append(rows, envTargetListRow{
				Env:        envName,
				Server:     srv.Name,
				Role:       string(srv.Role),
				Host:       srv.Host,
				SSHKeyRef:  srv.SSHKeyRef,
				RemotePath: srv.RemotePath,
				Primary:    srv.Primary,
			})
		}
	}

	if jsonOut {
		b, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	if len(rows) == 0 {
		fmt.Println("No servers found. Run 'nself env target add' to configure one.")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ENV\tSERVER\tROLE\tHOST\tKEY-REF\tPRIMARY")
	for _, r := range rows {
		primary := ""
		if r.Primary {
			primary = "yes"
		}
		host := r.Host
		if host == "" {
			host = "(local)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Env, r.Server, r.Role, host, r.SSHKeyRef, primary)
	}
	return tw.Flush()
}

// validRoles is the set of accepted server role strings.
var validRoles = map[string]controlplane.ServerRole{
	"app":           controlplane.RoleApp,
	"lb":            controlplane.RoleLB,
	"observability": controlplane.RoleObservability,
	"db":            controlplane.RoleDB,
	"worker":        controlplane.RoleWorker,
}

func runEnvTargetAdd(cmd *cobra.Command, args []string) error {
	envName := args[0]
	serverName := args[1]

	host, _ := cmd.Flags().GetString("host")
	roleStr, _ := cmd.Flags().GetString("role")
	keyRef, _ := cmd.Flags().GetString("key-ref")
	remotePath, _ := cmd.Flags().GetString("remote-path")
	primary, _ := cmd.Flags().GetBool("primary")
	upstreams, _ := cmd.Flags().GetStringSlice("upstreams")
	tierStr, _ := cmd.Flags().GetString("tier")
	arch, _ := cmd.Flags().GetString("arch")
	trust, _ := cmd.Flags().GetString("trust-host-key")
	if host != "" {
		if _, err := remote.ParseHostSpec(host); err != nil {
			return errs.New("E484", fmt.Sprintf("--host: %v", err))
		}
	}
	if arch != "" && arch != "amd64" && arch != "arm64" {
		return errs.New("E485", fmt.Sprintf("--arch: invalid %q", arch))
	}
	if tierStr != "" && tierStr != "local" && tierStr != "local-servers" && tierStr != "prod" {
		return errs.New("E485", fmt.Sprintf("--tier: invalid %q", tierStr))
	}

	// Security: reject inline key material in --host or --key-ref.
	if err := rejectInlineSecret("--host", host); err != nil {
		return err
	}
	if err := rejectInlineSecret("--key-ref", keyRef); err != nil {
		return err
	}
	// Security (T31): remotePath is persisted into .nself/control-plane.yaml
	// and later interpolated into shell command strings run on the remote
	// host (see internal/deploy/ssh.go's DeployViaSsh). Reject unsafe
	// characters here, at the same point --host/--key-ref are already
	// checked, using the same charset as `nself deploy --remote-path`.
	if err := deploy.ValidateRemotePath(remotePath); err != nil {
		return fmt.Errorf("--remote-path: %w", err)
	}

	role, ok := validRoles[strings.ToLower(roleStr)]
	if !ok {
		return fmt.Errorf("invalid --role %q (allowed: app, lb, observability, db, worker)", roleStr)
	}

	root, err := projectRoot()
	if err != nil {
		return err
	}
	inv, err := controlplane.Load(root)
	if err != nil {
		return fmt.Errorf("env target add: %w", err)
	}

	// Resolve or create the environment.
	env, exists := inv.Environments[envName]
	if !exists {
		kind := "remote"
		if host == "" {
			kind = "local"
		}
		env = controlplane.Environment{Name: envName, Kind: kind, Tier: controlplane.DeriveTier(envName, kind), Servers: nil}
	}
	if tierStr != "" {
		env.Tier = controlplane.Tier(tierStr)
	}

	// Reject duplicate server name within the environment.
	for _, s := range env.Servers {
		if s.Name == serverName {
			if trust != "" {
				if host != "" && host != s.Host {
					return errs.New("E487", "--host differs from the existing server host")
				}
				if err := controlplane.TrustHostKey(cmd.Context(), s.Host, trust); err != nil {
					return err
				}
				fmt.Printf("Trusted host key for server %q in environment %q.\n", serverName, envName)
				return nil
			}
			return fmt.Errorf("server %q already exists in environment %q; use 'env target remove' first", serverName, envName)
		}
	}

	// Enforce: remote servers must have a host.
	if env.Kind == "remote" && host == "" {
		return fmt.Errorf("--host is required for remote environment %q", envName)
	}
	srv := controlplane.Server{
		Name:       serverName,
		Role:       role,
		Host:       host,
		SSHKeyRef:  keyRef,
		RemotePath: remotePath,
		Primary:    primary,
		Arch:       arch,
		Upstreams:  upstreams,
	}
	env.Servers = append(env.Servers, srv)

	if inv.Environments == nil {
		inv.Environments = make(map[string]controlplane.Environment)
	}
	inv.Environments[envName] = env
	if err := controlplane.ValidateInventory(inv); err != nil {
		return err
	}
	if trust != "" {
		if err := controlplane.TrustHostKey(cmd.Context(), host, trust); err != nil {
			return err
		}
	}

	if err := controlplane.Write(root, inv); err != nil {
		return fmt.Errorf("env target add: %w", err)
	}

	fmt.Printf("Added server %q to environment %q. File: .nself/control-plane.yaml (0600).\n", serverName, envName)
	return nil
}

func runEnvTargetRemove(_ *cobra.Command, args []string) error {
	envName := args[0]
	serverName := args[1]

	root, err := projectRoot()
	if err != nil {
		return err
	}
	inv, err := controlplane.Load(root)
	if err != nil {
		return fmt.Errorf("env target remove: %w", err)
	}

	env, exists := inv.Environments[envName]
	if !exists {
		return fmt.Errorf("environment %q not found in inventory", envName)
	}

	var kept []controlplane.Server
	found := false
	for _, s := range env.Servers {
		if s.Name == serverName {
			found = true
			continue
		}
		kept = append(kept, s)
	}
	if !found {
		return fmt.Errorf("server %q not found in environment %q", serverName, envName)
	}
	env.Servers = kept
	inv.Environments[envName] = env
	if err := controlplane.ValidateInventory(inv); err != nil {
		return err
	}

	if err := controlplane.Write(root, inv); err != nil {
		return fmt.Errorf("env target remove: %w", err)
	}

	fmt.Printf("Removed server %q from environment %q.\n", serverName, envName)
	return nil
}
