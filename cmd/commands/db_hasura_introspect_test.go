package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/hasura"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

func TestHasuraIntrospectCommands(t *testing.T) {
	secret := "fixture-secret-32"
	schema, err := os.ReadFile("../../internal/hasura/testdata/introspect/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	permissions, err := os.ReadFile("../../internal/hasura/testdata/introspect/permissions.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Hasura-Admin-Secret") != secret {
			t.Error("admin secret not sent")
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/v1/graphql":
			_, _ = w.Write(schema)
		case "/v1/metadata":
			_, _ = w.Write(permissions)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	t.Setenv("NSELF_HASURA_GRAPHQL_URL", srv.URL)
	t.Setenv("HASURA_GRAPHQL_ADMIN_SECRET", secret)
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		run  func(*cobra.Command, []string) error
	}{
		{"schema", runDBHasuraSchema}, {"permissions", runDBHasuraPermissions},
	} {
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		cmd.Flags().Bool("json", true, "")
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		out, runErr := captureHasuraOutput(func() error { return tc.run(cmd, nil) })
		if err := os.Chdir(old); err != nil {
			t.Fatal(err)
		}
		if runErr != nil {
			t.Fatalf("%s: %v", tc.name, runErr)
		}
		if strings.Contains(out, secret) {
			t.Fatalf("%s leaked secret", tc.name)
		}
		var doc struct {
			SchemaVersion string          `json:"schema_version"`
			Command       string          `json:"command"`
			Data          json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("%s envelope: %v: %q", tc.name, err, out)
		}
		if doc.Command != "db hasura "+tc.name || doc.SchemaVersion != "1" {
			t.Fatalf("%s envelope: %+v", tc.name, doc)
		}
		if err := contractValidate(contractResolve(t, schemaFileFor(doc.Command)), doc.Data); err != nil {
			t.Fatalf("%s schema: %v", tc.name, err)
		}
	}
	req := mcp.CallToolRequest{}
	legacySchema, err := mcpGetSchemaHandler()(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if extractTextContent(legacySchema) != hasura.CompactSchema(schema) {
		t.Fatal("legacy schema bytes changed")
	}
	legacyPerms, err := mcpGetPermissionsHandler()(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	legacyBytes, err := json.Marshal(legacyPerms.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(legacyBytes, []byte(`"table":"np_users"`)) {
		t.Fatalf("legacy permissions changed: %s", legacyBytes)
	}
	for _, name := range []string{"db hasura schema", "db hasura permissions"} {
		if _, err := os.Stat(filepath.Join("testdata/json", strings.ReplaceAll(name, " ", "-")+".golden.json")); err != nil {
			t.Fatal(err)
		}
	}
	reg, err := BuildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"nself db hasura schema", "nself db hasura permissions"} {
		found := false
		for _, c := range reg.Commands {
			if c.Path == path {
				found = true
				if c.SideEffect != "read" || c.Output != "document" || c.JSON != "envelope" {
					t.Fatalf("bad registry row: %+v", c)
				}
			}
		}
		if !found {
			t.Fatalf("missing registry path %s", path)
		}
	}
}

func TestHasuraPermissionsInvalidExportEnvelope(t *testing.T) {
	defer chdir(t, t.TempDir())()
	for _, body := range []string{`{"error":"metadata export disabled"}`, `{}`} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			t.Setenv("NSELF_HASURA_GRAPHQL_URL", srv.URL)
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			cmd.Flags().Bool("json", true, "")
			err := runDBHasuraPermissions(cmd, nil)
			if err == nil || errs.ExitCodeFor(err) == 0 {
				t.Fatalf("expected non-zero command error, got %v", err)
			}
			var stdout bytes.Buffer
			if emitErr := output.EmitError(output.Writer{Out: &stdout}, "db hasura permissions", err); emitErr != nil {
				t.Fatal(emitErr)
			}
			var envelope struct {
				Error struct {
					Code     string `json:"code"`
					ExitCode int    `json:"exit_code"`
				} `json:"error"`
			}
			if json.Unmarshal(stdout.Bytes(), &envelope) != nil || envelope.Error.Code != "E200" || envelope.Error.ExitCode == 0 {
				t.Fatalf("expected non-zero E200 error envelope: %s", stdout.String())
			}
		})
	}
}

func TestHasuraPermissionsInvalidExportCLI(t *testing.T) {
	if len(os.Args) > 1 && os.Args[1] == "-test.run=^TestHasuraPermissionsInvalidExportCLI$/child" {
		os.Args = []string{"nself", "db", "hasura", "permissions", "--json"}
		err := Execute()
		if err == nil {
			os.Exit(0)
		}
		_ = output.EmitError(output.Default(), "db hasura permissions", err)
		os.Exit(errs.ExitCodeFor(err))
	}
	for _, body := range []string{`{"error":"metadata export disabled"}`, `{}`} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			child := exec.Command(os.Args[0], "-test.run=^TestHasuraPermissionsInvalidExportCLI$/child")
			child.Dir = t.TempDir()
			child.Env = append(os.Environ(), "NSELF_V15=1", "NSELF_HASURA_GRAPHQL_URL="+srv.URL)
			stdout, err := child.Output()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() == 0 {
				t.Fatalf("expected non-zero CLI exit, got %v, stdout %s", err, stdout)
			}
			var envelope struct {
				Error struct {
					Code     string `json:"code"`
					ExitCode int    `json:"exit_code"`
				} `json:"error"`
			}
			if json.Unmarshal(stdout, &envelope) != nil || envelope.Error.Code != "E200" || envelope.Error.ExitCode != exit.ExitCode() {
				t.Fatalf("expected CLI E200 envelope with exit %d, got %s; stderr %s", exit.ExitCode(), stdout, exit.Stderr)
			}
		})
	}
}

func TestMCPLegacyHasuraErrorText(t *testing.T) {
	const secret = "echoed-admin-secret"
	for _, tc := range []struct {
		status     int
		body, want string
	}{
		{503, "unavailable", "HTTP 503: unavailable"},
		{401, "denied: " + secret, "HTTP 401: denied: [REDACTED]"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		t.Setenv("NSELF_HASURA_GRAPHQL_URL", srv.URL)
		t.Setenv("HASURA_GRAPHQL_ADMIN_SECRET", secret)
		for _, tool := range []struct {
			name string
			run  func() (string, error)
		}{
			{"Schema introspection error", func() (string, error) {
				r, e := mcpGetSchemaHandler()(context.Background(), mcp.CallToolRequest{})
				return extractTextContent(r), e
			}},
			{"Permissions snapshot error", func() (string, error) {
				r, e := mcpGetPermissionsHandler()(context.Background(), mcp.CallToolRequest{})
				return extractTextContent(r), e
			}},
		} {
			got, err := tool.run()
			if err != nil || got != tool.name+": "+tc.want || strings.Contains(got, secret) {
				t.Fatalf("legacy error text: got %q, err %v", got, err)
			}
		}
		srv.Close()
	}
}

func captureHasuraOutput(run func() error) (string, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	old := os.Stdout
	os.Stdout = w
	runErr := run()
	os.Stdout = old
	_ = w.Close()
	out, readErr := io.ReadAll(r)
	_ = r.Close()
	if runErr != nil {
		return string(out), runErr
	}
	return string(out), readErr
}
