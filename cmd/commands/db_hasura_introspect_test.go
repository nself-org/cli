package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nself-org/cli/internal/hasura"
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
