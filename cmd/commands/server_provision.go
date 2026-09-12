package commands

// Purpose: `nself server provision` handler — parses flags, builds a
// server.ProvisionRequest, and reports the created server back.
// Inputs: --name, --type, --location, --image, --ssh-key (repeatable),
// --label (repeatable key=value), --token, --token-env, --json.
// Outputs: printed confirmation (or JSON) with the new server's ID and IP;
// a non-nil error on any validation, resolution, or provider failure.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/server"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var serverProvisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Create a new Hetzner Cloud server",
	Long: `Create a new Hetzner Cloud server.

Every server nself provisions is labeled managed-by=nself-cli (unless you
pass your own --label managed-by=..., which is respected as-is), so
'nself server list' and any future cleanup pass can tell nself-created
servers apart from anything else in the same Hetzner project.`,
	Example: `  nself server provision --name ci-runner-3 --type cx22 --location fsn1 --image ubuntu-24.04 --ssh-key deploy
  nself server provision --name ci-runner-3 --type cx22 --location fsn1 --image ubuntu-24.04 --label purpose=ci --json`,
	RunE: runServerProvision,
}

func init() {
	f := serverProvisionCmd.Flags()
	f.String("name", "", "server name (required)")
	f.String("type", "", "server type, e.g. cx22 (required)")
	f.String("location", "", "datacenter location, e.g. fsn1 (required)")
	f.String("image", "", "OS image, e.g. ubuntu-24.04 (required)")
	f.StringArray("ssh-key", nil, "Hetzner SSH key name to authorize (repeatable)")
	f.StringArray("label", nil, "label as key=value (repeatable)")
	f.Bool("json", false, "output as JSON")
}

func runServerProvision(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	serverType, _ := cmd.Flags().GetString("type")
	location, _ := cmd.Flags().GetString("location")
	image, _ := cmd.Flags().GetString("image")
	sshKeys, _ := cmd.Flags().GetStringArray("ssh-key")
	labelArgs, _ := cmd.Flags().GetStringArray("label")
	jsonOut, _ := cmd.Flags().GetBool("json")

	labels, err := parseLabels(labelArgs)
	if err != nil {
		return err
	}

	client, err := newServerClient(cmd)
	if err != nil {
		return err
	}

	if !jsonOut {
		ui.CommandHeader("nself server provision", fmt.Sprintf("%s (%s, %s)", name, serverType, location))
	}

	srv, err := server.Provision(cmd.Context(), client, server.ProvisionRequest{
		Name: name, ServerType: serverType, Location: location, Image: image,
		SSHKeys: sshKeys, Labels: labels,
	})
	if err != nil {
		return fmt.Errorf("provision %s: %w", name, err)
	}

	if jsonOut {
		return ui.PrintJSON(srv)
	}

	ui.Success(fmt.Sprintf("Created server %q (id %d)", srv.Name, srv.ID))
	ui.Info("IPv4: " + srv.IPv4)
	ui.Info("Status: " + srv.Status)
	return nil
}

// parseLabels turns repeated "key=value" flag values into a map, rejecting
// anything that isn't exactly one "=".
func parseLabels(args []string) (map[string]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	labels := make(map[string]string, len(args))
	for _, a := range args {
		k, v, ok := strings.Cut(a, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("--label must be key=value, got %q", a)
		}
		labels[k] = v
	}
	return labels, nil
}
