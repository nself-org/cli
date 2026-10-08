package admin

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// AdminConnection represents a saved remote admin connection.
type AdminConnection struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	User       string `json:"user"`
	Port       int    `json:"port"`
	LocalPort  int    `json:"local_port"`
	RemotePort int    `json:"remote_port"`
}

// AdminProject represents a project in the multi-project config.
type AdminProject struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Host    string `json:"host"`
	SSHUser string `json:"ssh_user"`
	URL     string `json:"url"`
}

func connectCommand() *cobra.Command {
	adminConnectCmd := &cobra.Command{
		Use:   "connect [host]",
		Short: "Connect to a remote nSelf Admin via SSH tunnel",
		Long: `Establish an SSH tunnel to a remote nSelf Admin instance.

Steps performed:
  1. Verify SSH key-based auth to HOST
  2. Start nself-admin on the server if not running
  3. Open local forward: -L localPort:127.0.0.1:remotePort
  4. Generate per-session token
  5. POST token to /api/auth/bootstrap (sets HttpOnly session cookie)
  6. Open browser to http://localhost:<localPort>
  7. Tear down on Ctrl+C or UI disconnect`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			allFlag, _ := cmd.Flags().GetBool("all")
			asUser, _ := cmd.Flags().GetString("as")
			user, _ := cmd.Flags().GetString("user")
			port, _ := cmd.Flags().GetInt("port")
			localPort, _ := cmd.Flags().GetInt("local-port")
			remotePort, _ := cmd.Flags().GetInt("remote-port")

			if len(args) == 0 && !allFlag {
				return fmt.Errorf("specify a host or use --all for multi-project mode")
			}

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			// Handle Ctrl+C for clean teardown
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				ui.Bullet("Disconnecting...")
				cancel()
			}()

			host := ""
			if len(args) > 0 {
				host = args[0]
			}

			if allFlag {
				return connectAllProjects(ctx, user, port, localPort, remotePort, asUser)
			}

			return connectSingle(ctx, host, user, port, localPort, remotePort, asUser)
		},
	}

	adminConnectCmd.Flags().String("user", "nself", "SSH user")
	adminConnectCmd.Flags().Int("port", 22, "SSH port")
	adminConnectCmd.Flags().Int("local-port", 3021, "Local port for tunnel")
	adminConnectCmd.Flags().Int("remote-port", 3021, "Remote admin port")
	adminConnectCmd.Flags().Bool("all", false, "Connect with all registered projects")
	adminConnectCmd.Flags().String("as", "", "Override authenticated identity (for ACL testing)")
	return adminConnectCmd
}
