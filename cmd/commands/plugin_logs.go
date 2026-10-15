package commands

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/docker"
	"github.com/spf13/cobra"
)

var pluginLogsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "Tail logs from a plugin container",
	Long: `Stream or print logs from a named plugin container.

Resolves the container name from the plugin name using the nSelf naming
convention (nself-plugin-<name>) or docker compose ps lookup.

Flags mirror docker logs flags for familiarity:

  nself plugin logs myplugin --follow       # live stream
  nself plugin logs myplugin --tail 50      # last 50 lines
  nself plugin logs myplugin --since 1h     # last 1 hour
  nself plugin logs myplugin --grep ERROR   # regex filter`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginLogs,
}

func init() {
	pluginLogsCmd.Flags().BoolP("follow", "f", false, "Follow log output (like tail -f)")
	pluginLogsCmd.Flags().Int("tail", 0, "Number of lines to show from the end (0 = all)")
	pluginLogsCmd.Flags().String("since", "", "Show logs since duration (e.g. 1h, 30m, 5s)")
	pluginLogsCmd.Flags().String("grep", "", "Regex pattern to filter log lines")
	pluginCmd.AddCommand(pluginLogsCmd)
}

func runPluginLogs(cmd *cobra.Command, args []string) error {
	name := args[0]
	follow, _ := cmd.Flags().GetBool("follow")
	tail, _ := cmd.Flags().GetInt("tail")
	since, _ := cmd.Flags().GetString("since")
	grepPattern, _ := cmd.Flags().GetString("grep")

	// Resolve the container name.
	containerName, err := resolvePluginContainer(name)
	if err != nil {
		return err
	}

	// Build docker logs args.
	dockerArgs := []string{"logs", containerName}
	if follow {
		dockerArgs = append(dockerArgs, "--follow")
	}
	if tail > 0 {
		dockerArgs = append(dockerArgs, "--tail", fmt.Sprintf("%d", tail))
	} else if tail == 0 && !follow {
		// Default to last 100 lines when not following.
		dockerArgs = append(dockerArgs, "--tail", "100")
	}
	if since != "" {
		dockerArgs = append(dockerArgs, "--since", since)
	}

	if grepPattern == "" {
		if err := docker.NewCompose().StreamLogs(cmd.Context(), "", dockerArgs, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("docker logs: %w", err)
		}
		return nil
	}

	// Validate the grep pattern before use so we get a helpful error on invalid regex.
	pattern, err := regexp.Compile(grepPattern)
	if err != nil {
		return fmt.Errorf("invalid --grep pattern %q: %w", grepPattern, err)
	}

	return docker.NewCompose().StreamLogLines(cmd.Context(), "", dockerArgs, cmd.ErrOrStderr(), func(line string) error {
		if pattern.MatchString(line) {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), line)
			return err
		}
		return nil
	})
}

// resolvePluginContainer resolves the Docker container name for a plugin.
// Tries nself-plugin-<name> first, then falls back to docker compose ps lookup.
func resolvePluginContainer(pluginName string) (string, error) {
	// Standard nSelf container naming convention.
	candidates := []string{
		"nself-plugin-" + pluginName,
		"nself_plugin_" + pluginName,
		pluginName,
	}

	for _, c := range candidates {
		inspectCmd := exec.Command("docker", "inspect", "--format", "{{.Name}}", c)
		out, err := inspectCmd.Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return c, nil
		}
	}

	// Last resort: docker compose ps lookup.
	return resolveViaCompose(pluginName)
}

// resolveViaCompose attempts to find the container via docker compose ps.
func resolveViaCompose(pluginName string) (string, error) {
	psCmd := exec.Command("docker", "compose", "ps", "--format", "json")
	out, err := psCmd.Output()
	if err != nil {
		return "", fmt.Errorf("plugin container for %q not found — is the plugin installed and running?", pluginName)
	}

	// Scan output lines for a service name matching the plugin.
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(strings.ToLower(line), strings.ToLower(pluginName)) {
			// Extract container name from JSON-ish output.
			if idx := strings.Index(line, `"Name":"`); idx >= 0 {
				rest := line[idx+8:]
				if end := strings.Index(rest, `"`); end >= 0 {
					return rest[:end], nil
				}
			}
		}
	}

	return "", fmt.Errorf("plugin container for %q not found — is the plugin running? Check with `docker compose ps`", pluginName)
}

// For production debugging, use Grafana/Loki with the nSelf monitoring stack.
// Equivalent Loki query for plugin logs:
//   {container=~"nself-plugin-<name>"} |= "ERROR"
// See nself.org/docs/observability for the Grafana dashboard.
