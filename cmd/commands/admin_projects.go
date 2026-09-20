package commands

// Purpose: the "nself admin projects" subtree (list/add/remove). Inputs are
// the cobra command/args; outputs are printed/updated saved admin projects
// or an error.
// Constraints: split out of admin_connect.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"

	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var adminProjectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "Manage multi-project admin configuration",
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself admin projects` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var adminProjectsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured admin projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")

		projects, err := loadAdminProjects()
		if err != nil {
			return fmt.Errorf("load projects: %w", err)
		}

		if jsonOut {
			data, _ := json.MarshalIndent(projects, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		if len(projects) == 0 {
			ui.Bullet("No projects configured. Use 'nself admin projects add' to add one.")
			return nil
		}

		ui.CommandHeader("Admin Projects", fmt.Sprintf("%d projects", len(projects)))
		for _, p := range projects {
			fmt.Printf("  %-20s  %-20s  %-30s  %s\n", p.ID, p.Host, p.Name, p.URL)
		}
		return nil
	},
}

var adminProjectsAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a project to the admin multi-project config",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		host, _ := cmd.Flags().GetString("host")
		sshUser, _ := cmd.Flags().GetString("ssh-user")
		url, _ := cmd.Flags().GetString("url")

		if name == "" || host == "" {
			return fmt.Errorf("--name and --host are required")
		}

		project := AdminProject{
			ID:      name,
			Name:    name,
			Host:    host,
			SSHUser: sshUser,
			URL:     url,
		}

		projects, _ := loadAdminProjects()
		for _, p := range projects {
			if p.ID == project.ID {
				return fmt.Errorf("project %q already exists", project.ID)
			}
		}

		projects = append(projects, project)
		if err := saveAdminProjects(projects); err != nil {
			return err
		}

		ui.Success(fmt.Sprintf("Added project %q (host: %s)", project.Name, project.Host))
		return nil
	},
}

var adminProjectsRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove a project from admin config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projects, err := loadAdminProjects()
		if err != nil {
			return err
		}

		var filtered []AdminProject
		found := false
		for _, p := range projects {
			if p.ID == args[0] {
				found = true
				continue
			}
			filtered = append(filtered, p)
		}

		if !found {
			return fmt.Errorf("project %q not found", args[0])
		}

		if err := saveAdminProjects(filtered); err != nil {
			return err
		}

		ui.Success(fmt.Sprintf("Removed project %q", args[0]))
		return nil
	},
}
