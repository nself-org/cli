package mcpgen_test

// Purpose: the failure paths of the resource handlers: no registry, a command
// that errors or returns a non-envelope or an error envelope, and the
// fallback for a legacy URI whose command is not exposed.
// Constraints: Invoke is a fake returning fixed bytes; no network, no child.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/mcpgen"
)

func resourceHandlers(t *testing.T, o mcpgen.ResourceOptions) map[string]server.ResourceHandlerFunc {
	t.Helper()
	res, _ := mcpgen.Resources(o)
	out := map[string]server.ResourceHandlerFunc{}
	for _, r := range res {
		out[r.Resource.URI] = r.Handler
	}
	return out
}

func fixedInvoke(stdout string, err error) mcpgen.Invoker {
	return func(context.Context, string, invoke.Request) (invoke.Result, error) {
		return invoke.Result{Stdout: []byte(stdout)}, err
	}
}

func TestRegistryResourceWithoutRegistry(t *testing.T) {
	h := resourceHandlers(t, mcpgen.ResourceOptions{})
	if _, err := readResource(t, h[mcpgen.URIRegistry], mcpgen.URIRegistry); err == nil || !strings.Contains(err.Error(), "no registry") {
		t.Fatalf("want a no-registry error, got %v", err)
	}
}

func TestCommandResourceFailures(t *testing.T) {
	const uri = "nself://urls"
	cases := []struct {
		name   string
		invoke mcpgen.Invoker
		want   string
	}{
		{"runner error", fixedInvoke("", errors.New("boom")), "nself status urls: boom"},
		{"not json", fixedInvoke("plain text", nil), "did not return an envelope"},
		{"no schema version", fixedInvoke(`{"command":"status urls"}`, nil), "did not return an envelope"},
		{"error envelope", fixedInvoke(`{"schema_version":"1","command":"status urls","error":{"code":"E404","message":"gone"}}`, nil), "E404 gone"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := resourceHandlers(t, mcpgen.ResourceOptions{Invoke: c.invoke, V15: true})
			_, err := readResource(t, h[uri], uri)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestCommandResourceReturnsIndentedData(t *testing.T) {
	const uri = "nself://urls"
	h := resourceHandlers(t, mcpgen.ResourceOptions{V15: true,
		Invoke: fixedInvoke(`{"schema_version":"1","command":"status urls","data":{"hasura":"http://x"}}`, nil)})
	got, err := readResource(t, h[uri], uri)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"hasura\": \"http://x\"\n}\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// With no registry the command is not exposed, so a configured fallback
// serves the legacy URI; with one that exposes it the command handler wins.
func TestLegacyResourceFallback(t *testing.T) {
	const uri = "nself://urls"
	fallback := map[string]server.ResourceHandlerFunc{uri: func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return nil, errors.New("served by the fallback")
	}}
	h := resourceHandlers(t, mcpgen.ResourceOptions{V15: true, Fallback: fallback, Invoke: fixedInvoke("", errors.New("command ran"))})
	if _, err := readResource(t, h[uri], uri); err == nil || !strings.Contains(err.Error(), "fallback") {
		t.Fatalf("no registry: want the fallback, got %v", err)
	}

	reg := newHarness(t, true, false, false).reg
	h = resourceHandlers(t, mcpgen.ResourceOptions{V15: true, Registry: reg, Fallback: fallback, Invoke: fixedInvoke("", errors.New("command ran"))})
	if _, err := readResource(t, h[uri], uri); err == nil || !strings.Contains(err.Error(), "command ran") {
		t.Fatalf("exposed command: want the command handler, got %v", err)
	}
}

func TestLegacyResourceFallbackIgnoredForUnknownCommand(t *testing.T) {
	// "nself://env" maps to a command the fixture does not have: not exposed.
	const uri = "nself://env"
	reg := newHarness(t, true, false, false).reg
	fallback := map[string]server.ResourceHandlerFunc{uri: func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return nil, errors.New("served by the fallback")
	}}
	h := resourceHandlers(t, mcpgen.ResourceOptions{V15: true, Registry: reg, Fallback: fallback})
	if _, err := readResource(t, h[uri], uri); err == nil || !strings.Contains(err.Error(), "fallback") {
		t.Fatalf("want the fallback for an unregistered command, got %v", err)
	}
}

func TestResultRejectsNonEnvelope(t *testing.T) {
	const garbage = "SECRET-TOKEN not an envelope"
	for _, in := range []string{garbage, `{"command":"x"}`, ""} {
		res := mcpgen.Result([]byte(in), nil)
		if !res.IsError || len(res.Content) != 1 {
			t.Fatalf("%q: want one error content, got %+v", in, res)
		}
		body := res.Content[0].(mcp.TextContent).Text
		if !strings.Contains(body, "E422") || !strings.Contains(body, "v1 envelope") {
			t.Errorf("%q: error text %q", in, body)
		}
		if in != "" && strings.Contains(body, in) {
			t.Errorf("the error echoes the untrusted document: %q", body)
		}
	}
}
