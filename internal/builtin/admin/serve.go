package admin

// Purpose: serve the authenticated registry HTTP API from the admin family.
// Inputs: registry/schema providers, project resolver, bind flags and token source.
// Outputs: one HTTP listener and diagnostic lines on stderr.
// Constraints: token always; non-loopback binds require explicit configuration.

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/httpapi"
	"github.com/nself-org/cli/internal/machineauth"
	"github.com/nself-org/cli/internal/ports"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// ServeDeps supplies composition-root providers without importing commands.
type ServeDeps struct {
	// Registry supplies the command snapshot used by the server.
	Registry func() (*cmdregistry.Registry, error)
	// DataSchema provides committed command data schemas for OpenAPI.
	DataSchema func(string) ([]byte, bool)
	// ProjectDir resolves the invocation working directory.
	ProjectDir func() (string, error)
	// Listen binds the listener before announcing readiness; nil uses net.Listen.
	Listen func(network, address string) (net.Listener, error)
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
	flag := cmd.Flags().Lookup("allowed-host")
	flag.Value = allowedHostsValue{Value: flag.Value, SliceValue: flag.Value.(pflag.SliceValue)}
	cmd.Flags().DurationVar(&timeout, "timeout", 300*time.Second, "Document invocation timeout")
	return cmd
}

// allowedHostsValue preserves string-slice flags while rejecting input that CSV
// would discard, including empty occurrences and extra newline records.
type allowedHostsValue struct {
	pflag.Value
	pflag.SliceValue
}

func (v allowedHostsValue) Set(raw string) error {
	if raw == "" || strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return errs.Newf("E430", "Invalid allowed host %q: hosts must be non-empty and contain no whitespace", raw).
			WithWhy("an allowed host is a DNS name, an IPv4 literal or a bracketed IPv6 literal, without scheme, port or path").
			WithFix("pass --allowed-host <name>; the server adds the port itself")
	}
	return v.Value.Set(raw)
}

func runServe(cmd *cobra.Command, d ServeDeps, host string, port int, allowed []string, timeout time.Duration) error {
	if cmd.Flags().Changed("allowed-host") && len(allowed) == 0 {
		return httpapi.ValidateAllowedHosts([]string{""})
	}
	if err := httpapi.ValidateAllowedHosts(allowed); err != nil {
		return err
	}
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
	listen := d.Listen
	if listen == nil {
		listen = net.Listen
	}
	ln, err := listen("tcp", addr)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "nself: HTTP API listening on http://%s\n", ln.Addr())
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return s.Serve(ctx, ln)
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
	if len(tok) < 32 || strings.IndexFunc(tok, unicode.IsSpace) >= 0 {
		path, err := machineauth.TokenPath()
		if err != nil {
			return "", err
		}
		return "", errs.New("E426", "non-loopback HTTP API requires a token of at least 32 bytes without whitespace").
			WithFix("Run nself admin serve on loopback to create " + path + "; use its generated token for the non-loopback bind.")
	}
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "nself: warning: bearer tokens cross the network over cleartext HTTP; use a trusted network or TLS proxy")
	return tok, nil
}
