package commands

// Purpose: runs the real bodies of four typed data rows (db migrate status,
// db drift scan, db seed list, init template list) against fakes and checks
// the envelope data they print (P7-SURF-14 fix round 1, finding S3), plus the
// failed-registry case of init template list (S2).
// Inputs: a docker stub on PATH for the SQL queries, an httptest registry,
// temp project and seed directories. Outputs: pass or fail.
// Constraints: no network beyond loopback; POSIX shell stub.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

// fakeDockerSQL answers the two queries the typed rows read and nothing else.
const fakeDockerSQL = `#!/bin/sh
for a; do last="$a"; done
case "$last" in
  *information_schema.columns*) printf 'np_demo|np_orders|id|uuid\nnp_demo|np_orders|created_at|timestamptz\n' ;;
  *"FROM np_common.schema_versions ORDER BY applied_at"*) printf '20260101_init.sql|2026-01-02T03:04:05Z\n' ;;
esac
exit 0
`

// runTypedBody runs cmd.RunE the way the CLI does (human text to stderr, the
// document to the real stdout) in v1.5 --json mode and returns the document.
func runTypedBody(t *testing.T, cmd *cobra.Command, flags map[string]string) (string, error) {
	t.Helper()
	flags["json"] = "true"
	cmd.SetContext(context.Background())
	return jsonV15(t, cmd, flags, func() error {
		restore := output.IsolateStdout()
		defer restore()
		return cmd.RunE(cmd, nil)
	})
}

func dataProject(t *testing.T) string {
	t.Helper()
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte("PROJECT_NAME=proj\nENV=dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)
	t.Setenv("NSELF_BACKUP_HEARTBEAT_REMOTE", "")
	return proj
}

func dataOf(t *testing.T, out, command string, into any) {
	t.Helper()
	doc := oneEnvelope(t, out, command)
	if err := json.Unmarshal(doc["data"], into); err != nil {
		t.Fatalf("%s data: %v\n%s", command, err, out)
	}
}

func TestDataBodiesTypedRows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the docker double is a POSIX shell script")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(fakeDockerSQL), 0o755); err != nil {
		t.Fatal(err)
	}
	prependPath(t, bin)

	t.Run("db migrate status", func(t *testing.T) {
		proj := dataProject(t)
		mig := filepath.Join(proj, "migs")
		if err := os.MkdirAll(mig, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, n := range []string{"20260101_init.sql", "20260102_next.sql"} {
			if err := os.WriteFile(filepath.Join(mig, n), []byte("SELECT 1;\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		out, err := runTypedBody(t, dbMigrateStatusCmd, map[string]string{"migration-dir": mig})
		if err != nil {
			t.Fatal(err)
		}
		var d dbMigrateStatusData
		dataOf(t, out, "db migrate status", &d)
		if len(d.Migrations) != 2 || d.Migrations[0].Name != "20260101_init.sql" || d.Migrations[0].Status != "applied" ||
			d.Migrations[0].AppliedAt == nil || *d.Migrations[0].AppliedAt != "2026-01-02T03:04:05Z" ||
			d.Migrations[1].Status != "pending" || d.Migrations[1].AppliedAt != nil {
			t.Fatalf("migrations: %+v\n%s", d.Migrations, out)
		}
	})

	t.Run("db drift scan", func(t *testing.T) {
		dataProject(t)
		out, err := runTypedBody(t, dbDriftScanCmd, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		var d dbDriftScanData
		dataOf(t, out, "db drift scan", &d)
		if d.TotalTables != 1 || len(d.Tables) != 1 || d.Tables[0].Schema != "np_demo" || d.Tables[0].Table != "np_orders" ||
			len(d.Tables[0].Present) != 2 || len(d.Tables[0].Missing) == 0 || d.Tables[0].Score == 0 || d.DriftedTables != 1 {
			t.Fatalf("drift: %+v\n%s", d, out)
		}
	})

	t.Run("db seed list", func(t *testing.T) {
		proj := dataProject(t)
		dir := filepath.Join(proj, "db", "seeds", "dev")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "001_users.sql"), []byte("INSERT INTO t VALUES (1);\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := runTypedBody(t, dbSeedListCmd, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		var d dbSeedListData
		dataOf(t, out, "db seed list", &d)
		if len(d.Seeds) != 1 || d.Seeds[0].Env != "dev" || !strings.Contains(d.Seeds[0].Name, "001_users") || d.Seeds[0].Type != "standard" {
			t.Fatalf("seeds: %+v\n%s", d.Seeds, out)
		}
	})
}

// TestTemplateListRegistryState: a reachable registry fills community and
// reports community_available true; a failed fetch must say so instead of
// printing an empty list that reads as "none exist" (S2).
func TestTemplateListRegistryState(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		available bool
		community int
	}{{"reachable", http.StatusOK, true, 1}, {"unreachable", http.StatusInternalServerError, false, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.status != http.StatusOK {
					http.Error(w, "down", tc.status)
					return
				}
				_, _ = w.Write([]byte(`{"templates":[{"slug":"blog","displayName":"Blog","requiredPlugins":[]}]}`))
			}))
			defer srv.Close()
			t.Setenv("NSELF_TEMPLATE_REGISTRY_URL", srv.URL)
			dataProject(t)
			t.Cleanup(applyCanon(RootCmd, true)) // the row is registered at its v1.5 path, init template list
			out, err := runTypedBody(t, templateListCmd, map[string]string{})
			if err != nil {
				t.Fatal(err)
			}
			var d templateListData
			dataOf(t, out, "init template list", &d)
			if d.CommunityAvailable != tc.available || len(d.Community) != tc.community || len(d.Bundled) == 0 {
				t.Fatalf("available=%v community=%d bundled=%d, want available=%v community=%d\n%s",
					d.CommunityAvailable, len(d.Community), len(d.Bundled), tc.available, tc.community, out)
			}
		})
	}
}

// TestDefaultDataRequiresSetData: a typed row whose body never hands over its
// data fails loudly instead of printing a zero document (N3).
func TestDefaultDataRequiresSetData(t *testing.T) {
	_, err := jsonV15(t, dbMigrateStatusCmd, map[string]string{"json": "true"}, func() error {
		return emitDataEnvelope(dbMigrateStatusCmd, nil)
	})
	if err == nil || !strings.Contains(err.Error(), "internal:") {
		t.Fatalf("a typed row without setData must error, got %v", err)
	}
}

// TestResetChecksumConfirmBlock: db reset-checksum refuses without
// --i-know-what-im-doing, so the registry exposes it with that flag as its
// confirm block instead of marking it cli-only (S1).
func TestResetChecksumConfirmBlock(t *testing.T) {
	compattest.Set(t, true)
	resetRegistryCache()
	t.Cleanup(resetRegistryCache)
	reg, err := buildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := reg.Lookup("db reset-checksum")
	if !ok {
		t.Fatal("db reset-checksum is not registered")
	}
	if c.Surface == "cli-only" || c.Confirm == nil || strings.Join(c.Confirm.Flags, ",") != "i-know-what-im-doing" {
		t.Fatalf("surface=%q confirm=%+v, want an exposed row confirmed by --i-know-what-im-doing", c.Surface, c.Confirm)
	}
}
