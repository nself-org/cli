package mcpgen_test

// Purpose: the self-description resources, the legacy URIs in v1.5 mode and the
// server identity.
// Constraints: the catalogue is a function returning fixed bytes; no network.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/mcpgen"
	"github.com/nself-org/cli/schemas"
)

func readResource(t *testing.T, h func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error), uri string) (string, error) {
	t.Helper()
	req := mcp.ReadResourceRequest{}
	req.Params.URI = uri
	out, err := h(context.Background(), req)
	if err != nil {
		return "", err
	}
	if len(out) != 1 {
		t.Fatalf("%s: %d contents", uri, len(out))
	}
	return out[0].(mcp.TextResourceContents).Text, nil
}

func TestResources(t *testing.T) {
	h := newHarness(t, true, false, false)
	catalog := []byte(`{"schema_version":1,"plugins":[]}` + "\n")
	opts := mcpgen.ResourceOptions{Registry: h.reg, Invoke: h.invoke, Schema: schemas.Lookup, V15: true,
		Catalog: func() ([]byte, error) { return catalog, nil }}
	res, templates := mcpgen.Resources(opts)
	byURI := map[string]server.ResourceHandlerFunc{}
	for _, r := range res {
		byURI[r.Resource.URI] = r.Handler
	}
	for _, uri := range []string{"nself://registry", "nself://errors", "nself://plugins/catalog",
		"nself://config", "nself://services", "nself://env", "nself://urls"} {
		if byURI[uri] == nil {
			t.Errorf("resource %s is not registered in v1.5 mode", uri)
		}
	}
	if len(templates) == 0 || templates[0].Template.URITemplate.Raw() != "nself://schemas/{name}" {
		t.Errorf("schema template: %+v", templates)
	}

	// nself://registry is the registry document of help --json.
	got, err := readResource(t, byURI["nself://registry"], "nself://registry")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := h.reg.Marshal()
	if got != string(want) {
		t.Error("nself://registry differs from the registry document")
	}

	// nself://errors lists every registered code.
	body, err := readResource(t, byURI["nself://errors"], "nself://errors")
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Codes []struct{ Code, Summary string }
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Codes) != len(errs.Registry) || len(list.Codes) < 100 {
		t.Errorf("%d codes listed, %d registered", len(list.Codes), len(errs.Registry))
	}
	seen := map[string]bool{}
	for _, c := range list.Codes {
		seen[c.Code] = true
		if c.Summary == "" {
			t.Errorf("%s has no summary", c.Code)
		}
	}
	for code := range errs.Registry {
		if !seen[code] {
			t.Errorf("code %s is not listed", code)
		}
	}
	if !seen["E433"] {
		t.Error("E433 is not listed")
	}

	// nself://schemas/<name> is the committed bytes, nested names included.
	schemaHandler := templates[0].Handler
	for uri, name := range map[string]string{
		"nself://schemas/envelope.v1.schema.json":            "envelope.v1.schema.json",
		"nself://schemas/commands/status.v1.schema.json":     "commands/status.v1.schema.json",
		"nself://schemas/invoke/confirmation.v1.schema.json": "invoke/confirmation.v1.schema.json",
	} {
		wantBytes, ok := schemas.Lookup(name)
		if !ok {
			t.Fatalf("no committed schema %s", name)
		}
		got, err := readResource(t, schemaHandler, uri)
		if err != nil || got != string(wantBytes) {
			t.Errorf("%s: err %v, bytes equal %v", uri, err, got == string(wantBytes))
		}
	}
	for _, uri := range []string{"nself://schemas/nope.json", "nself://schemas/../go.mod", "nself://schemas//etc/passwd", "nself://registry"} {
		if _, err := readResource(t, schemaHandler, uri); err == nil {
			t.Errorf("%s was served", uri)
		}
	}

	// nself://plugins/catalog is the cached catalogue; an absent cache is an error.
	if got, err := readResource(t, byURI["nself://plugins/catalog"], "nself://plugins/catalog"); err != nil || got != string(catalog) {
		t.Errorf("catalog %q %v", got, err)
	}
	opts.Catalog = func() ([]byte, error) { return nil, errors.New("no cached catalogue") }
	res2, _ := mcpgen.Resources(opts)
	for _, r := range res2 {
		if r.Resource.URI == "nself://plugins/catalog" {
			if _, err := readResource(t, r.Handler, r.Resource.URI); err == nil || !strings.Contains(err.Error(), "no cached") {
				t.Errorf("a missing cache returned %v", err)
			}
		}
	}

	// The four legacy URIs come from the command's data.
	body, err = readResource(t, byURI["nself://urls"], "nself://urls")
	if err != nil {
		t.Fatal(err)
	}
	var data struct{ Argv []string }
	if err := json.Unmarshal([]byte(body), &data); err != nil || len(data.Argv) < 2 || data.Argv[0] != "status" || data.Argv[1] != "urls" {
		t.Errorf("nself://urls: %q %v", body, err)
	}
	// In v1.4 mode the hand handlers keep the four URIs.
	opts.V15 = false
	res3, _ := mcpgen.Resources(opts)
	for _, r := range res3 {
		if r.Resource.URI == "nself://urls" || r.Resource.URI == "nself://config" {
			t.Errorf("v1.4 mode registers %s twice", r.Resource.URI)
		}
	}
}

func TestResourceCommandError(t *testing.T) {
	h := newHarness(t, true, false, false, "STUBCLI_MODE", "text")
	res, _ := mcpgen.Resources(mcpgen.ResourceOptions{Registry: h.reg, Invoke: h.invoke, Schema: schemas.Lookup, V15: true})
	for _, r := range res {
		if r.Resource.URI == "nself://urls" {
			if _, err := readResource(t, r.Handler, r.Resource.URI); err == nil {
				t.Error("a command that returned no envelope was served as a resource")
			}
		}
	}
}

func TestServerInfo(t *testing.T) {
	initialize := func() string {
		s := server.NewMCPServer(mcpgen.ServerName, mcpgen.ServerVersion, mcpgen.ServerOptions()...)
		out := s.HandleMessage(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`))
		b, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	// One identity in both modes and no experimental flag (P7-SURF-23).
	got := initialize()
	if strings.Contains(got, "experimental") || !strings.Contains(got, `"name":"nSelf MCP Server"`) || !strings.Contains(got, `"version":"`+mcpgen.ServerVersion+`"`) {
		t.Errorf("initialize: %s", got)
	}
}
