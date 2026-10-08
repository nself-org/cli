package admin

import (
	"context"
	"fmt"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/httptimeout"
	"github.com/spf13/cobra"
	"net/http"
	"os"
	"os/exec"
	"time"
)

func startCommand(r runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Admin service",
		Long: `Start the nSelf Admin service.

By default the Admin UI is bound to 127.0.0.1:3021 and is only reachable from
the local machine. Use --expose-network to bind to 0.0.0.0:3021 and make it
reachable from other hosts on your network.

WARNING: --expose-network must only be used behind a TLS-terminating reverse
proxy with authentication enabled. Never expose the Admin UI to the public
internet without additional protection.`,
		RunE: r.runAdminStart,
	}

	cmd.Flags().Bool("expose-network", false, "Bind to 0.0.0.0 instead of 127.0.0.1 (WARNING: only use behind a TLS reverse proxy)")
	return cmd
}

// runAdminStart enables the admin service, rebuilds, and starts it.
// Idempotent: prints "already running" if the container is healthy.
func (r runner) runAdminStart(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	exposeNetwork, _ := cmd.Flags().GetBool("expose-network")
	port := adminPort()
	adminURL := "http://localhost:" + port + "/health"

	// --expose-network warning banner. Admin is a local-only tool; binding to
	// 0.0.0.0 exposes credentials and sensitive operations to the network.
	if exposeNetwork {
		fmt.Println("WARNING: --expose-network binds Admin to 0.0.0.0:" + port)
		fmt.Println("         Only use this behind a TLS reverse proxy with authentication.")
		fmt.Println("         Never expose Admin directly to the public internet.")
		fmt.Println()
	}

	// Check if already running.
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	probeReq, _ := http.NewRequestWithContext(probeCtx, http.MethodGet, adminURL, nil)
	resp, err := httptimeout.Health.Do(probeReq)
	if err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			fmt.Println("Admin is already running at http://localhost:" + port)
			return nil
		}
	}

	// Enable admin in .env.
	envFile, err2 := r.d.ResolveEnvFile("")
	if err2 != nil {
		return err2
	}
	if err2 := r.d.SetEnvKeyInFile(envFile, "NSELF_ADMIN_ENABLED", "true"); err2 != nil {
		return fmt.Errorf("enabling admin: %w", err2)
	}

	// Set network binding. Default is 127.0.0.1 (loopback only). When
	// --expose-network is passed, set to 0.0.0.0 so the admin is reachable
	// from other hosts on the network (use with caution, see warning above).
	adminBindHost := "127.0.0.1"
	if exposeNetwork {
		adminBindHost = "0.0.0.0"
	}
	if err2 := r.d.SetEnvKeyInFile(envFile, "NSELF_ADMIN_BIND_HOST", adminBindHost); err2 != nil {
		return fmt.Errorf("setting admin bind host: %w", err2)
	}

	// Rebuild.
	fmt.Println("Building admin service...")
	buildCmd := exec.CommandContext(ctx, "nself", "build")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err2 := buildCmd.Run(); err2 != nil {
		return fmt.Errorf("nself build: %w", err2)
	}

	// Start the admin container.
	fmt.Println("Starting admin container...")
	startCmd := exec.CommandContext(ctx, "docker", "start", r.adminContainerID())
	startCmd.Stdout = os.Stdout
	startCmd.Stderr = os.Stderr
	if err2 := startCmd.Run(); err2 != nil {
		// First run: the container does not exist yet, so `docker start`
		// cannot find it. Bring up ONLY the admin service.
		//
		// This used to shell out to `nself start admin`, which does not do
		// what it reads like: runStart ignores its arguments entirely, so
		// that call booted the WHOLE stack. On a machine where the stack was
		// already running (the E2E golden path does exactly this — step 5
		// starts the stack, step 11 starts admin) it then failed the
		// whole-stack port preflight against the project's OWN containers,
		// and ran the AI first-run wizard, which cannot install Ollama
		// without systemd. Admin is a companion tool; starting it must not
		// re-run the stack's boot sequence.
		// The compose SERVICE is "nself-admin" (compose.AdminServiceName); only
		// the container is "<project>_admin". Passing "admin" here fails with
		// "no such service".
		projectDir, composeFiles := adminComposeContext()
		c := docker.NewCompose(composeFiles...)
		if err3 := c.ComposeUpNoDeps(ctx, projectDir, compose.AdminServiceName); err3 != nil {
			return fmt.Errorf("starting admin: %w", err3)
		}
	}

	listenAddr := adminBindHost + ":" + port
	fmt.Printf("Admin started. Listening on %s\n", listenAddr)
	return nil
}
