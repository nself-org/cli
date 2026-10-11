//go:build integration

// Integration test for TrackPluginTables against real postgres + hasura
// containers (docker run on a private network), through the production seams: the admin secret is
// read from the running Hasura container and table existence from psql.
//
// Run with:
//
//	INTEGRATION=1 go test -tags integration -mod=vendor -count=1 -timeout 300s \
//	    -v ./internal/database/ -run '^TestTrackPluginTablesIntegration$'
package database

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/httptimeout"
)

const trackIntegSecret = "fixture-admin-secret-track-integration"

// trackIntegParse is a minimal export_metadata parser (database cannot import
// appready; the production caller injects appready.TrackedTables).
func trackIntegParse(raw []byte) (map[string]bool, error) {
	var doc struct {
		Sources []struct {
			Tables []struct {
				Table struct{ Schema, Name string } `json:"table"`
			} `json:"tables"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, s := range doc.Sources {
		for _, t := range s.Tables {
			set[t.Table.Schema+"."+t.Table.Name] = true
		}
	}
	return set, nil
}

func trackIntegVersion(t *testing.T, cfg *config.Config) int {
	t.Helper()
	raw, err := postMetadataAdmin(context.Background(), cfg, trackIntegSecret, "export_metadata", struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		ResourceVersion int `json:"resource_version"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.ResourceVersion
}

func TestTrackPluginTablesIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run (requires Docker)")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found in PATH")
	}
	// docker run (not compose) so the test also runs where the compose plugin is absent.
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
			t.Fatalf("docker %s: %v\n%s", strings.Join(args[:2], " "), err, out)
		}
	}
	cleanup := func() {
		for _, c := range []string{"trk78_hasura", "trk78_postgres"} {
			_, _ = exec.Command("docker", "rm", "-f", "-v", c).CombinedOutput()
		}
		_, _ = exec.Command("docker", "network", "rm", "trk78_net").CombinedOutput()
	}
	cleanup()
	t.Cleanup(cleanup)
	run("network", "create", "trk78_net")
	run("run", "-d", "--name", "trk78_postgres", "--network", "trk78_net",
		"--tmpfs", "/var/lib/postgresql/data", "-e", "POSTGRES_PASSWORD=fixture-pg-password", "-e", "POSTGRES_DB=nself", "postgres:16-alpine")
	waitTrackIntegPostgres(t)
	run("run", "-d", "--name", "trk78_hasura", "--network", "trk78_net", "-p", "18078:8080",
		"-e", "HASURA_GRAPHQL_DATABASE_URL=postgres://postgres:fixture-pg-password@trk78_postgres:5432/nself",
		"-e", "HASURA_GRAPHQL_ADMIN_SECRET="+trackIntegSecret,
		"-e", "HASURA_GRAPHQL_ENABLE_CONSOLE=false", "hasura/graphql-engine:v2.44.0")

	cfg := &config.Config{Env: "dev", ProjectName: "trk78"}
	cfg.Hasura.Port = 18078
	ctx := context.Background()
	waitTrackIntegHealthy(t, cfg.Hasura.Port)

	// Plugin tables exist in Postgres; auth_users stands in for hasura-auth's
	// pre-tracked table. ext.np_integ_b proves schema-qualified entries.
	psql := func(sql string) {
		t.Helper()
		if out, err := exec.Command("docker", "exec", "trk78_postgres", "psql", "-U", "postgres", "-d", "nself", "-c", sql).CombinedOutput(); err != nil {
			t.Fatalf("psql %q: %v\n%s", sql, err, out)
		}
	}
	psql("CREATE SCHEMA ext; CREATE TABLE public.np_integ_a (id int primary key); CREATE TABLE ext.np_integ_b (id int primary key); CREATE TABLE public.auth_users (id int primary key)")
	if _, err := postMetadataAdmin(ctx, cfg, trackIntegSecret, "pg_track_table", map[string]interface{}{
		"source": "default", "table": map[string]string{"schema": "public", "name": "auth_users"}}); err != nil {
		t.Fatalf("pre-track auth_users: %v", err)
	}

	plugins := []PluginTables{{Plugin: "integ", Tables: []string{"np_integ_a", "ext.np_integ_b"}}}
	res, err := TrackPluginTables(ctx, cfg, plugins, trackIntegParse)
	if err != nil {
		t.Fatalf("TrackPluginTables: %v", err)
	}
	if strings.Join(res.Tracked, ",") != "ext.np_integ_b,public.np_integ_a" {
		t.Fatalf("tracked = %v", res.Tracked)
	}
	raw, err := postMetadataAdmin(ctx, cfg, trackIntegSecret, "export_metadata", struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	set, err := trackIntegParse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"public.np_integ_a", "ext.np_integ_b", "public.auth_users"} {
		if !set[want] {
			t.Errorf("%s is not tracked after the call (tracked: %v)", want, set)
		}
	}

	// Second call: nothing to do, no bulk request (resource_version would move).
	before := trackIntegVersion(t, cfg)
	res, err = TrackPluginTables(ctx, cfg, plugins, trackIntegParse)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if len(res.Tracked) != 0 || len(res.Already) != 2 {
		t.Fatalf("second call: tracked=%v already=%v", res.Tracked, res.Already)
	}
	if after := trackIntegVersion(t, cfg); after != before {
		t.Fatalf("metadata resource_version moved %d -> %d: the second call wrote", before, after)
	}
}

func waitTrackIntegPostgres(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if exec.Command("docker", "exec", "trk78_postgres", "pg_isready", "-h", "127.0.0.1", "-U", "postgres", "-d", "nself").Run() == nil {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("postgres never became ready")
}

func waitTrackIntegHealthy(t *testing.T, port int) {
	t.Helper()
	url := fmt.Sprintf("http://localhost:%d/healthz", port)
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		if resp, err := httptimeout.Default.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatal("hasura never became healthy")
}
