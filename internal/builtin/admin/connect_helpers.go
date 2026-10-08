package admin

// Purpose: the SSH tunnel helpers used by "nself admin connect":
// connectSingle for one project and connectAllProjects for the whole saved
// set. Inputs are a context, host/user/port settings; outputs are an open
// tunnel session or an error.
// Constraints: split out of admin_connect.go (CLI-R12) as a pure move, no behavior change.

import (
	"context"
	"fmt"

	"github.com/nself-org/cli/internal/admin"
	"github.com/nself-org/cli/internal/ui"
)

func connectSingle(ctx context.Context, host, user string, port, localPort, remotePort int, asUser string) error {
	// Step 1: Verify SSH key auth
	ui.Bullet(fmt.Sprintf("Verifying SSH key auth for %s@%s:%d...", user, host, port))
	if err := admin.VerifySSHKey(ctx, user, host, port); err != nil {
		return err
	}
	ui.Success("SSH key auth verified")

	// Step 2: Start nself-admin on remote if needed
	ui.Bullet("Ensuring nself-admin is running on remote host...")
	if err := admin.EnsureRemoteAdmin(ctx, user, host, port); err != nil {
		ui.Warn(fmt.Sprintf("Could not start remote admin: %v", err))
	}

	// Step 3: Generate session token
	token, err := admin.NewSessionToken()
	if err != nil {
		return err
	}

	// Save connection config
	conn := AdminConnection{
		Name:       host,
		Host:       host,
		User:       user,
		Port:       port,
		LocalPort:  localPort,
		RemotePort: remotePort,
	}
	if saveErr := saveAdminConnection(conn); saveErr != nil {
		ui.Warn(fmt.Sprintf("Failed to save connection: %v", saveErr))
	}

	// Step 4: Open SSH tunnel
	opts := admin.ConnectOpts{
		Host:       host,
		User:       user,
		SSHPort:    port,
		LocalPort:  localPort,
		RemotePort: remotePort,
		AsUser:     asUser,
	}

	ui.Bullet(fmt.Sprintf("Opening SSH tunnel: localhost:%d -> %s:127.0.0.1:%d", localPort, host, remotePort))
	tunnelCmd, err := admin.OpenTunnel(ctx, opts)
	if err != nil {
		return err
	}

	// Step 5: Bootstrap session token via localhost POST (token never goes in URL)
	if bsErr := admin.BootstrapSession(localPort, token); bsErr != nil {
		ui.Warn(fmt.Sprintf("Could not bootstrap session: %v", bsErr))
	}

	// Step 6: Open browser — URL carries no token
	adminURL := admin.AdminURL(localPort, "")
	ui.Success(fmt.Sprintf("Connected. Admin UI: %s", adminURL))
	if openErr := admin.OpenBrowser(adminURL); openErr != nil {
		ui.Warn(fmt.Sprintf("Could not open browser: %v (open manually: %s)", openErr, adminURL))
	}

	ui.Bullet("Press Ctrl+C to disconnect")

	// Step 6: Wait for tunnel to end (Ctrl+C or disconnect)
	return tunnelCmd.Wait()
}

func connectAllProjects(ctx context.Context, defaultUser string, sshPort, localPort, remotePort int, asUser string) error {
	projects, err := loadAdminProjects()
	if err != nil {
		return fmt.Errorf("load projects: %w", err)
	}
	if len(projects) == 0 {
		return fmt.Errorf("no projects configured; use 'nself admin projects add' first")
	}

	// Connect to the first project by default
	p := projects[0]
	host := p.Host
	if host == "" {
		return fmt.Errorf("project %q has no host configured", p.Name)
	}
	sshUser := p.SSHUser
	if sshUser == "" {
		sshUser = defaultUser
	}

	ui.CommandHeader("Multi-Project Admin", fmt.Sprintf("%d projects registered", len(projects)))
	for _, proj := range projects {
		fmt.Printf("  %-20s  %s\n", proj.ID, proj.Host)
	}

	return connectSingle(ctx, host, sshUser, sshPort, localPort, remotePort, asUser)
}
