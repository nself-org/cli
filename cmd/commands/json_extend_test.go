package commands

// Tests for the services, extend, admin and bundle_remove fragments
// (P7-SURF-13): every runnable document row is an envelope in v1.5, the rows
// that cannot be one carry a reason, and the real bodies of add, remove, the
// services and plugin reads and the admin family print the documents their
// schemas describe.
//
// Constraints: fake plugin dir, project and HOME under t.TempDir; docker is a
// POSIX shell stub (those tests skip on Windows); no network beyond loopback;
// obviously fake names only.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/builtin/admin"
	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

func TestEnvelopeCoverageServices(t *testing.T) {
	assertFragmentEnvelopeCoverage(t, "services")
}

func TestEnvelopeCoverageExtend(t *testing.T) {
	assertFragmentEnvelopeCoverage(t, "extend")
	rows := registryRows(t, true)
	if r := rows["nself config plugins"]; r.Output != canon.OutputDocument || r.JSON != canon.JSONEnvelope {
		t.Fatalf("config plugins = output %s json %s, want a document envelope (the bare hub)", r.Output, r.JSON)
	}
	if r := rows["nself config plugins logs"]; r.Output != canon.OutputStream {
		t.Fatalf("config plugins logs output = %s, want stream", r.Output)
	}
	if r := rows["nself add"]; r.SideEffect != canon.SideEffectRemote || r.Confirm == nil || !hasString(r.Confirm.Flags, "yes") {
		t.Fatalf("add = %+v, want remote with a confirm block naming --yes", r)
	}
	old := registryRows(t, false)
	for _, p := range []string{"config plugins list", "config plugins info", "config plugins search", "config plugins count", "config bundles list", "config bundles info"} {
		if old["nself "+p].JSON != canon.JSONLegacy {
			t.Errorf("v1.4 %s json = %s, want legacy (it printed JSON before the contract)", p, old["nself "+p].JSON)
		}
		if rows["nself "+p].JSON != canon.JSONEnvelope {
			t.Errorf("v1.5 %s json = %s, want envelope", p, rows["nself "+p].JSON)
		}
	}
}

// TestEnvelopeCoverageAdmin: the admin family is a builtin plugin (canon
// plugin), which the generic coverage helper skips, so every row is checked
// here too: documents are envelopes, `admin logs --follow` carries the stream
// flag override, `admin connect` is a stream with a reason comment.
func TestEnvelopeCoverageAdmin(t *testing.T) {
	assertFragmentEnvelopeCoverage(t, "admin")
	rows := registryRows(t, true)
	for _, p := range []string{"admin", "admin health", "admin logs", "admin projects", "admin projects add", "admin projects list", "admin projects remove", "admin start", "admin stop"} {
		r, ok := rows["nself "+p]
		if !ok || r.Output != canon.OutputDocument || r.JSON != canon.JSONEnvelope {
			t.Errorf("%s = %+v, want a document envelope", p, r)
		}
	}
	follow := ""
	for _, f := range rows["nself admin logs"].Flags {
		if f.Name == "follow" && f.Output != nil {
			follow = *f.Output
		}
	}
	if follow != canon.OutputStream {
		t.Fatalf("admin logs --follow output = %q, want stream", follow)
	}
	if got := jsonRecordTypes["admin logs"]; got == nil {
		t.Fatal("admin logs has no registered stream record type")
	}
	raw, err := os.ReadFile("../../internal/canon/domains/admin.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if r := rows["nself admin connect"]; r.Output != canon.OutputInteractive || !rowHasReason(string(raw), "admin connect") {
		t.Fatalf("admin connect = output %s, want interactive with a reason comment", r.Output)
	}
}

// TestEnvelopeCoverageBundleRemove: PLUG-20 ruled that bundle remove has no
// fragment file of its own (its row lives in extend.yaml and is a v1.5 shim of
// `remove`), so the destructive-with-confirm contract is checked on the rows.
func TestEnvelopeCoverageBundleRemove(t *testing.T) {
	rows := registryRows(t, true)
	rm := rows["nself remove"]
	if rm.JSON != canon.JSONEnvelope || rm.SideEffect != canon.SideEffectDestructive {
		t.Fatalf("remove = json %s side_effect %s, want envelope/destructive", rm.JSON, rm.SideEffect)
	}
	if rm.Confirm == nil || !hasString(rm.Confirm.Flags, "yes") {
		t.Fatalf("remove confirm = %+v, want a block naming --yes (the consent flag of --delete-data)", rm.Confirm)
	}
	hasDelete := false
	for _, f := range rm.Flags {
		hasDelete = hasDelete || f.Name == "delete-data"
	}
	if !hasDelete {
		t.Fatal("remove has no --delete-data flag")
	}
	br := rows["nself bundle remove"]
	if br.SideEffect != canon.SideEffectDestructive || br.Target == nil || *br.Target != "remove" {
		t.Fatalf("bundle remove = side_effect %s target %v, want destructive shim of remove", br.SideEffect, br.Target)
	}
	// The v1.4 row is the command itself: destructive, with the consent flag and --delete-data.
	old := registryRows(t, false)["nself bundle remove"]
	oldDelete := false
	for _, f := range old.Flags {
		oldDelete = oldDelete || f.Name == "delete-data"
	}
	if old.SideEffect != canon.SideEffectDestructive || !oldDelete || old.Confirm == nil || !hasString(old.Confirm.Flags, "yes") {
		t.Fatalf("v1.4 bundle remove = %+v, want destructive with --delete-data and a confirm block naming --yes", old)
	}
}

func TestInstallSourceIdentity(t *testing.T) {
	for in, want := range map[string]string{
		"waf": "waf",
		"https://user" + ":" + "synthetic@example.test/p/plugin.tgz?token=abc#frag": "https://example.test/p/plugin.tgz",
		"https://example.test/p/plugin.tgz":                           "https://example.test/p/plugin.tgz",
	} {
		if got := installSourceIdentity(in); got != want {
			t.Errorf("installSourceIdentity(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConfigPluginsHubAndProxyRefusal(t *testing.T) {
	cmd := extendCmd(t, "config", "plugins")
	out, err := runExt(t, cmd, nil, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	var d dataHub
	extendData(t, out, "config plugins", &d)
	if len(d.Subcommands) == 0 {
		t.Fatalf("no subcommands: %s", out)
	}
	if _, err := runExt(t, cmd, []string{"someplugin", "status"}, map[string]string{}); err == nil || !strings.Contains(err.Error(), "does not support --json") {
		t.Fatalf("proxy with --json: err = %v, want the E402 refusal", err)
	}
}

func registryRows(t *testing.T, v15 bool) map[string]cmdregistry.Command {
	t.Helper()
	reg, err := buildRegistry(v15)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]cmdregistry.Command{}
	for _, c := range reg.Commands {
		rows[c.Path] = c
	}
	return rows
}

func hasString(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

// extendCmd finds a command of the canonical v1.5 tree by its words.
func extendCmd(t *testing.T, words ...string) *cobra.Command {
	t.Helper()
	undo := prepareTreeWith(&canonTable, RootCmd, true, false)
	t.Cleanup(undo)
	c, _, err := RootCmd.Find(words)
	if err != nil || c == RootCmd {
		t.Fatalf("no command %v: %v", words, err)
	}
	return c
}

// runExt runs the (wrapped) RunE of cmd in v1.5 --json mode, the way the CLI
// does, and returns the document written to the real stdout.
func runExt(t *testing.T, cmd *cobra.Command, args []string, flags map[string]string) (string, error) {
	t.Helper()
	flags["json"] = "true"
	cmd.SetContext(context.Background())
	return jsonV15(t, cmd, flags, func() error {
		restore := output.IsolateStdout()
		defer restore()
		return cmd.RunE(cmd, args)
	})
}

func extendData(t *testing.T, out, command string, into any) {
	t.Helper()
	doc := oneEnvelope(t, out, command)
	dec := json.NewDecoder(strings.NewReader(string(doc["data"])))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		t.Fatalf("%s data: %v\n%s", command, err, out)
	}
}

func fakeProject(t *testing.T, env string) {
	t.Helper()
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)
}

// The union row of `config services list` prints the same bytes as the
// pre-contract rows, for a project listing and for --core.
func TestServiceListUnionBytes(t *testing.T) {
	status := []serviceStatusRow{{Service: "redis", Enabled: false, Status: "disabled", Port: "6379", EnvVar: "REDIS_ENABLED"}, {Service: "admin", Enabled: true, Status: "enabled", Port: "3021", EnvVar: "NSELF_ADMIN_ENABLED"}}
	want, _ := json.Marshal(status)
	if got, _ := json.Marshal(newServiceListData(status, nil)); string(got) != string(want) {
		t.Fatalf("project rows differ\n got %s\nwant %s", got, want)
	}
	cat := serviceCatalogRows()
	if len(cat) == 0 {
		t.Fatal("empty service catalog")
	}
	want, _ = json.Marshal(cat)
	if got, _ := json.Marshal(newServiceListData(nil, cat)); string(got) != string(want) {
		t.Fatalf("catalog rows differ\n got %s\nwant %s", got, want)
	}
}

func TestServicesBodies(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		fakeProject(t, "PROJECT_NAME=smoke\nREDIS_ENABLED=true\n")
		out, err := runExt(t, extendCmd(t, "config", "services", "list"), nil, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		var d serviceListData
		extendData(t, out, "config services list", &d)
		if len(d) != len(knownServices) || d[0].Service != "redis" || d[0].Enabled == nil || !*d[0].Enabled {
			t.Fatalf("rows: %+v", d)
		}
	})
	t.Run("list core", func(t *testing.T) {
		fakeProject(t, "PROJECT_NAME=smoke\n")
		out, err := runExt(t, extendCmd(t, "config", "services", "list"), nil, map[string]string{"core": "true"})
		if err != nil {
			t.Fatal(err)
		}
		var d serviceListData
		extendData(t, out, "config services list", &d)
		if len(d) != len(serviceCatalogRows()) || d[0].Tier == "" || d[0].Enabled != nil {
			t.Fatalf("catalog rows: %+v", d)
		}
	})
	t.Run("enable and disable report the state after", func(t *testing.T) {
		fakeProject(t, "PROJECT_NAME=smoke\n")
		for _, tc := range []struct {
			verb string
			want bool
		}{{"enable", true}, {"disable", false}} {
			out, err := runExt(t, extendCmd(t, "config", "services", tc.verb), []string{"mailpit"}, map[string]string{})
			if err != nil {
				t.Fatal(err)
			}
			var d serviceToggleData
			extendData(t, out, "config services "+tc.verb, &d)
			if d.Service != "email" || d.EnvVar != "MAILPIT_ENABLED" || d.Enabled != tc.want {
				t.Fatalf("%s: %+v", tc.verb, d)
			}
		}
	})
	t.Run("group answers with its subcommands", func(t *testing.T) {
		out, err := runExt(t, extendCmd(t, "config", "services"), nil, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		var d dataHub
		extendData(t, out, "config services", &d)
		if len(d.Subcommands) == 0 {
			t.Fatalf("no subcommands: %s", out)
		}
	})
}

// A fake docker prints two lines for `logs` and nothing else.
func fakeDockerLogs(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the docker double is a POSIX shell script")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in logs) printf 'admin up\\n'; printf 'admin ready\\n' >&2 ;; esac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	prependPath(t, bin)
}

func TestExtendBodies(t *testing.T) {
	t.Run("add and remove a builtin", func(t *testing.T) {
		t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
		out, err := runExt(t, extendCmd(t, "add"), []string{"admin"}, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		var a addData
		extendData(t, out, "add", &a)
		if len(a.Installed) != 1 || a.Installed[0] != "admin" || len(a.Skipped) != 0 || a.Next != "nself start" || a.DryRun {
			t.Fatalf("add: %+v", a)
		}
		out, err = runExt(t, extendCmd(t, "remove"), []string{"admin"}, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		var r removeData
		extendData(t, out, "remove", &r)
		if len(r.Removed) != 1 || r.Removed[0] != "admin" || r.Next != "nself start" || len(r.Kept) != 0 {
			t.Fatalf("remove: %+v", r)
		}
	})
	t.Run("bundle results come from the ledger-backed results", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("NSELF_PLUGIN_DIR", dir)
		manifest, err := os.ReadFile(filepath.Join("testdata", "mount", "collide-completion", "plugin.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "zzhint"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "zzhint", "plugin.json"), manifest, 0o644); err != nil {
			t.Fatal(err)
		}
		install := outcomeBody(func(cmd *cobra.Command, _ []string) error {
			recordBundleInstall(cmd, &bundle.InstallResult{Installed: []string{"zzhint", "other"}, Skipped: []string{"gone"}})
			return nil
		}, addFacts)
		cmd := extendCmd(t, "add")
		cmd.SetContext(context.Background())
		out, err := jsonV15(t, cmd, map[string]string{"json": "true"}, func() error {
			restore := output.IsolateStdout()
			defer restore()
			return install(cmd, []string{"nchat"})
		})
		if err != nil {
			t.Fatal(err)
		}
		var a addData
		extendData(t, out, "add", &a)
		if strings.Join(a.Installed, ",") != "zzhint,other" || strings.Join(a.Skipped, ",") != "gone" || a.Next != "nself completion --help" {
			t.Fatalf("add: %+v", a)
		}
		remove := outcomeBody(func(cmd *cobra.Command, _ []string) error {
			recordBundleRemove(cmd, &bundle.RemoveResult{Removed: []string{"a"}, Kept: []bundle.KeptPlugin{{Name: "b", Reason: "explicit"}}, Planned: []string{"a"}})
			return nil
		}, removeFacts)
		rm := extendCmd(t, "remove")
		rm.SetContext(context.Background())
		out, err = jsonV15(t, rm, map[string]string{"json": "true", "dry-run": "true"}, func() error {
			restore := output.IsolateStdout()
			defer restore()
			return remove(rm, []string{"nchat"})
		})
		if err != nil {
			t.Fatal(err)
		}
		var r removeData
		extendData(t, out, "remove", &r)
		if strings.Join(r.Removed, ",") != "a" || strings.Join(r.Kept, ",") != "b" || strings.Join(r.Planned, ",") != "a" || !r.DryRun {
			t.Fatalf("remove: %+v", r)
		}
	})
	t.Run("inventory status updates of an empty plugin dir", func(t *testing.T) {
		t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
		out, err := runExt(t, extendCmd(t, "config", "plugins", "inventory"), nil, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		var inv pluginInventoryData
		extendData(t, out, "config plugins inventory", &inv)
		if inv.Plugins == nil || len(inv.Plugins) != 0 {
			t.Fatalf("inventory: %+v", inv)
		}
		out, err = runExt(t, extendCmd(t, "config", "plugins", "status"), nil, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		var st pluginStatusData
		extendData(t, out, "config plugins status", &st)
		if st.Plugins == nil || len(st.Plugins) != 0 {
			t.Fatalf("status: %+v", st)
		}
		out, err = runExt(t, extendCmd(t, "config", "plugins", "list"), nil, map[string]string{"installed": "true"})
		if err != nil {
			t.Fatal(err)
		}
		var l pluginListData
		extendData(t, out, "config plugins list", &l)
		if l == nil || len(l) != 0 {
			t.Fatalf("list: %+v", l)
		}
	})
	t.Run("count prints the embedded artifact", func(t *testing.T) {
		out, err := runExt(t, extendCmd(t, "config", "plugins", "count"), nil, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		doc := oneEnvelope(t, out, "config plugins count")
		if !strings.Contains(string(doc["data"]), `"advertised"`) {
			t.Fatalf("count data: %s", doc["data"])
		}
	})
	t.Run("compat-check of an empty plugin dir", func(t *testing.T) {
		t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
		t.Setenv("NSELF_PLUGIN_REGISTRY", "http://127.0.0.1:1")
		out, err := runExt(t, extendCmd(t, "config", "plugins", "compat-check"), nil, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		var d compatCheckData
		extendData(t, out, "config plugins compat-check", &d)
		if d.CLIVersion == "" || d.Plugins == nil || len(d.Plugins) != 0 {
			t.Fatalf("compat-check: %+v", d)
		}
	})
}

// adminTree mounts the builtin admin family under a root with the persistent
// --json flag, as the CLI does, and returns the subcommand for words.
func adminTree(t *testing.T, words ...string) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "nself"}
	root.PersistentFlags().Bool("json", false, "")
	root.AddCommand(admin.Command(adminDeps()))
	c, _, err := root.Find(append([]string{"admin"}, words...))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAdminStartBinding(t *testing.T) {
	for _, tc := range []struct {
		ports  []string
		listen string
		expose bool
		seen   bool
	}{
		{[]string{"127.0.0.1:3021->3021/tcp"}, "127.0.0.1:3021", false, true},
		{[]string{"0.0.0.0:3021->3021/tcp", "3000/tcp"}, "0.0.0.0:3021", true, true},
		{[]string{"3021/tcp"}, "", false, false},
		{nil, "", false, false},
	} {
		got := admin.BindingOf(tc.ports)
		if got.Listen != tc.listen || got.ExposeNetwork != tc.expose || got.Observed != tc.seen {
			t.Errorf("BindingOf(%v) = %+v", tc.ports, got)
		}
	}
}

func TestAdminBodies(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Run("projects add, list, remove", func(t *testing.T) {
		out, err := runExt(t, adminTree(t, "projects", "add"), nil, map[string]string{"name": "demo", "host": "demo.example.test"})
		if err != nil {
			t.Fatal(err)
		}
		var ch admin.ProjectChange
		extendData(t, out, "admin projects add", &ch)
		if ch.ID != "demo" || ch.Action != "added" {
			t.Fatalf("add: %+v", ch)
		}
		out, err = runExt(t, adminTree(t, "projects", "list"), nil, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		var l admin.ProjectList
		extendData(t, out, "admin projects list", &l)
		if len(l) != 1 || l[0].ID != "demo" || l[0].Host != "demo.example.test" {
			t.Fatalf("list: %+v", l)
		}
		out, err = runExt(t, adminTree(t, "projects", "remove"), []string{"demo"}, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		extendData(t, out, "admin projects remove", &ch)
		if ch.ID != "demo" || ch.Action != "removed" {
			t.Fatalf("remove: %+v", ch)
		}
		out, err = runExt(t, adminTree(t, "projects", "list"), nil, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		extendData(t, out, "admin projects list", &l)
		if l == nil || len(l) != 0 {
			t.Fatalf("empty list must be [], got %+v", l)
		}
	})
	t.Run("health", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
		defer srv.Close()
		u, _ := url.Parse(srv.URL)
		t.Setenv("NSELF_ADMIN_PORT", u.Port())
		out, err := runExt(t, adminTree(t, "health"), nil, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		var d admin.HealthData
		extendData(t, out, "admin health", &d)
		if !d.Healthy || d.StatusCode != 200 || !strings.HasSuffix(d.URL, "/health") {
			t.Fatalf("health: %+v", d)
		}
	})
	t.Run("logs", func(t *testing.T) {
		fakeDockerLogs(t)
		out, err := runExt(t, adminTree(t, "logs"), nil, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		var d admin.LogsData
		extendData(t, out, "admin logs", &d)
		if len(d.Lines) != 2 || d.Lines[1] != "admin ready" || d.Container == "" {
			t.Fatalf("logs: %+v", d)
		}
	})
	t.Run("logs follow is a stream", func(t *testing.T) {
		fakeDockerLogs(t)
		out, err := runExt(t, adminTree(t, "logs"), nil, map[string]string{"follow": "true"})
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != 3 || !strings.Contains(lines[0], `"type":"log"`) || !strings.Contains(lines[0], `"message":"admin up"`) ||
			!strings.Contains(lines[2], `"type":"result"`) || !strings.Contains(lines[2], `"lines":2`) {
			t.Fatalf("stream:\n%s", out)
		}
	})
	t.Run("opening Admin in a headless run", func(t *testing.T) {
		out, err := runExt(t, adminTree(t), nil, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		var d admin.OpenData
		extendData(t, out, "admin", &d)
		if !strings.HasPrefix(d.URL, "http://localhost:") {
			t.Fatalf("open: %+v", d)
		}
	})
}
