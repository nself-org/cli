package mcpgen

// Purpose: the self-description resources of the MCP server (EPIC D5):
// nself://registry, nself://errors, nself://schemas/{name}, nself://plugins/catalog,
// and, in v1.5 mode, the four legacy project-state URIs served through the invoker.
// Inputs: the machine registry, the schema lookup, the cached plugin catalogue
// (passed in, so this package never imports internal/plugin) and an Invoker.
// Outputs: server resources and resource templates with JSON bodies.
// Constraints: no network. A legacy URI returns the command's data, never
// secrets: the commands behind them mask secrets themselves.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/invoke"
)

// ResourceOptions configures Resources.
type ResourceOptions struct {
	Registry *cmdregistry.Registry
	Invoke   Invoker
	Schema   func(name string) ([]byte, bool)
	// Catalog returns the cached plugin catalogue document, or an error when
	// there is none; it must not touch the network.
	Catalog func() ([]byte, error)
	// Fallback serves a legacy URI whose command is not exposed on machine
	// surfaces yet (for example one with no envelope), so the URI keeps working
	// until the command is converted. Keyed by URI; optional.
	Fallback map[string]server.ResourceHandlerFunc
	V15      bool
}

// URIs of the resources this package serves.
const (
	URIRegistry = "nself://registry"
	URIErrors   = "nself://errors"
	URICatalog  = "nself://plugins/catalog"
	URISchemas  = "nself://schemas/"
)

// legacyResources are the four URIs the hand server has always served.
var legacyResources = []struct{ uri, name, desc, command string }{
	{"nself://config", "Project config snapshot", "Resolved project config for the active env file, secrets redacted", "config show"},
	{"nself://services", "Service inventory", "Every optional service with its enabled state", "config services list"},
	{"nself://env", "Effective env cascade", "The effective .env cascade and which file wins, secrets redacted", "config env explain"},
	{"nself://urls", "Service URLs", "Computed service URLs for the current project", "status urls"},
}

// Resources builds the resources and templates. The legacy URIs are included
// only in v1.5 mode: in v1.4 mode the hand handlers keep serving them.
func Resources(o ResourceOptions) ([]server.ServerResource, []server.ServerResourceTemplate) {
	json := mcp.WithMIMEType("application/json")
	res := []server.ServerResource{
		{Resource: mcp.NewResource(URIRegistry, "Command registry", mcp.WithResourceDescription("The machine command registry with installed plugin commands, always in the v1.5 command spelling; in v1.5 mode it equals the data of 'nself help --json'"), json), Handler: o.registryHandler},
		{Resource: mcp.NewResource(URIErrors, "Error codes", mcp.WithResourceDescription("Every registered error code with its summary, cause, fix and exit class"), json), Handler: o.errorsHandler},
		{Resource: mcp.NewResource(URICatalog, "Plugin catalogue", mcp.WithResourceDescription("The cached plugin catalogue (read from the local cache, never fetched)"), json), Handler: o.catalogHandler},
	}
	if o.V15 {
		for _, l := range legacyResources {
			handler := o.commandHandler(l.command)
			if fb := o.Fallback[l.uri]; fb != nil && !o.exposed(l.command) {
				handler = fb
			}
			res = append(res, server.ServerResource{
				Resource: mcp.NewResource(l.uri, l.name, mcp.WithResourceDescription(l.desc+" (same data as nself "+l.command+")"), json),
				Handler:  handler,
			})
		}
	}
	var templates []server.ServerResourceTemplate
	for _, dir := range []string{"", "commands/", "invoke/"} {
		templates = append(templates, server.ServerResourceTemplate{
			Template: mcp.NewResourceTemplate(URISchemas+dir+"{name}", "Committed JSON schema",
				mcp.WithTemplateDescription("A committed JSON schema by file name ("+URISchemas+dir+"envelope.v1.schema.json)"), mcp.WithTemplateMIMEType("application/json")),
			Handler: o.schemaHandler,
		})
	}
	return res, templates
}

// exposed reports whether the registry has command and serves it over MCP.
func (o ResourceOptions) exposed(command string) bool {
	if o.Registry == nil {
		return false
	}
	c, ok := o.Registry.Lookup(command)
	if !ok {
		return false
	}
	ok, _ = invoke.Exposure(c, invoke.TransportMCP, nil)
	return ok
}

func text(uri string, body []byte) []mcp.ResourceContents {
	return []mcp.ResourceContents{mcp.TextResourceContents{URI: uri, MIMEType: "application/json", Text: string(body)}}
}

func (o ResourceOptions) registryHandler(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	if o.Registry == nil {
		return nil, fmt.Errorf("no registry")
	}
	b, err := o.Registry.Marshal()
	if err != nil {
		return nil, err
	}
	return text(req.Params.URI, b), nil
}

type errorRow struct {
	Code     string `json:"code"`
	Category string `json:"category"`
	Summary  string `json:"summary"`
	Why      string `json:"why"`
	Fix      string `json:"fix"`
	Docs     string `json:"docs_path"`
	Exit     int    `json:"exit"`
}

func (o ResourceOptions) errorsHandler(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	rows := make([]errorRow, 0, len(errs.Registry))
	for _, e := range errs.Registry {
		rows = append(rows, errorRow{e.Code, e.Category, e.Summary, e.DefaultWhy, e.DefaultFix, e.DocsPath, e.Exit})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Code < rows[j].Code })
	b, err := json.MarshalIndent(map[string]any{"codes": rows}, "", "  ")
	if err != nil {
		return nil, err
	}
	return text(req.Params.URI, append(b, '\n')), nil
}

func (o ResourceOptions) schemaHandler(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	name := strings.TrimPrefix(req.Params.URI, URISchemas)
	if o.Schema == nil || name == req.Params.URI {
		return nil, fmt.Errorf("no schema %q", req.Params.URI)
	}
	b, ok := o.Schema(name)
	if !ok {
		return nil, fmt.Errorf("no committed schema %q", name)
	}
	return text(req.Params.URI, b), nil
}

func (o ResourceOptions) catalogHandler(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	if o.Catalog == nil {
		return nil, fmt.Errorf("no plugin catalogue is available")
	}
	b, err := o.Catalog()
	if err != nil {
		return nil, err
	}
	return text(req.Params.URI, b), nil
}

// commandHandler serves a command's envelope data as a resource.
func (o ResourceOptions) commandHandler(command string) server.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		res, err := o.Invoke(ctx, command, invoke.Request{})
		if err != nil {
			return nil, fmt.Errorf("nself %s: %w", command, err)
		}
		var doc envelopeDoc
		if json.Unmarshal(res.Stdout, &doc) != nil || doc.SchemaVersion == "" {
			return nil, fmt.Errorf("nself %s did not return an envelope", command)
		}
		if len(doc.Error) > 0 && string(doc.Error) != "null" {
			var e struct{ Code, Message string }
			_ = json.Unmarshal(doc.Error, &e)
			return nil, fmt.Errorf("nself %s: %s %s", command, e.Code, e.Message)
		}
		var buf bytes.Buffer
		if json.Indent(&buf, doc.Data, "", "  ") != nil {
			buf.Reset()
			buf.Write(doc.Data)
		}
		buf.WriteByte('\n')
		return text(req.Params.URI, buf.Bytes()), nil
	}
}
