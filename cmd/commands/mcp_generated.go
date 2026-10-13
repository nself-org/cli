package commands

// mcp_generated.go — wires the generated MCP surface into `nself mcp`.
//
// Purpose: EPIC P7-SURF D5. The tools, the self-description resources and, in
// v1.5 mode, the four project-state resources come from internal/mcpgen over
// the machine registry; the 26 hand tools in mcp.go and mcp_sentry.go stay
// registered in v1.4 mode only (P7-SHIP-09 deletes them at the flip).
// Inputs:  the transport name (stdio|sse|http), the compat mode.
// Outputs: tools, resources and resource templates on the MCP server.
// Constraints: machineRegistry is built once, at server start, before the
// transport serves; the invoker runs every call as a child of this binary
// through the confirmation gate, with one Guard per server process.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/mcpgen"
	"github.com/nself-org/cli/internal/plugin/catalog"
	"github.com/nself-org/cli/schemas"
)

// newMCPServer builds the MCP server for the compat mode: v1.4 registers the
// 26 hand tools and the four hand resources first and the generated ones on
// top; v1.5 registers the generated surface only. Prompts are authored.
func newMCPServer(transport string, v15 bool) (*server.MCPServer, error) {
	s := server.NewMCPServer(mcpgen.ServerName, mcpgen.ServerVersion, mcpgen.ServerOptions(v15)...)
	if !v15 {
		registerMCPTools(s)
		registerMCPResources(s)
	}
	if err := registerGeneratedMCP(s, transport, v15); err != nil {
		if v15 {
			return nil, err
		}
		// v1.4 mode: the generated tools are additive and experimental, so a
		// registry that cannot be built (a broken plugin manifest) never takes
		// the 26 hand tools down with it.
		fmt.Fprintf(os.Stderr, "nself mcp: generated tools are unavailable, serving the built-in tools only: %v\n", err)
	}
	registerMCPPrompts(s)
	return s, nil
}

// mcpAuditSurface is the audit label of an MCP transport.
func mcpAuditSurface(transport string) string { return "mcp-" + transport }

// registerGeneratedMCP attaches the generated tools and resources to s.
func registerGeneratedMCP(s *server.MCPServer, transport string, v15 bool) error {
	reg, err := machineRegistry()
	if err != nil {
		return err
	}
	dir, err := resolveProjectDir()
	if err != nil {
		return err
	}
	auditPath, err := invoke.DefaultAuditPath()
	if err != nil {
		return fmt.Errorf("machine audit log path: %w", err)
	}
	guard := invoke.NewGuard(auditPath)
	inv := func(ctx context.Context, path string, req invoke.Request) (invoke.Result, error) {
		return invoke.Invoke(ctx, reg, path, req, invoke.Options{
			Transport: invoke.TransportMCP, Surface: mcpAuditSurface(transport), Dir: dir, Guard: guard,
		})
	}
	tools, err := mcpgen.Tools(mcpgen.Options{Registry: reg, Invoke: inv, Schema: schemas.Lookup, V15: v15})
	if err != nil {
		return err
	}
	s.AddTools(tools...)
	resources, templates := mcpgen.Resources(mcpgen.ResourceOptions{
		Registry: reg, Invoke: inv, Schema: schemas.Lookup, Catalog: cachedCatalogJSON, V15: v15,
		// A legacy URI whose command has no envelope yet keeps its hand handler.
		Fallback: map[string]server.ResourceHandlerFunc{
			"nself://config": mcpConfigResourceHandler(), "nself://services": mcpServicesResourceHandler(),
			"nself://env": mcpEnvResourceHandler(), "nself://urls": mcpURLsResourceHandler(),
		},
	})
	s.AddResources(resources...)
	s.AddResourceTemplates(templates...)
	return nil
}

// cachedCatalogJSON is the plugin catalogue exactly as the local cache holds
// it; it never fetches (a server start must not depend on the network). The
// cache is decoded first, so a corrupt file is "no cache", not a resource.
func cachedCatalogJSON() ([]byte, error) {
	dir := catalog.DefaultCacheDir()
	if _, err := catalog.LoadCached(dir); err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(dir, catalogCacheFile))
}

// catalogCacheFile is the body file name internal/plugin/catalog documents.
const catalogCacheFile = "catalog.json"
