package commands

// mcp_test.go — unit tests for the nSelf MCP server (CLI-R15).
//
// Purpose: exercise registration, redaction, and the safety gates without
//   requiring a real nSelf project or Docker. Init→build→start→verify via
//   MCP tools is covered by internal/integration/mcp_test.go instead, since
//   that needs a real project directory and (for start) Docker.
// SPORT: CLI-CMD-MCP-001

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TestMCPCmdRegistered verifies that the mcp subcommand is registered on RootCmd.
func TestMCPCmdRegistered(t *testing.T) {
	for _, c := range RootCmd.Commands() {
		if c.Name() == "mcp" {
			return
		}
	}
	t.Fatal("mcp command not registered on RootCmd")
}

// TestMCPCmdFlags verifies the expected flags are declared (no --no-mdns:
// the mDNS feature was removed, see mcp.go's header comment).
func TestMCPCmdFlags(t *testing.T) {
	flags := []string{"transport", "port", "timeout"}
	for _, name := range flags {
		if mcpCmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s not declared on mcp command", name)
		}
	}
	if mcpCmd.Flags().Lookup("no-mdns") != nil {
		t.Error("--no-mdns should not exist: the mDNS feature was removed (CLI-R15)")
	}
}

// TestRegisterMCPTools checks that every tool — the pre-existing Hasura
// tools, ɳSentry, and the new core-surface tools — is registered.
func TestRegisterMCPTools(t *testing.T) {
	s := server.NewMCPServer("test", "0.0.1", server.WithToolCapabilities(true))
	registerMCPTools(s)

	wantTools := []string{
		// Hasura data tools (pre-existing).
		"nself_get_schema", "nself_get_permissions", "nself_run_migration",
		// ɳSentry.
		"sentry_monitors_list", "sentry_monitors_add", "sentry_incidents_list",
		"sentry_incidents_ack", "sentry_status",
		// Core surface (CLI-R15).
		"nself_status", "nself_doctor", "nself_urls", "nself_logs",
		"nself_service_list", "nself_env_list", "nself_config_get",
		"nself_config_show", "nself_config_set", "nself_build", "nself_start",
		"nself_stop", "nself_restart", "nself_db_migrate_status",
		"nself_backup_list", "nself_deploy_status", "nself_plugin_list",
		"nself_plugin_install",
	}

	ctx := context.Background()
	result := s.HandleMessage(ctx, json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal tools/list result: %v", err)
	}

	for _, name := range wantTools {
		if !strings.Contains(string(resultJSON), name) {
			t.Errorf("tool %q not found in tools/list response", name)
		}
	}
}

// TestMCPResourcesRegistered checks that all four resources are registered.
func TestMCPResourcesRegistered(t *testing.T) {
	s := server.NewMCPServer("test", "0.0.1", server.WithResourceCapabilities(true, true))
	registerMCPResources(s)

	ctx := context.Background()
	result := s.HandleMessage(ctx, json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`))
	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal resources/list result: %v", err)
	}
	for _, uri := range []string{"nself://config", "nself://services", "nself://env", "nself://urls"} {
		if !strings.Contains(string(resultJSON), uri) {
			t.Errorf("resource %q not found in resources/list response", uri)
		}
	}
}

// TestMCPPromptsRegistered checks that all three prompts are registered.
func TestMCPPromptsRegistered(t *testing.T) {
	s := server.NewMCPServer("test", "0.0.1", server.WithPromptCapabilities(true))
	registerMCPPrompts(s)

	ctx := context.Background()
	result := s.HandleMessage(ctx, json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"prompts/list","params":{}}`))
	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal prompts/list result: %v", err)
	}
	for _, name := range []string{"diagnose-failure", "add-service", "prepare-deploy"} {
		if !strings.Contains(string(resultJSON), name) {
			t.Errorf("prompt %q not found in prompts/list response", name)
		}
	}
}

// TestMCPHandshake simulates the MCP initialize handshake over the server.
func TestMCPHandshake(t *testing.T) {
	s := server.NewMCPServer("nSelf MCP Server", "2.0.0", server.WithToolCapabilities(true))
	registerMCPTools(s)

	ctx := context.Background()
	initMsg := json.RawMessage(`{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {"name": "test-client", "version": "0.0.1"}
		}
	}`)

	result := s.HandleMessage(ctx, initMsg)
	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal initialize result: %v", err)
	}
	resultStr := string(resultJSON)
	if !strings.Contains(resultStr, "nSelf MCP Server") {
		t.Errorf("server name not found in initialize response: %s", resultStr)
	}
	if !strings.Contains(resultStr, "tools") {
		t.Errorf("tools capability not found in initialize response: %s", resultStr)
	}
}

// TestMCPMigrationConfirmGate verifies the confirmation gate on nself_run_migration.
func TestMCPMigrationConfirmGate(t *testing.T) {
	handler := mcpRunMigrationHandler()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"sql":     "ALTER TABLE np_plugins ADD COLUMN test TEXT;",
		"confirm": false,
	}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	content := extractTextContent(result)
	if !strings.Contains(content, "confirm") && !strings.Contains(content, "safety") {
		t.Errorf("expected rejection message, got: %s", content)
	}
}

// TestMCPMigrationNoSQL verifies that empty SQL is rejected.
func TestMCPMigrationNoSQL(t *testing.T) {
	handler := mcpRunMigrationHandler()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"sql": "", "confirm": true}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	content := extractTextContent(result)
	if !strings.Contains(strings.ToLower(content), "sql") {
		t.Errorf("expected sql-required error, got: %s", content)
	}
}

// TestMCPLogsNoProject verifies the logs tool fails gracefully outside a project.
func TestMCPLogsNoProject(t *testing.T) {
	dir := t.TempDir()
	restore := chdir(t, dir)
	defer restore()

	handler := mcpLogsHandler()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	content := extractTextContent(result)
	if !strings.Contains(strings.ToLower(content), "no nself project") {
		t.Errorf("expected no-project error, got: %s", content)
	}
}

// TestMCPGetSchemaReachesHasura verifies the schema handler calls Hasura correctly.
func TestMCPGetSchemaReachesHasura(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/graphql" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"__schema":{"types":[],"queryType":{"name":"query_root"}}}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	t.Setenv("NSELF_HASURA_GRAPHQL_URL", ts.URL)

	handler := mcpGetSchemaHandler()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	content := extractTextContent(result)
	if !strings.Contains(content, "query_root") {
		t.Errorf("expected schema content, got: %s", content)
	}
}

// TestMCPGetPermissionsReachesHasura verifies the permissions handler calls Hasura metadata API.
func TestMCPGetPermissionsReachesHasura(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/metadata" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"sources":[{"tables":[{"table":{"name":"np_plugins"},"select_permissions":[{"role":"user"}]}]}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	t.Setenv("NSELF_HASURA_GRAPHQL_URL", ts.URL)

	handler := mcpGetPermissionsHandler()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	content := extractTextContent(result)
	if !strings.Contains(content, "np_plugins") {
		t.Errorf("expected permissions content, got: %s", content)
	}
}

// TestMCPResolveHasuraEndpoint checks env-var resolution order.
func TestMCPResolveHasuraEndpoint(t *testing.T) {
	t.Setenv("NSELF_HASURA_GRAPHQL_URL", "")
	t.Setenv("HASURA_GRAPHQL_URL", "")
	got := resolveHasuraEndpoint()
	if got != "http://127.0.0.1:8080" {
		t.Errorf("expected default localhost URL, got %q", got)
	}

	t.Setenv("NSELF_HASURA_GRAPHQL_URL", "http://example.com:8080")
	got = resolveHasuraEndpoint()
	if got != "http://example.com:8080" {
		t.Errorf("expected NSELF_HASURA_GRAPHQL_URL, got %q", got)
	}
}

// TestMCPPortConstant verifies the port constant matches the spec.
func TestMCPPortConstant(t *testing.T) {
	if mcpServerPort != 3825 {
		t.Errorf("mcpServerPort must be 3825 (spec), got %d", mcpServerPort)
	}
}

// TestMCPConfigGetRedactsSecret proves a secret-shaped key comes back masked
// through the MCP tool, never in the clear (ticket requirement).
func TestMCPConfigGetRedactsSecret(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("HASURA_GRAPHQL_ADMIN_SECRET=supersecretvalue\nBASE_DOMAIN=local.nself.org\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restore := chdir(t, dir)
	defer restore()

	handler := mcpConfigGetHandler()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"key": "HASURA_GRAPHQL_ADMIN_SECRET"}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.StructuredContent == nil {
		t.Fatal("expected structured content")
	}
	out, ok := result.StructuredContent.(ConfigGetResult)
	if !ok {
		t.Fatalf("unexpected structured content type: %T", result.StructuredContent)
	}
	if out.Value == "supersecretvalue" {
		t.Fatal("secret value was returned in the clear over MCP")
	}
	if !out.Redacted {
		t.Error("expected Redacted=true for a secret-shaped key")
	}
}

// TestMCPConfigShowRedactsSecrets proves nself_config_show masks every
// secret-shaped key, not just the ones a caller explicitly asks for.
func TestMCPConfigShowRedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("AUTH_JWT_SECRET=topsecretjwt\nPROJECT_NAME=demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restore := chdir(t, dir)
	defer restore()

	handler := mcpConfigShowHandler()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	out, ok := result.StructuredContent.(ConfigShowResult)
	if !ok {
		t.Fatalf("unexpected structured content type: %T", result.StructuredContent)
	}
	if out.Values["AUTH_JWT_SECRET"] == "topsecretjwt" {
		t.Fatal("secret value was returned in the clear over MCP")
	}
	if out.Values["PROJECT_NAME"] != "demo" {
		t.Errorf("non-secret value should pass through unmasked, got %q", out.Values["PROJECT_NAME"])
	}
}

// TestMCPExecSelfNeverUsesBarePath proves mcpExecSelf resolves a real path
// (via os.Executable or the test override), never the bare string "nself".
func TestMCPExecSelfNeverUsesBarePath(t *testing.T) {
	bin, err := selfExecutablePath()
	if err != nil {
		t.Fatalf("selfExecutablePath: %v", err)
	}
	if bin == "nself" {
		t.Fatal("selfExecutablePath must never resolve to the bare string \"nself\"")
	}
	if !filepath.IsAbs(bin) {
		t.Errorf("expected an absolute path, got %q", bin)
	}
}

// TestMCPExecSelfOverride proves the test-only env override works, since the
// integration test depends on it to point re-exec at a built binary instead
// of the go test harness binary.
func TestMCPExecSelfOverride(t *testing.T) {
	t.Setenv(mcpSelfExecOverrideEnv, "/tmp/fake-nself")
	bin, err := selfExecutablePath()
	if err != nil {
		t.Fatalf("selfExecutablePath: %v", err)
	}
	if bin != "/tmp/fake-nself" {
		t.Errorf("expected override path, got %q", bin)
	}
}

// TestMCPBearerMiddlewarePassthrough verifies no-op behavior when
// NSELF_MCP_TOKEN is unset.
func TestMCPBearerMiddlewarePassthrough(t *testing.T) {
	t.Setenv(mcpTokenEnv, "")
	called := false
	h := mcpBearerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called || rec.Code != http.StatusOK {
		t.Errorf("expected pass-through to the wrapped handler, called=%v code=%d", called, rec.Code)
	}
}

// TestMCPBearerMiddlewareRejectsWithoutToken verifies requests are rejected
// when NSELF_MCP_TOKEN is set but no/invalid Authorization header is sent.
func TestMCPBearerMiddlewareRejectsWithoutToken(t *testing.T) {
	t.Setenv(mcpTokenEnv, "secret-token")
	h := mcpBearerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without a token, got %d", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Authorization", "Bearer secret-token")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200 with a matching token, got %d", rec2.Code)
	}
}

// extractTextContent pulls the text from the first text content block.
func extractTextContent(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	for _, c := range result.Content {
		if tc, ok := mcp.AsTextContent(c); ok {
			return tc.Text
		}
	}
	return ""
}

// chdir changes to dir for the duration of the test and returns a restore func.
func chdir(t *testing.T, dir string) func() {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() { _ = os.Chdir(orig) }
}
