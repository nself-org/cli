package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installFixture writes <dir>/<name>/plugin.json from the canonical v2 fixture.
// apply "" drops the migrations block; withDir creates migrations/001.sql.
func installFixture(t *testing.T, dir, name, apply string, withDir bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "plugin", "manifestv2", "testdata", "v2", "full.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["name"] = name
	m["schema"] = "np_" + name
	m["tables"] = []string{"np_" + name + "_items"}
	if apply == "" {
		delete(m, "migrations")
	} else {
		m["migrations"] = map[string]any{"dir": "migrations", "apply": apply}
	}
	out, _ := json.Marshal(m)
	pdir := filepath.Join(dir, name)
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pdir, "plugin.json"), out, 0o644); err != nil {
		t.Fatal(err)
	}
	if withDir {
		if err := os.MkdirAll(filepath.Join(pdir, "migrations"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pdir, "migrations", "001_init.sql"), []byte("select 1;"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func noRunning(context.Context) (map[string]runningPlugin, error) { return nil, nil }

func find(results []CheckResult, prefix string) []CheckResult {
	var out []CheckResult
	for _, r := range results {
		if strings.HasPrefix(r.Name, prefix) {
			out = append(out, r)
		}
	}
	return out
}

func TestPluginMigrationsDoctorE118(t *testing.T) {
	dir := t.TempDir()
	installFixture(t, dir, "behind", "boot", true)
	installFixture(t, dir, "caughtup", "boot", true)
	installFixture(t, dir, "nofield", "boot", true)
	installFixture(t, dir, "stopped", "boot", true)

	health := func(body string) string {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		t.Cleanup(s.Close)
		return s.URL
	}
	running := map[string]runningPlugin{
		"behind":   {"behind", health(`{"migrations":{"applied":1,"expected":3}}`)},
		"caughtup": {"caughtup", health(`{"migrations":{"applied":3,"expected":3}}`)},
		"nofield":  {"nofield", health(`{"status":"ok"}`)},
	}
	lookup := func(context.Context) (map[string]runningPlugin, error) { return running, nil }
	res := pluginMigrationChecks(context.Background(), dir, lookup)

	if r := find(res, "PLUGIN-MIGRATIONS-behind"); len(r) != 1 || r[0].Status != "fail" || !strings.HasPrefix(r[0].Message, "E118:") || !strings.Contains(r[0].Message, "1 of 3") {
		t.Errorf("behind: %+v", r)
	}
	if r := find(res, "PLUGIN-MIGRATIONS-caughtup"); len(r) != 1 || r[0].Status != "pass" {
		t.Errorf("caughtup: %+v", r)
	}
	if r := find(res, "PLUGIN-MIGRATIONS-nofield"); len(r) != 1 || r[0].Status != "fail" || !strings.HasPrefix(r[0].Message, "E119:") {
		t.Errorf("nofield: %+v", r)
	}
	if r := find(res, "PLUGIN-MIGRATIONS-stopped"); len(r) != 0 {
		t.Errorf("a plugin that is not running must not be probed: %+v", r)
	}

	// A failing lookup is reported, never turned into "all fine".
	bad := func(context.Context) (map[string]runningPlugin, error) { return nil, errors.New("docker down") }
	res = pluginMigrationChecks(context.Background(), dir, bad)
	if r := find(res, "PLUGIN-MIGRATIONS-running"); len(r) != 1 || r[0].Status != "warn" {
		t.Errorf("lookup failure: %+v", res)
	}
}

func TestPluginMigrationsDoctorE120(t *testing.T) {
	dir := t.TempDir()
	installFixture(t, dir, "noapply", "", true)          // migrations/ but no apply: boot
	installFixture(t, dir, "wrongapply", "manual", true) // not boot (validation may refuse it: then a warn names it)
	installFixture(t, dir, "compliant", "boot", true)
	installFixture(t, dir, "nomigs", "", false) // no migrations at all

	res := pluginMigrationChecks(context.Background(), dir, noRunning)
	r := find(res, "PLUGIN-MIGRATIONS-BOOT-noapply")
	if len(r) != 1 || r[0].Status != "fail" || !strings.HasPrefix(r[0].Message, "E120:") || !strings.Contains(r[0].Message, "noapply") {
		t.Errorf("noapply: %+v", res)
	}
	for _, name := range []string{"compliant", "nomigs"} {
		for _, x := range res {
			if strings.Contains(x.Name, name) || strings.Contains(x.Message, " "+name+" ") {
				t.Errorf("%s must yield nothing, got %+v", name, x)
			}
		}
	}
	// wrongapply: either E120 or an explicit invalid-manifest warn, never silence.
	hit := false
	for _, x := range res {
		if strings.Contains(x.Name, "wrongapply") {
			hit = true
		}
	}
	if !hit {
		t.Errorf("wrongapply produced no finding: %+v", res)
	}

	// Missing plugin dir: nothing to report.
	if got := pluginMigrationChecks(context.Background(), filepath.Join(dir, "absent"), noRunning); len(got) != 0 {
		t.Errorf("absent dir: %+v", got)
	}
}
