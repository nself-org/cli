package commands

// Purpose: `nself server list` handler — renders server.List's result as a
// table (default) or JSON (--json).
// Inputs: --label-selector, --token, --token-env, --json.
// Outputs: a table (or JSON array) of servers in the Hetzner project.

import (
	"fmt"

	"github.com/nself-org/cli/internal/server"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var serverListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Hetzner Cloud servers",
	Example: `  nself server list
  nself server list --label-selector managed-by=nself-cli
  nself server list --json`,
	RunE: runServerList,
}

func init() {
	serverListCmd.Flags().String("label-selector", "", "filter by Hetzner label selector, e.g. managed-by=nself-cli")
	serverListCmd.Flags().Bool("json", false, "output as JSON")
}

func runServerList(cmd *cobra.Command, args []string) error {
	labelSelector, _ := cmd.Flags().GetString("label-selector")
	jsonOut, _ := cmd.Flags().GetBool("json")

	client, err := newServerClient(cmd)
	if err != nil {
		return err
	}

	servers, err := server.List(cmd.Context(), client, server.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return fmt.Errorf("list servers: %w", err)
	}

	if jsonOut {
		return ui.PrintJSON(servers)
	}

	if len(servers) == 0 {
		fmt.Println("No servers found.")
		return nil
	}

	table := ui.NewTable("ID", "NAME", "STATUS", "TYPE", "LOCATION", "IPV4")
	for _, s := range servers {
		table.AddRow(fmt.Sprintf("%d", s.ID), s.Name, s.Status, s.ServerType, s.Location, s.IPv4)
	}
	table.Render()
	return nil
}
