// Package apidocs generates an OpenAPI 3.1 spec from the Hasura introspection
// result and any registered plugin REST routes. The output is written to
// .nself/dist/openapi.json and served by nginx at /api-docs. A self-contained
// Scalar HTML page is served at /docs.
package apidocs

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed scalar.html
var scalarHTML []byte

// ApiDocsConfig is the input type consumed by the apidocs package.
// The build orchestrator maps config.ApiDocsConfig → apidocs.ApiDocsConfig
// to avoid an import cycle (apidocs must not import config).
type ApiDocsConfig struct {
	Enabled         bool
	Path            string
	Title           string
	Theme           string   // default | moon | purple | solarized
	AuthEnvVar      string   // env var containing anon key for try-out pre-fill
	HideEndpoints   []string // path prefixes to exclude
	GraphQLEnabled  bool
	GraphQLEndpoint string // default: /v1/graphql
}

// OpenAPISpec is the top-level OpenAPI 3.1 document.
type OpenAPISpec struct {
	OpenAPI    string                     `json:"openapi"`
	Info       OpenAPIInfo                `json:"info"`
	Servers    []OpenAPIServer            `json:"servers"`
	Paths      map[string]OpenAPIPathItem `json:"paths"`
	Components OpenAPIComponents          `json:"components"`
}

// OpenAPIInfo holds the API metadata.
type OpenAPIInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
	// x-nself-generated marks this as a generated spec.
	Extensions map[string]interface{} `json:"-"`
}

// OpenAPIServer lists base URLs.
type OpenAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

// OpenAPIPathItem holds operations for a single path.
type OpenAPIPathItem map[string]*OpenAPIOperation

// OpenAPIOperation is one HTTP operation.
type OpenAPIOperation struct {
	Summary     string                     `json:"summary"`
	OperationID string                     `json:"operationId"`
	Tags        []string                   `json:"tags,omitempty"`
	RequestBody *OpenAPIRequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]OpenAPIResponse `json:"responses"`
	Security    []map[string][]string      `json:"security,omitempty"`
	// x-nself-websocket marks subscription operations.
	XNselfWebsocket bool `json:"x-nself-websocket,omitempty"`
}

// OpenAPIRequestBody describes the request payload.
type OpenAPIRequestBody struct {
	Required bool                        `json:"required"`
	Content  map[string]OpenAPIMediaType `json:"content"`
}

// OpenAPIMediaType wraps a schema reference.
type OpenAPIMediaType struct {
	Schema OpenAPISchemaRef `json:"schema"`
}

// OpenAPISchemaRef is a JSON Schema reference or inline schema.
type OpenAPISchemaRef struct {
	Ref        string                      `json:"$ref,omitempty"`
	Type       string                      `json:"type,omitempty"`
	Properties map[string]OpenAPISchemaRef `json:"properties,omitempty"`
	Items      *OpenAPISchemaRef           `json:"items,omitempty"`
	Nullable   bool                        `json:"nullable,omitempty"`
}

// OpenAPIResponse describes one response.
type OpenAPIResponse struct {
	Description string                      `json:"description"`
	Content     map[string]OpenAPIMediaType `json:"content,omitempty"`
}

// OpenAPIComponents holds reusable schemas and security schemes.
type OpenAPIComponents struct {
	SecuritySchemes map[string]OpenAPISecurityScheme `json:"securitySchemes,omitempty"`
	Schemas         map[string]OpenAPISchemaRef      `json:"schemas,omitempty"`
}

// OpenAPISecurityScheme defines an auth method.
type OpenAPISecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
	Name         string `json:"name,omitempty"`
	In           string `json:"in,omitempty"`
}

// GenerateResult is returned from Generate.
type GenerateResult struct {
	// OpenAPIPath is the absolute path to the written openapi.json.
	OpenAPIPath string
	// ScalarHTMLPath is the absolute path to the written scalar.html.
	ScalarHTMLPath string
}

// Render builds the OpenAPI 3.1 spec and the rendered Scalar page and returns
// their bytes without writing anything. A disabled config returns nils.
//
// projectName is used as the default API title; baseDomain is the primary
// domain (for the Servers list); pluginRoutes are REST routes contributed by
// plugins (from plugin_routes.go). Generate and the build's plan mode
// (internal/build, P7-LIVE-21) share this one renderer.
func Render(projectName, baseDomain string, cfg ApiDocsConfig, pluginRoutes []PluginRoute) (openapi, scalar []byte, err error) {
	if !cfg.Enabled {
		return nil, nil, nil
	}

	title := cfg.Title
	if title == "" {
		title = projectName + " API"
	}
	graphqlEndpoint := cfg.GraphQLEndpoint
	if graphqlEndpoint == "" {
		graphqlEndpoint = "/v1/graphql"
	}

	spec := buildSpec(title, baseDomain, cfg, pluginRoutes, graphqlEndpoint)

	openapi, err = json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling openapi spec: %w", err)
	}

	// Render scalar.html with project-specific values substituted.
	return openapi, []byte(renderScalarHTML(string(scalarHTML), cfg, title)), nil
}

// Generate builds the OpenAPI 3.1 spec and writes both dist files:
//   - .nself/dist/openapi.json
//   - .nself/dist/scalar.html
//
// workdir is the nSelf project root (contains .nself/).
// projectName is used as the default API title.
// baseDomain is the primary domain (for the Servers list).
// cfg is the resolved ApiDocsConfig.
// pluginRoutes are REST routes contributed by plugins (from plugin_routes.go).
func Generate(workdir, projectName, baseDomain string, cfg ApiDocsConfig, pluginRoutes []PluginRoute) (*GenerateResult, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	distDir := filepath.Join(workdir, ".nself", "dist")
	if err := os.MkdirAll(distDir, 0755); err != nil {
		return nil, fmt.Errorf("creating dist dir: %w", err)
	}

	raw, scalar, err := Render(projectName, baseDomain, cfg, pluginRoutes)
	if err != nil {
		return nil, err
	}

	openapiPath := filepath.Join(distDir, "openapi.json")
	if err := os.WriteFile(openapiPath, raw, 0644); err != nil {
		return nil, fmt.Errorf("writing openapi.json: %w", err)
	}

	scalarPath := filepath.Join(distDir, "scalar.html")
	if err := os.WriteFile(scalarPath, scalar, 0644); err != nil {
		return nil, fmt.Errorf("writing scalar.html: %w", err)
	}

	return &GenerateResult{
		OpenAPIPath:    openapiPath,
		ScalarHTMLPath: scalarPath,
	}, nil
}
