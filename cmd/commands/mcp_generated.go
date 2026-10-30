package commands

// mcp_generated.go — wires the generated MCP surface into `nself mcp`.
//
// Purpose: EPIC P7-SURF D5. The tools, the self-description resources and, in
// v1.5 mode, the four project-state resources come from internal/mcpgen over
// the machine registry; the 26 hand tools in mcp.go and mcp_sentry.go stay
// registered in v1.4 mode only (P7-SHIP-09 deletes them at the flip).
// Inputs:  the transport name (stdio|sse|http), the compat mode.
// Outputs: tools, resources and resource templates on the MCP server.
// Constraints: machineRegistry is built at server start, before the transport
// serves, and again when mcpgen.Surface sees the project or the plugin set
// change; the invoker runs every call as a child of this binary through the
// confirmation gate, with one Guard per server process.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/mcpgen"
	"github.com/nself-org/cli/internal/plugin/catalog"
	"github.com/nself-org/cli/schemas"
)

// newMCPServer builds the MCP server: v1.4 lists the 26 hand tools and four hand
// resources, then the generated ones; v1.5 only the generated surface
// (mcpgen.Surface keeps the list live). timeout 0 is invoke.DefaultTimeout.
func newMCPServer(transport string, v15 bool, timeout time.Duration) (*server.MCPServer, error) {
	s := server.NewMCPServer(mcpgen.ServerName, mcpgen.ServerVersion, mcpgen.ServerOptions()...)
	o := mcpgen.SurfaceOptions{Server: s, V15: v15, Schema: schemas.Lookup, PluginDir: resolvePluginDir,
		Project: func() string { d, _ := resolveProjectDir(); return d },
		Warn:    func(msg string) { fmt.Fprintln(os.Stderr, "nself mcp: "+msg) }}
	if !v15 {
		o.Hand = registerMCPTools
		registerMCPResources(s)
	}
	if err := registerGeneratedMCP(o, transport, timeout); err != nil {
		if v15 {
			return nil, err
		}
		// v1.4 mode: the generated tools are additive, so a
		// registry that cannot be built (a broken plugin manifest) never takes
		// the 26 hand tools down with it.
		fmt.Fprintf(os.Stderr, "nself mcp: generated tools are unavailable, serving the built-in tools only: %v\n", err)
	}
	registerMCPPrompts(s)
	return s, nil
}

// mcpAuditSurface is the audit label of an MCP transport.
func mcpAuditSurface(transport string) string { return "mcp-" + transport }

// registerGeneratedMCP lists the tools (the hand tools stay listed on an
// error) and attaches the generated resources.
func registerGeneratedMCP(o mcpgen.SurfaceOptions, transport string, timeout time.Duration) error {
	auditPath, auditErr := invoke.DefaultAuditPath()
	guard := invoke.NewGuard(auditPath)
	o.Registry = func(rebuild bool) (*cmdregistry.Registry, error) {
		if auditErr != nil {
			return nil, fmt.Errorf("machine audit log path: %w", auditErr)
		} else if rebuild {
			resetMachineRegistry()
		}
		return machineRegistry()
	}
	o.Run = func(ctx context.Context, reg *cmdregistry.Registry, path string, req invoke.Request, dir string) (invoke.Result, error) {
		return invoke.Invoke(ctx, reg, path, req, invoke.Options{Transport: invoke.TransportMCP, Surface: mcpAuditSurface(transport), Dir: dir, Guard: guard, Timeout: timeout})
	}
	surf := mcpgen.NewSurface(o)
	reg, err := surf.Start()
	if err != nil {
		return err
	}
	resources, templates := mcpgen.Resources(mcpgen.ResourceOptions{
		Registry: reg, Invoke: surf.Invoker(reg), Schema: schemas.Lookup, Catalog: cachedCatalogJSON, V15: o.V15,
		// A legacy URI whose command has no envelope yet keeps its hand handler.
		Fallback: map[string]server.ResourceHandlerFunc{
			"nself://config": mcpConfigResourceHandler(), "nself://services": mcpServicesResourceHandler(),
			"nself://env": mcpEnvResourceHandler(), "nself://urls": mcpURLsResourceHandler(),
		},
	})
	o.Server.AddResources(resources...)
	o.Server.AddResourceTemplates(templates...)
	return nil
}

// cachedCatalogJSON is the plugin catalogue exactly as the local cache holds
// it; it never fetches (a server start must not depend on the network). The
// bytes are read once and decoded before they are served, so a corrupt or
// replaced file is "no cache", not a resource.
func cachedCatalogJSON() ([]byte, error) {
	dir := catalog.DefaultCacheDir()
	if dir == "" {
		return nil, errs.New("E115", "no plugin catalogue cache directory")
	}
	b, err := os.ReadFile(filepath.Join(dir, catalogCacheFile))
	if err != nil {
		return nil, errs.New("E115", "no cached plugin catalogue yet").WithFix("Run nself plugin search once while online.")
	}
	if _, err := catalog.Decode(b); err != nil {
		return nil, err
	}
	return b, nil
}

// catalogCacheFile is the body file name internal/plugin/catalog documents.
const catalogCacheFile = "catalog.json"
