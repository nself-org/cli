package commands

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/importer"
	"github.com/spf13/pflag"
)

// firebaseStamp matches the run timestamps in artifact names and headers.
var firebaseStamp = regexp.MustCompile(`20[0-9]{12}|[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z`)

const firebaseAuthFixture = "../../test/fixtures/import/firebase/auth-export.json"

// resetFirebaseFlags restores every migrate firebase flag to its default after a test.
func resetFirebaseFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		migrateFirebaseCmd.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	})
}

func setFirebaseFlags(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		if err := migrateFirebaseCmd.Flags().Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
}

// TestMigrateFirebaseModes: v1.4 keeps the artifact generator byte for byte (golden taken from
// the merge base); v1.5 runs the import pipeline and writes no artifact.
func TestMigrateFirebaseModes(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		resetFirebaseFlags(t)
		if compat.V15() {
			firebaseV15Runs(t)
			return
		}
		firebaseV14Artifacts(t)
	})
}

func firebaseV14Artifacts(t *testing.T) {
	if err := migrateFirebaseCmd.RunE(migrateFirebaseCmd, nil); err == nil || err.Error() != `required flag(s) "export-dir" not set` {
		t.Fatalf("missing --export-dir must keep its error, got %v", err)
	}
	dir := t.TempDir()
	export := filepath.Join(dir, "export")
	if err := os.MkdirAll(filepath.Join(export, "users"), 0o750); err != nil {
		t.Fatal(err)
	}
	docs := `[{"__name__":"users/u1","name":"Ann","age":30,"active":true},{"__name__":"users/u2","name":"Bob","age":41,"active":false}]`
	if err := os.WriteFile(filepath.Join(export, "users", "docs.json"), []byte(docs), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	setFirebaseFlags(t, map[string]string{"export-dir": export, "output-dir": out, "auth-export": firebaseAuthFixture, "project-name": "golden_proj"})
	if _, err := captureHasuraOutput(func() error { return migrateFirebaseCmd.RunE(migrateFirebaseCmd, nil) }); err != nil {
		t.Fatal(err)
	}
	names, _ := filepath.Glob(filepath.Join(out, "*"))
	sort.Strings(names)
	var got strings.Builder
	for _, n := range names {
		b, _ := os.ReadFile(n)
		got.WriteString("=== " + firebaseStamp.ReplaceAllString(filepath.Base(n), "<TS>") + "\n" + firebaseStamp.ReplaceAllString(strings.ReplaceAll(string(b), dir, "<TMP>"), "<TS>") + "\n")
	}
	golden := filepath.Join("testdata", "migrate-firebase-v14.golden")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		_ = os.WriteFile(golden, []byte(got.String()), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != string(want) {
		t.Fatalf("v1.4 artifacts differ from the merge-base golden:\n%s", got.String())
	}
}

func firebaseV15Runs(t *testing.T) {
	old := importRun
	t.Cleanup(func() { importRun = old })
	t.Setenv("FIRESTORE_EMULATOR_HOST", "127.0.0.1:8080")
	t.Setenv("STORAGE_EMULATOR_HOST", "127.0.0.1:9199")
	called := false
	importRun = func(_ context.Context, _ *config.Config, s importer.Source, in importer.Input, opts importer.RunOptions) (importer.Report, error) {
		called = true
		if s.Name() != "firebase" || !opts.Plan || in.Values["project"] != "demo-nself" || in.Values["auth-export"] != firebaseAuthFixture || in.Values["bucket"] != "demo-bucket" {
			t.Fatalf("wrong inputs: %s %+v %+v", s.Name(), in.Values, opts)
		}
		return importer.Report{Source: s.Name(), Result: "pass", Steps: []string{"extract snapshot"}}, nil
	}
	migrateFirebaseCmd.SetContext(context.Background())
	setFirebaseFlags(t, map[string]string{"project": "demo-nself", "auth-export": firebaseAuthFixture, "bucket": "demo-bucket", "plan": "true", "json": "true"})
	out, err := captureHasuraOutput(func() error { return migrateFirebaseCmd.RunE(migrateFirebaseCmd, nil) })
	if err != nil || !called {
		t.Fatalf("pipeline not run: %v called=%v", err, called)
	}
	var doc struct {
		Command string          `json:"command"`
		Data    importer.Report `json:"data"`
	}
	if json.Unmarshal([]byte(out), &doc) != nil || doc.Command != "db import firebase" || doc.Data.Result != "pass" {
		t.Fatalf("invalid report JSON: %s", out)
	}
	if _, e := os.Stat(filepath.Join(filepath.Dir(firebaseAuthFixture), "nself-migration")); e == nil {
		t.Fatal("v1.5 wrote artifacts")
	}
}

func TestMigrateFirebaseV15RefusesBadFlags(t *testing.T) {
	compattest.Set(t, true)
	resetFirebaseFlags(t)
	t.Setenv("FIRESTORE_EMULATOR_HOST", "")
	migrateFirebaseCmd.SetContext(context.Background())
	// No --project: the command names the flag before the importer runs.
	setFirebaseFlags(t, map[string]string{"auth-export": firebaseAuthFixture})
	old := importRun
	t.Cleanup(func() { importRun = old })
	importRun = func(context.Context, *config.Config, importer.Source, importer.Input, importer.RunOptions) (importer.Report, error) {
		t.Fatal("importer ran with invalid flags")
		return importer.Report{}, nil
	}
	if err := migrateFirebaseCmd.RunE(migrateFirebaseCmd, nil); err == nil || !strings.Contains(err.Error(), "--project") {
		t.Fatalf("err %v", err)
	}
}

func TestFirebaseImportRegistered(t *testing.T) {
	if _, ok := jsonDataTypes["db import firebase"].(importer.Report); !ok {
		t.Fatalf("db import firebase report type = %T", jsonDataTypes["db import firebase"])
	}
}
