package commands

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var pluginDebugCmd = &cobra.Command{
	Use:   "debug <name>",
	Short: "Attach a dlv debugger to a running plugin process",
	Long: `Start the named plugin under the Delve debugger in headless mode.

Auto-allocates a port from the 2345-2399 range (handles multiple plugins in
simultaneous debug sessions). Prints the port and a VS Code launch.json snippet
you can paste directly.

  nself plugin debug myplugin
  nself plugin debug myplugin --port 2350
  nself plugin debug myplugin --port-only      # prints port only, for scripting

NOTE: dlv must be installed. Install with: go install github.com/go-delve/delve/cmd/dlv@latest`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginDebug,
}

func init() {
	pluginDebugCmd.Flags().Int("port", 0, "Debugger listen port (0 = auto-allocate from 2345-2399)")
	pluginDebugCmd.Flags().Bool("port-only", false, "Print the allocated port and exit (for scripting)")
	pluginCmd.AddCommand(pluginDebugCmd)
}

func runPluginDebug(cmd *cobra.Command, args []string) error {
	name := args[0]
	// The name picks the plugin directory; a path-shaped name must never reach it (P7-PLUG-20).
	if err := plugin.ValidateName(name); err != nil {
		return err
	}
	manualPort, _ := cmd.Flags().GetInt("port")
	portOnly, _ := cmd.Flags().GetBool("port-only")

	// Resolve or allocate port.
	port, err := resolveDebugPort(manualPort)
	if err != nil {
		return err
	}

	if portOnly {
		fmt.Printf(":%d\n", port)
		return nil
	}

	// Verify dlv is installed.
	if _, err := exec.LookPath("dlv"); err != nil {
		return fmt.Errorf("dlv not found: install with `go install github.com/go-delve/delve/cmd/dlv@latest`")
	}

	pluginPath, err := resolvePluginPath(name)
	if err != nil {
		return err
	}

	// Build the plugin binary first.
	// Into a private directory with a fixed file name, so the output path never depends
	// on the plugin name or on a predictable shared temp path.
	buildDir, err := os.MkdirTemp("", "nself-debug-")
	if err != nil {
		return fmt.Errorf("creating debug build directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(buildDir) }()
	binaryPath := filepath.Join(buildDir, "plugin-debug")
	uiInfof("Building %s for debug...", name)
	buildCmd := exec.Command("go", "build", "-gcflags=all=-N -l", "-o", binaryPath, "./cmd")
	buildCmd.Dir = pluginPath
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("building plugin for debug: %w", err)
	}

	uiSuccessf("Plugin built at %s", binaryPath)
	uiInfof("Starting dlv on :%d...", port)

	// Print VS Code launch.json snippet.
	printLaunchJSON(name, port)

	// Start dlv exec in headless mode.
	dlvCmd := exec.Command("dlv",
		"exec", binaryPath,
		"--headless",
		"--listen", fmt.Sprintf(":%d", port),
		"--api-version", "2",
		"--accept-multiclient",
	)
	dlvCmd.Dir = pluginPath
	dlvCmd.Stdout = os.Stdout
	dlvCmd.Stderr = os.Stderr
	dlvCmd.Env = append(os.Environ(), "NSELF_PLUGIN_DEV=1")

	if err := dlvCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 130 {
			return nil // SIGINT
		}
		return fmt.Errorf("dlv exited: %w", err)
	}
	return nil
}

// resolveDebugPort returns the manually specified port or auto-allocates one from 2345-2399.
func resolveDebugPort(manual int) (int, error) {
	if manual != 0 {
		if manual < 1024 || manual > 65535 {
			return 0, fmt.Errorf("invalid port %d: must be 1024-65535", manual)
		}
		return manual, nil
	}

	// Scan 2345-2399 for an available port.
	for port := 2345; port <= 2399; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available debug port in range 2345-2399 — use --port to specify one manually")
}

// printLaunchJSON prints a VS Code launch.json snippet to stdout.
func printLaunchJSON(pluginName string, port int) {
	ui.Dimmed("--- VS Code launch.json snippet ---")
	fmt.Printf(`{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Attach to %s",
      "type": "go",
      "request": "attach",
      "mode": "remote",
      "remotePath": "${workspaceFolder}",
      "port": %d,
      "host": "127.0.0.1"
    }
  ]
}
`, pluginName, port)
	ui.Dimmed("---")
	uiDimmedf("GoLand: Run > Attach to Process > Remote... > host=127.0.0.1, port=%d", port)
	ui.Dimmed("---")
}
