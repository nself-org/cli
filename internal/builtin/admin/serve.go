package admin

// Purpose: serve the authenticated registry HTTP API from the admin family.
// Inputs: registry/schema providers, project resolver, bind flags and token source.
// Outputs: one HTTP listener and diagnostic lines on stderr.
// Constraints: token always; non-loopback binds require explicit configuration.

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/httpapi"
	"github.com/nself-org/cli/internal/machineauth"
	"github.com/nself-org/cli/internal/ports"
	"github.com/spf13/cobra"
)

// ServeDeps supplies composition-root providers without importing commands.
type ServeDeps struct {
	Registry   func() (*cmdregistry.Registry, error)
	DataSchema func(string) ([]byte, bool)
	ProjectDir func() (string, error)
	// Listen is a test seam; nil uses Server.ListenAndServe.
	Listen func(context.Context, *httpapi.Server, string) error
}

// ServeCommand creates an API-only server command, independently of compat mode.
func ServeCommand(d ServeDeps) *cobra.Command {
	var host string
	var port int
	var allowed []string
	var timeout time.Duration
	cmd := &cobra.Command{
		Use: "serve", Short: "Serve the authenticated nSelf HTTP API",
		Long: "Serve the experimental registry HTTP API. A bearer token is always required.\nNon-loopback hosts require --allowed-host and a configured NSELF_MCP_TOKEN or token file.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd, d, host, port, allowed, timeout)
		},
	}
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "Bind address")
	cmd.Flags().IntVar(&port, "port", ports.AdminAPIPort, "HTTP API port")
	cmd.Flags().StringSliceVar(&allowed, "allowed-host", nil, "Additional reachable host name (repeatable; required for non-loopback)")
	cmd.Flags().DurationVar(&timeout, "timeout", 300*time.Second, "Document invocation timeout")
	return cmd
}

func runServe(cmd *cobra.Command, d ServeDeps, host string, port int, allowed []string, timeout time.Duration) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if timeout < 0 {
		return fmt.Errorf("timeout must not be negative")
	}
	if d.ProjectDir == nil || d.Registry == nil {
		return fmt.Errorf("HTTP API dependencies are not configured")
	}
	dir, err := d.ProjectDir()
	if err != nil {
		return err
	}
	tok, err := serveToken(cmd, host, allowed)
	if err != nil {
		return err
	}
	reg, err := d.Registry()
	if err != nil {
		return err
	}
	s, err := httpapi.New(httpapi.Options{
		Registry: reg, ProjectDir: dir, Port: port, AllowedHosts: allowed,
		Timeout: timeout, Auth: httpapi.BearerAuth(tok), DataSchema: d.DataSchema,
		Log: cmd.ErrOrStderr(),
	})
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"), strconv.Itoa(port))
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "nself: HTTP API listening on http://%s\n", addr)
	if d.Listen != nil {
		return d.Listen(cmd.Context(), s, addr)
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return s.ListenAndServe(ctx, addr)
}

func serveToken(cmd *cobra.Command, host string, allowed []string) (string, error) {
	if machineauth.IsLoopback(host) {
		tok, _, _, err := machineauth.ResolveTo(cmd.ErrOrStderr())
		return tok, err
	}
	tok, source, err := machineauth.Lookup()
	if err != nil {
		return "", err
	}
	if err := machineauth.CheckBind(host, allowed, source); err != nil {
		return "", err
	}
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "nself: warning: bearer tokens cross the network over cleartext HTTP; use a trusted network or TLS proxy")
	return tok, nil
}
