package hasura

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func fixtureServer(t *testing.T, route, fixture, secret string) *httptest.Server {
	t.Helper()
	contents, err := os.ReadFile("testdata/introspect/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != route || r.Method != http.MethodPost || r.Header.Get("X-Hasura-Admin-Secret") != secret {
			t.Errorf("request method/path/auth mismatch")
			w.WriteHeader(403)
			return
		}
		var payload map[string]interface{}
		if json.NewDecoder(r.Body).Decode(&payload) != nil {
			t.Error("invalid JSON request")
			w.WriteHeader(400)
			return
		}
		if route == "/v1/graphql" {
			query, _ := payload["query"].(string)
			if !strings.Contains(query, "__schema") {
				t.Error("no introspection query")
			}
		} else if payload["type"] != "export_metadata" {
			t.Error("not export_metadata")
		}
		_, _ = w.Write(contents)
	}))
}

func TestIntrospectSchema(t *testing.T) {
	const secret = "fixture-admin-secret"
	srv := fixtureServer(t, "/v1/graphql", "schema.json", secret)
	defer srv.Close()
	s, err := Schema(context.Background(), srv.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Types) != 1 || s.Types[0].Name != "np_users" || s.QueryType.Name != "query_root" {
		t.Fatalf("schema: %+v", s)
	}
	if !strings.Contains(CompactSchema(s.Raw), `"np_users"`) {
		t.Fatal("raw legacy response missing")
	}
	if strings.Contains(string(mustJSON(t, s)), secret) {
		t.Fatal("secret leaked")
	}
}

func TestIntrospectPermissions(t *testing.T) {
	const secret = "fixture-admin-secret"
	srv := fixtureServer(t, "/v1/metadata", "permissions.json", secret)
	defer srv.Close()
	s, err := Permissions(context.Background(), srv.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Valid || len(s.Tables) != 1 || s.Tables[0].Table != "np_users" {
		t.Fatalf("permissions: %+v", s)
	}
	if _, ok := s.Tables[0].Permissions["select_permissions"]; !ok {
		t.Fatal("select permission missing")
	}
	if strings.Contains(string(mustJSON(t, s)), secret) {
		t.Fatal("secret leaked")
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(401); _, _ = w.Write([]byte(secret)) }))
	defer bad.Close()
	_, err = Permissions(context.Background(), bad.URL, secret)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unredacted error: %v", err)
	}
}

func TestIntrospectEndpointAndFailures(t *testing.T) {
	t.Setenv("NSELF_HASURA_GRAPHQL_URL", "")
	t.Setenv("HASURA_GRAPHQL_URL", "")
	if got := Endpoint(); got != "http://127.0.0.1:8080" {
		t.Fatalf("default endpoint: %s", got)
	}
	t.Setenv("HASURA_GRAPHQL_URL", "http://fallback")
	if got := Endpoint(); got != "http://fallback" {
		t.Fatalf("fallback endpoint: %s", got)
	}
	t.Setenv("NSELF_HASURA_GRAPHQL_URL", "http://preferred")
	if got := Endpoint(); got != "http://preferred" {
		t.Fatalf("preferred endpoint: %s", got)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer bad.Close()
	s, err := Schema(context.Background(), bad.URL, "")
	if err != nil || s.Valid || string(s.Raw) != "not-json" {
		t.Fatalf("malformed schema: valid=%v err=%v", s.Valid, err)
	}
	p, err := Permissions(context.Background(), bad.URL, "")
	if err != nil || p.Valid || string(p.Raw) != "not-json" {
		t.Fatalf("malformed permissions: valid=%v err=%v", p.Valid, err)
	}
	if got := CompactSchema([]byte("not-json")); got != "not-json" {
		t.Fatalf("malformed legacy text: %q", got)
	}

	if _, err := Schema(context.Background(), ":invalid", ""); err == nil {
		t.Fatal("schema accepted invalid URL")
	}
	if _, err := Permissions(context.Background(), ":invalid", ""); err == nil {
		t.Fatal("permissions accepted invalid URL")
	}
	if _, err := postJSON(context.Background(), bad.URL, "", make(chan int)); err == nil {
		t.Fatal("unsupported request payload accepted")
	}
}

func mustJSON(t *testing.T, value interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
