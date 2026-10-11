package commands

// mcp_resources.go — MCP resources for the nSelf MCP server.
//
// Purpose: CLI-R15. Resources let an MCP client read project state without
//   framing it as a tool call — the four nSelf-specific ones an agent needs
//   before making a plan: the resolved config, the service catalog, the raw
//   env-cascade inputs, and the computed URLs.
// Inputs:  none beyond the current working directory (each handler resolves
//   the nSelf project root itself, same as the tool handlers).
// Outputs: mcp.TextResourceContents with a JSON body.
// Constraints: nself://env intentionally does NOT claim to compute which
//   file wins for a given key — internal/config's cascade order is being
//   revised under CLI-R18 in a parallel change, and this ticket must not
//   touch internal/config or cmd/commands/env.go. It lists each candidate
//   env file that exists, redacted, and points at `nself env explain` (the
//   CLI-R18 deliverable) as the authority on precedence. Secrets are always
//   redacted (reused from config.IsSecretKey/maskValue).
// SPORT: CLI-CMD-MCP-001

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/joho/godotenv"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

// registerMCPResources attaches the four project-state resources.
func registerMCPResources(s *server.MCPServer) {
	s.AddResource(
		mcp.NewResource("nself://config", "Project config snapshot",
			mcp.WithResourceDescription("Resolved project config for the active env file, secrets redacted"),
			mcp.WithMIMEType("application/json"),
		),
		mcpConfigResourceHandler(),
	)
	s.AddResource(
		mcp.NewResource("nself://services", "Service inventory",
			mcp.WithResourceDescription("The compose service catalog: required + optional services, their enable-env, and pinned image"),
			mcp.WithMIMEType("application/json"),
		),
		mcpServicesResourceHandler(),
	)
	s.AddResource(
		mcp.NewResource("nself://env", "Effective env cascade inputs",
			mcp.WithResourceDescription("Every env file present in the project, secrets redacted. Does not compute precedence — see 'nself env explain'"),
			mcp.WithMIMEType("application/json"),
		),
		mcpEnvResourceHandler(),
	)
	s.AddResource(
		mcp.NewResource("nself://urls", "Service URLs",
			mcp.WithResourceDescription("Computed service URLs for the current project (same data as the nself_urls tool)"),
			mcp.WithMIMEType("application/json"),
		),
		mcpURLsResourceHandler(),
	)
}

// mcpJSONResourceContents marshals v and wraps it as a single text resource.
func mcpJSONResourceContents(uri string, v interface{}) ([]mcp.ResourceContents, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal resource %s: %w", uri, err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{URI: uri, MIMEType: "application/json", Text: string(data)},
	}, nil
}

func mcpConfigResourceHandler() server.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		projectDir, err := resolveProjectDir()
		if err != nil {
			return nil, err
		}
		envFile := envFileName(projectDir, "")
		pairs, err := godotenv.Read(envFile)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", envFile, err)
		}
		values := make(map[string]string, len(pairs))
		for k, v := range pairs {
			values[k] = maskValue(k, v, false)
		}
		return mcpJSONResourceContents(req.Params.URI, ConfigShowResult{EnvFile: envFile, Values: values})
	}
}

func mcpServicesResourceHandler() server.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return mcpJSONResourceContents(req.Params.URI, compose.ServiceCatalog())
	}
}

// envCascadeFile is one candidate env file's redacted contents.
type envCascadeFile struct {
	File   string            `json:"file"`
	Exists bool              `json:"exists"`
	Values map[string]string `json:"values,omitempty"`
}

// envCascadeSnapshot is the structured output of the nself://env resource.
type envCascadeSnapshot struct {
	Files []envCascadeFile `json:"files"`
	Note  string           `json:"note"`
}

// knownEnvCascadeFiles lists every filename nSelf has ever cascaded from
// (old and new order alike) so this resource stays accurate across the
// CLI-R18 order change without hardcoding whichever order wins.
var knownEnvCascadeFiles = []string{
	".env", ".env.dev", ".env.staging", ".env.prod",
	".env.secrets", ".env.local", ".env.ai",
}

func mcpEnvResourceHandler() server.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		projectDir, err := resolveProjectDir()
		if err != nil {
			return nil, err
		}

		snapshot := envCascadeSnapshot{
			Note: "raw per-file contents only; precedence is not computed here — run 'nself env explain [VAR]' for the authoritative winner",
		}
		for _, name := range knownEnvCascadeFiles {
			path := filepath.Join(projectDir, name)
			entry := envCascadeFile{File: name}
			if _, statErr := os.Stat(path); statErr == nil {
				entry.Exists = true
				if pairs, readErr := godotenv.Read(path); readErr == nil {
					entry.Values = make(map[string]string, len(pairs))
					for k, v := range pairs {
						entry.Values[k] = maskValue(k, v, false)
					}
				}
			}
			snapshot.Files = append(snapshot.Files, entry)
		}
		sort.Slice(snapshot.Files, func(i, j int) bool { return snapshot.Files[i].File < snapshot.Files[j].File })
		return mcpJSONResourceContents(req.Params.URI, snapshot)
	}
}

func mcpURLsResourceHandler() server.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		raw, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cwd, err := config.FindNSelfRoot(raw)
		if err != nil {
			return nil, fmt.Errorf("no nself project found in %s — run 'nself init' first", raw)
		}
		cfg, err := config.Load(cwd)
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
		return mcpJSONResourceContents(req.Params.URI, buildURLOutput(cfg, true))
	}
}
