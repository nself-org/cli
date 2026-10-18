package clone

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestAll_ReturnsSevenTemplates verifies the six app clones plus multi-app are bundled.
func TestAll_ReturnsSevenTemplates(t *testing.T) {
	templates := All()
	if len(templates) != 7 {
		t.Errorf("expected 7 clone templates, got %d: %v", len(templates), templates)
	}
}

// TestAll_AllNonEmpty verifies no template name is empty.
func TestAll_AllNonEmpty(t *testing.T) {
	for _, name := range All() {
		if name == "" {
			t.Error("All() contains empty template name")
		}
	}
}

// TestIsCloneTemplate_Known verifies all documented templates are recognised.
func TestIsCloneTemplate_Known(t *testing.T) {
	known := []string{
		"airbnb-clone",
		"discord-clone",
		"notion-clone",
		"slack-clone",
		"substack-clone",
		"zoom-clone",
		"multi-app",
	}
	for _, name := range known {
		t.Run(name, func(t *testing.T) {
			if !IsCloneTemplate(name) {
				t.Errorf("IsCloneTemplate(%q) = false, want true", name)
			}
		})
	}
}

// TestIsCloneTemplate_Unknown verifies unknown names return false.
func TestIsCloneTemplate_Unknown(t *testing.T) {
	unknown := []string{"", "myapp", "twitter-clone", "custom-template"}
	for _, name := range unknown {
		t.Run(name, func(t *testing.T) {
			if IsCloneTemplate(name) {
				t.Errorf("IsCloneTemplate(%q) = true, want false", name)
			}
		})
	}
}

// TestGetManifest_AllBundled verifies that every bundled template has a
// readable, non-empty manifest.
func TestGetManifest_AllBundled(t *testing.T) {
	for _, name := range All() {
		t.Run(name, func(t *testing.T) {
			m, err := GetManifest(name)
			if err != nil {
				t.Fatalf("GetManifest(%q): %v", name, err)
			}
			if m.Name == "" {
				t.Errorf("manifest Name is empty for %q", name)
			}
			if m.Version == "" {
				t.Errorf("manifest Version is empty for %q", name)
			}
		})
	}
}

// TestGetManifest_Unknown verifies an unknown template returns an error.
func TestGetManifest_Unknown(t *testing.T) {
	_, err := GetManifest("does-not-exist")
	if err == nil {
		t.Error("GetManifest for unknown template should return error, got nil")
	}
}

// TestScaffold_CreatesFiles verifies that Scaffold creates files in the output
// directory.
func TestScaffold_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	templateName := All()[0] // use first available template

	opts := ScaffoldOptions{TargetDir: dir}
	written, err := Scaffold(templateName, opts)
	if err != nil {
		t.Fatalf("Scaffold(%q): %v", templateName, err)
	}
	if len(written) == 0 {
		t.Error("Scaffold wrote no files")
	}

	// The output dir should contain at least one file.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("Scaffold created no files in output dir")
	}
	_ = filepath.Join(dir, templateName) // ensure output dir structure is understood
}

// TestScaffold_DryRun verifies that DryRun returns file list without writing.
func TestScaffold_DryRun(t *testing.T) {
	dir := t.TempDir()
	templateName := All()[0]

	opts := ScaffoldOptions{TargetDir: dir, DryRun: true}
	written, err := Scaffold(templateName, opts)
	if err != nil {
		t.Fatalf("Scaffold DryRun: %v", err)
	}
	if len(written) == 0 {
		t.Error("Scaffold DryRun should return file list")
	}

	// In dry run, no files should be written.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("DryRun wrote files: %v", entries)
	}
}

// TestScaffold_InvalidTemplate verifies that an unknown template returns an error.
func TestScaffold_InvalidTemplate(t *testing.T) {
	dir := t.TempDir()
	opts := ScaffoldOptions{TargetDir: dir}
	_, err := Scaffold("does-not-exist", opts)
	if err == nil {
		t.Error("Scaffold with invalid template should return error, got nil")
	}
}

// Multi-app isolation template (P7-ADOPT-04) and the optional manifest next_steps field.
// The permission filter is the isolation wall: these tests pin it exactly so an
// edit that weakens it fails here, before the Linux e2e.

// membershipFilter is the review-F10 filter: the token's app id AND a real
// app_members row for this user in that app.
const membershipFilter = `{"_and":[{"source_account_id":{"_eq":"X-Hasura-Source-Account-Id"}},` +
	`{"_exists":{"_table":{"schema":"public","name":"app_members"},` +
	`"_where":{"user_id":{"_eq":"X-Hasura-User-Id"},"source_account_id":{"_eq":"X-Hasura-Source-Account-Id"}}}}]}`

type maPerm struct {
	Role       string                     `json:"role"`
	Permission map[string]json.RawMessage `json:"permission"`
}

func maDecode(t *testing.T, raw json.RawMessage) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	return v
}

// maTables loads the shipped hasura/metadata.json as table name -> fields.
func maTables(t *testing.T) map[string]map[string]json.RawMessage {
	t.Helper()
	data, err := templateFS.ReadFile("multi-app/hasura/metadata.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Sources []struct {
			Tables []map[string]json.RawMessage `json:"tables"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]json.RawMessage{}
	for _, tb := range doc.Sources[0].Tables {
		var ref struct{ Name string }
		if err := json.Unmarshal(tb["table"], &ref); err != nil {
			t.Fatal(err)
		}
		out[ref.Name] = tb
	}
	return out
}

// maUserPerm returns the single role-user permission of one operation.
func maUserPerm(t *testing.T, tb map[string]json.RawMessage, op string) map[string]json.RawMessage {
	t.Helper()
	var perms []maPerm
	if err := json.Unmarshal(tb[op+"_permissions"], &perms); err != nil || len(perms) != 1 || perms[0].Role != "user" {
		t.Fatalf("%s permissions = %s, want exactly one for role user", op, tb[op+"_permissions"])
	}
	return perms[0].Permission
}

// TestMultiAppFiles pins the file list and the manifest (golden).
func TestMultiAppFiles(t *testing.T) {
	got, err := Scaffold("multi-app", ScaffoldOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{".env.example": true, "README.md": true, "hasura/metadata.json": true,
		"hasura/metadata/databases/default/tables/tables.yaml": true,
		"manifest.json": true, "migrations/001_multi_app.sql": true, "nself.yml": true}
	if len(got) != len(want) {
		t.Fatalf("files = %v, want %d files", got, len(want))
	}
	for _, f := range got {
		if !want[f] {
			t.Errorf("unexpected file %q", f)
		}
	}
	if HasFlutter("multi-app") {
		t.Error("multi-app ships a flutter/ folder")
	}
	m, err := GetManifest("multi-app")
	if err != nil {
		t.Fatal(err)
	}
	steps := []string{"nself build", "nself start", "nself db migrate up --migration-dir migrations", "nself db reconcile --apply"}
	if !reflect.DeepEqual(m.NextSteps, steps) || len(m.RequiredPlugins) != 0 || m.Version != "1.0.0" {
		t.Errorf("manifest = %+v, want next_steps %v, version 1.0.0 and no required plugins", m, steps)
	}
}

// TestSixClonesKeepOldShape: the six app clones declare no next_steps and keep
// their flutter/ starter, so their printed output is unchanged.
func TestSixClonesKeepOldShape(t *testing.T) {
	for _, name := range All() {
		if name == "multi-app" {
			continue
		}
		m, err := GetManifest(name)
		if err != nil {
			t.Fatal(err)
		}
		if m.NextSteps != nil {
			t.Errorf("%s declares next_steps %v", name, m.NextSteps)
		}
		if !HasFlutter(name) {
			t.Errorf("%s lost its flutter/ starter", name)
		}
	}
}

// TestMultiAppSecurityFilters pins the isolation wall on notes AND projects:
// every user operation carries the exact membership filter, update also as its
// post-update check, and app_members has no permissions at all.
func TestMultiAppSecurityFilters(t *testing.T) {
	tables := maTables(t)
	want := maDecode(t, json.RawMessage(membershipFilter))
	for k := range tables["app_members"] {
		if k != "table" {
			t.Errorf("app_members declares %q: it must have no user permissions", k)
		}
	}
	for _, name := range []string{"notes", "projects"} {
		tb := tables[name]
		for op, field := range map[string]string{"select": "filter", "insert": "check", "update": "filter", "delete": "filter"} {
			p := maUserPerm(t, tb, op)
			if got := maDecode(t, p[field]); !reflect.DeepEqual(got, want) {
				t.Errorf("%s %s %s = %v, want the membership filter", name, op, field, got)
			}
		}
		if got := maDecode(t, maUserPerm(t, tb, "update")["check"]); !reflect.DeepEqual(got, want) {
			t.Errorf("%s update check = %v, want the membership filter", name, got)
		}
	}
}

// TestMultiAppNoSpoofing: inserts take source_account_id from the token and the
// client can never send it; updates cannot move a row to another app.
func TestMultiAppNoSpoofing(t *testing.T) {
	cols := map[string][]string{"notes": {"title"}, "projects": {"title", "name"}}
	for name, want := range cols {
		tb := maTables(t)[name]
		ins := maUserPerm(t, tb, "insert")
		var set map[string]string
		if err := json.Unmarshal(ins["set"], &set); err != nil || !reflect.DeepEqual(set, map[string]string{"source_account_id": "X-Hasura-Source-Account-Id"}) {
			t.Errorf("%s insert presets = %s", name, ins["set"])
		}
		for _, op := range []string{"insert", "update"} {
			var got []string
			if err := json.Unmarshal(maUserPerm(t, tb, op)["columns"], &got); err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("%s %s columns = %v, want %v (no source_account_id)", name, op, got, want)
			}
		}
	}
}

// TestMultiAppMetadataInSync: the YAML that `nself db reconcile` reads and the
// required hasura/metadata.json describe the same tables.
func TestMultiAppMetadataInSync(t *testing.T) {
	raw, err := templateFS.ReadFile("multi-app/hasura/metadata/databases/default/tables/tables.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var fromYAML interface{}
	if err := yaml.Unmarshal(raw, &fromYAML); err != nil {
		t.Fatal(err)
	}
	jsonTables := []map[string]json.RawMessage{}
	for _, name := range []string{"notes", "projects", "app_members"} {
		jsonTables = append(jsonTables, maTables(t)[name])
	}
	b, _ := json.Marshal(jsonTables)
	if !reflect.DeepEqual(fromYAML, maDecode(t, b)) {
		t.Errorf("tables.yaml and metadata.json differ:\nyaml: %v\njson: %s", fromYAML, b)
	}
}

// TestMultiAppMigration pins the schema exactly and Constitution 5.5.
func TestMultiAppMigration(t *testing.T) {
	raw, err := templateFS.ReadFile("multi-app/migrations/001_multi_app.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, must := range []string{
		"set lock_timeout = '5s'", "set statement_timeout = '60s'",
		"create table if not exists public.notes (\n    id uuid primary key default gen_random_uuid(),\n    source_account_id text not null default 'primary',\n    title text not null,\n    created_at timestamptz default now()\n);",
		"create table if not exists public.projects (\n    id uuid primary key default gen_random_uuid(),\n    source_account_id text not null default 'primary',\n    title text not null,\n    name text not null,\n    created_at timestamptz default now()\n);",
		"create table if not exists public.app_members (\n    user_id uuid primary key references auth.users(id) on delete cascade,\n    source_account_id text not null\n);",
		"create index if not exists notes_source_account_id_idx on public.notes (source_account_id)",
		"create index if not exists projects_source_account_id_idx on public.projects (source_account_id)",
	} {
		if !strings.Contains(sql, must) {
			t.Errorf("migration lacks %q", must)
		}
	}
	for _, bad := range []string{"drop ", "truncate", "delete from", "tenant_id", "author_id", "body"} {
		if strings.Contains(sql, bad) {
			t.Errorf("migration contains %q", bad)
		}
	}
}

// TestMultiAppTemplateHygiene: source_account_id only, no tenant wall, no
// legacy UI stacks, no licensed plugins, no replace_metadata path.
func TestMultiAppTemplateHygiene(t *testing.T) {
	files, _ := Scaffold("multi-app", ScaffoldOptions{DryRun: true})
	for _, f := range files {
		data, err := templateFS.ReadFile("multi-app/" + f)
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(data))
		for _, bad := range []string{"tenant_id", "flutter", "next.js", "nextjs", "replace_metadata", "metadata apply --file"} {
			if strings.Contains(lower, bad) {
				t.Errorf("%s contains %q", f, bad)
			}
		}
	}
	env, _ := templateFS.ReadFile("multi-app/.env.example")
	for _, want := range []string{"PROJECT_NAME=", "BASE_DOMAIN=", `AUTH_JWT_CUSTOM_CLAIMS={"source-account-id":"metadata.source_account_id"}`} {
		if !strings.Contains(string(env), want) {
			t.Errorf(".env.example lacks %q", want)
		}
	}
	// AUTH_JWT_CUSTOM_CLAIMS is a known variable (D-0918): the template must not
	// carry an ENV_ALLOWLIST workaround for it.
	if strings.Contains(string(env), "ENV_ALLOWLIST") {
		t.Errorf(".env.example still carries the ENV_ALLOWLIST workaround")
	}
}

// TestNoTemplateShipsSecrets: no bundled template carries a secret-looking value.
func TestNoTemplateShipsSecrets(t *testing.T) {
	secret := regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY|sk_(live|test)_[A-Za-z0-9]{8,}|AKIA[0-9A-Z]{16}|eyJ[A-Za-z0-9_-]{20,}\.|(?i)(secret|password|token|api_?key)[A-Za-z_]*\s*[=:]\s*["']?[A-Za-z0-9/+_-]{8,}`)
	// Synthetic positive controls, built at runtime so no credential-shaped literal is shipped.
	for i, sample := range []string{
		"AUTH_SECRET=" + "FAKE-SYNTHETIC-SECRET-0000",
		"password: " + "FAKE-SYNTHETIC-PASSWORD-0000",
		"sk_" + "live_" + "FAKESYNTHETIC0000",
	} {
		if !secret.MatchString(sample) {
			t.Fatalf("scanner misses positive control %d", i)
		}
	}
	if len(All()) != 7 {
		t.Fatalf("scanning %d templates, want 7", len(All()))
	}
	for _, name := range All() {
		scanned := map[string]bool{}
		err := fs.WalkDir(templateFS, name, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			scanned[path] = true
			data, rerr := templateFS.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			if secret.Match(data) {
				t.Errorf("%s holds a secret-looking value", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(scanned) < 3 {
			t.Errorf("%s: scanned only %d files", name, len(scanned))
		}
		if name == "multi-app" && !scanned["multi-app/.env.example"] {
			t.Error("multi-app/.env.example was not scanned")
		}
	}
}

// TestScaffoldEnvFilesArePrivate: .env* files are written 0600, also when an
// overlay replaces an existing 0644 file; other files stay 0644.
func TestScaffoldEnvFilesArePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env.example"), []byte("OLD=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold("multi-app", ScaffoldOptions{TargetDir: dir}); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]os.FileMode{".env.example": 0o600, "README.md": 0o644} {
		info, err := os.Stat(filepath.Join(dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s mode = %v, want %v", rel, info.Mode().Perm(), want)
		}
	}
}
