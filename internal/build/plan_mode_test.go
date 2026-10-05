package build

// plan_mode_test.go — fixtures and tests for the Sink/Effects seam (P7-LIVE-21).
//
// Purpose: prove (1) write mode produces the same tree, bytes and file modes
// as before the seam existed (TestWriteModeGolden compares against manifests
// recorded from origin/main in testdata/plan-mode/), and (2) plan mode renders
// the same artifacts in memory while touching nothing (see plan_mode_plan_test.go).
// Inputs: three fixture projects built in a hermetic temp root (HOME, plugin
// dir and fronting stack all live under the root so one snapshot covers every
// place a build may write).
// Outputs: golden manifests (path, mode, sha256 of path-normalized content).
// Constraints: secrets are pre-seeded so two builds of one fixture agree.
// Regenerate goldens only from a commit whose write mode is the reference:
// UPDATE_GOLDEN=1 go test ./internal/build -run WriteModeGolden.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginx"
)

const goldenDir = "testdata/plan-mode"

// fixtureNames lists the three reference projects, in golden file order.
var fixtureNames = []string{"dev-minimal", "prod-plugins", "fronted"}

// planFixture is one hermetic project: root holds everything a build may touch.
type planFixture struct {
	name    string
	root    string // snapshot root (HOME, plugins, fronting stack, project)
	workdir string
	home    string
	plugins string
}

const fixtureSecrets = `POSTGRES_PASSWORD=Kq7vZp3Lw9XnR2tYb8MdQa4s
HASURA_GRAPHQL_ADMIN_SECRET=Hn5Rt8WcXe2Vb7Zk3Qy9Lm4Pd6Sa1FgTz8Ju3
HASURA_GRAPHQL_JWT_SECRET={"type":"HS256","key":"Jw8Nc3Vx7Rb2Kt5Hy9Zp4Lq6Md1Sf0GaTe8Uo3Wi5Xk"}
PLUGIN_INTERNAL_SECRET=Yb4Tn8Cw2Vx6Rk9Hq3Zp7Lm5Jd1Sa0Fe
NOTIFY_INTERNAL_SECRET=Gp7Wc3Nx9Vb5Rt2Kq8Hy4Zl6Md1Js0Af
CRON_INTERNAL_SECRET=Xe5Rn9Cw3Vb7Tk2Hq8Zp4Ly6Jd1Ms0Ga
MINIO_ROOT_USER=minio-user-Tq8Zk
MINIO_ROOT_PASSWORD=Vb3Nc7Rx9Wt5Kq2Hy8Zp4Lm6Jd1Sa0Fe
REDIS_PASSWORD=Cw9Tn3Rx7Vb5Kq2Hy8Zp4Lm6Jd1Sa0Fg
GRAFANA_ADMIN_PASSWORD=Pq4Zk8Tn2Vx6Rc9Hw3Lb7Md5Jd1Sa0Fe
AUTH_JWT_SECRET=Rt6Nc2Vx8Wb4Kq9Hy3Zp7Lm5Jd1Sa0FeGk
HASURA_GRAPHQL_CORS_DOMAIN=https://example.org
`

// isolateEnv restores the process environment after the test: config.Load
// writes the resolved cascade into the process env, which must not leak into
// the next fixture.
func isolateEnv(t *testing.T) {
	t.Helper()
	saved := os.Environ()
	for _, kv := range saved {
		if k, _, ok := strings.Cut(kv, "="); ok && isBuildEnvKey(k) {
			_ = os.Unsetenv(k)
		}
	}
	t.Cleanup(func() {
		for _, kv := range os.Environ() {
			if k, _, ok := strings.Cut(kv, "="); ok && isBuildEnvKey(k) {
				_ = os.Unsetenv(k)
			}
		}
		for _, kv := range saved {
			if k, v, ok := strings.Cut(kv, "="); ok && isBuildEnvKey(k) {
				_ = os.Setenv(k, v)
			}
		}
	})
}

// isBuildEnvKey reports whether k is a project setting a build reads or sets.
func isBuildEnvKey(k string) bool {
	switch k {
	case "PATH", "HOME", "TMPDIR", "USER", "SHELL", "TERM", "LANG", "PWD", "TZ", "UPDATE_GOLDEN":
		return false
	}
	return !strings.HasPrefix(k, "GO") && !strings.HasPrefix(k, "CGO") &&
		!strings.HasPrefix(k, "LC_") && !strings.HasPrefix(k, "XPC_") &&
		!strings.HasPrefix(k, "__") && !strings.HasPrefix(k, "COLIMA") &&
		!strings.HasPrefix(k, "DOCKER") && !strings.HasPrefix(k, "RTK")
}

func writeFixtureFile(t *testing.T, path, body string, perm fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), perm); err != nil {
		t.Fatal(err)
	}
}

// newPlanFixture lays out the named fixture under a fresh temp root.
func newPlanFixture(t *testing.T, name string) *planFixture {
	t.Helper()
	isolateEnv(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &planFixture{name: name, root: root, home: filepath.Join(root, "home"), plugins: filepath.Join(root, "plugins")}
	f.workdir = filepath.Join(root, "project")
	if name == "fronted" {
		f.workdir = filepath.Join(root, "fronted-stack", "backend")
	}
	for _, d := range []string{f.home, f.plugins, f.workdir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", f.home)
	t.Setenv("USERPROFILE", f.home)
	t.Setenv("NSELF_PLUGIN_DIR", f.plugins)

	env := "PROJECT_NAME=golden\nENV=dev\nBASE_DOMAIN=example.test\nSSL_MODE=none\n"
	switch name {
	case "prod-plugins":
		env = "PROJECT_NAME=golden\nENV=prod\nBASE_DOMAIN=example.org\nSSL_MODE=letsencrypt\nMONITORING_ENABLED=true\n"
		certDir := filepath.Join(f.workdir, "ssl", "certificates", "example-org")
		writeFixtureFile(t, filepath.Join(certDir, "fullchain.pem"), "CERT\n", 0o644)
		writeFixtureFile(t, filepath.Join(certDir, "privkey.pem"), "KEY\n", 0o600)
		writeFixtureFile(t, filepath.Join(f.workdir, "nginx", "sites", "hand-written.conf"), "server { listen 8081; server_name hand.example.org; }\n", 0o644)
		writeFixtureFile(t, filepath.Join(f.workdir, "nginx", "sites", "stale.conf"), nginxGeneratedMarker+"\nserver { listen 80; server_name stale.example.org; }\n", 0o644)
		writePlanPlugin(t, f.plugins, "nself-alpha", 3901, true)
		writePlanPlugin(t, f.plugins, "nself-beta", 3902, false)
	case "local-tls":
		// Local certificates are generated by mkcert/openssl and the hosts file
		// is managed: the host effects plan mode must record and not perform.
		env = "PROJECT_NAME=golden\nENV=dev\nBASE_DOMAIN=app.local.nself.org\nSSL_MODE=local\n"
		writePlanPlugin(t, f.plugins, "nself-gamma", 3903, false)
		writeFixtureFile(t, filepath.Join(f.plugins, "nself-gamma", pluginComposeFilename), "services:\n  nself-gamma:\n    build:\n      context: .\n      dockerfile: Dockerfile.go\n    ports:\n      - \"3903:3903\"\n", 0o644)
	case "fronted":
		env += "NGINX_FRONTED_BY=fronted-stack\n"
		writeFixtureFile(t, filepath.Join(root, "fronted-stack", "nginx", "sites", "other-project.conf"), "# hand\nserver { listen 80; server_name other.example.test; }\n", 0o644)
	}
	secrets := fixtureSecrets
	if name == "local-tls" {
		// Leave the three persisted secrets unset: build generates them and
		// persists them to .env.secrets (the secrets-persist effect).
		var keep []string
		for _, l := range strings.Split(secrets, "\n") {
			if !strings.HasPrefix(l, "PLUGIN_INTERNAL_SECRET") && !strings.HasPrefix(l, "NOTIFY_INTERNAL_SECRET") &&
				!strings.HasPrefix(l, "CRON_INTERNAL_SECRET") && !strings.HasPrefix(l, "HASURA_GRAPHQL_JWT_SECRET") {
				keep = append(keep, l)
			}
		}
		secrets = strings.Join(keep, "\n")
	}
	writeFixtureFile(t, filepath.Join(f.workdir, ".env"), env+secrets, 0o600)
	return f
}

// writePlanPlugin installs a fake plugin with a compose fragment and nginx conf.
func writePlanPlugin(t *testing.T, pluginsDir, name string, port int, withNginx bool) {
	t.Helper()
	dir := filepath.Join(pluginsDir, name)
	writeFixtureFile(t, filepath.Join(dir, "plugin.json"), fmt.Sprintf(`{"name":%q,"port":%d,"language":"go"}`, name, port), 0o644)
	writeFixtureFile(t, filepath.Join(dir, pluginComposeFilename), fmt.Sprintf("services:\n  %s:\n    image: nself/%s:latest\n    ports:\n      - \"%d:%d\"\n", name, name, port, port), 0o644)
	writeFixtureFile(t, filepath.Join(dir, "Dockerfile"), "FROM scratch\n", 0o644)
	if withNginx {
		writeFixtureFile(t, filepath.Join(dir, "nginx", "route.conf"), "server {\n  listen 80;\n  server_name ${PLUGIN_NAME}.example.org;\n}\n", 0o644)
	}
}

// backupStampRE matches the wall-clock suffix of an nginx/sites snapshot dir.
var backupStampRE = regexp.MustCompile(`nginx-sites-\d{8}-\d{6}`)

// normalizeFixtureBytes makes a file's content comparable across runs: the
// temp root becomes <ROOT>, and .env.computed (whose extra variables are
// emitted in Go map order by buildEnvComputed, a pre-existing nondeterminism
// outside this Ticket's scope) has its lines sorted.
func normalizeFixtureBytes(base, data, root string) string {
	data = strings.ReplaceAll(data, root, "<ROOT>")
	if runtime.GOOS == "windows" {
		// Windows renders paths with backslashes; the goldens are recorded on Unix.
		data = strings.ReplaceAll(strings.ReplaceAll(data, filepath.ToSlash(root), "<ROOT>"), `\`, "/")
	}
	if base == ".env.computed" {
		lines := strings.Split(data, "\n")
		sort.Strings(lines)
		data = strings.Join(lines, "\n")
	}
	return data
}

// modeField renders a permission field; Windows has no Unix permission bits,
// so the field is masked there (and in the golden, see maskModes).
func modeField(info fs.FileInfo) string {
	if runtime.GOOS == "windows" {
		return "----"
	}
	return fmt.Sprintf("%04o", info.Mode().Perm())
}

// maskModes blanks the permission field of golden lines on Windows.
func maskModes(golden string) string {
	if runtime.GOOS != "windows" {
		return golden
	}
	return regexp.MustCompile(`(?m)^([df]) [0-7]{4} `).ReplaceAllString(golden, "$1 ---- ")
}

// planSnapshot returns one sorted line per entry: kind, mode, path, sha256 of
// the content with the temp root replaced by <ROOT>.
func planSnapshot(t *testing.T, root string) []string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = backupStampRE.ReplaceAllString(rel, "nginx-sites-<TS>")
		if d.IsDir() {
			lines = append(lines, fmt.Sprintf("d %s %s", modeField(info), filepath.ToSlash(rel)))
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256([]byte(normalizeFixtureBytes(filepath.Base(p), string(data), root)))
		lines = append(lines, fmt.Sprintf("f %s %s %s", modeField(info), filepath.ToSlash(rel), hex.EncodeToString(sum[:])))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return lines
}

// TestWriteModeGolden builds each fixture in write mode and compares the whole
// tree (paths, modes, content hashes) with the manifest recorded from
// origin/main before the seam existed.
func TestWriteModeGolden(t *testing.T) {
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			f := newPlanFixture(t, name)
			if _, err := Build(f.workdir, BuildOptions{}); err != nil {
				t.Fatalf("Build: %v", err)
			}
			got := strings.Join(planSnapshot(t, f.root), "\n") + "\n"
			path := filepath.Join(goldenDir, "golden-"+name+".txt")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll(goldenDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if maskModes(string(want)) != got {
				t.Errorf("write-mode tree for %s differs from the origin/main golden:\n%s", name, lineDiff(maskModes(string(want)), got))
			}
		})
	}
}

// lineDiff lists the lines present in only one of two newline-separated texts.
func lineDiff(want, got string) string {
	in := func(text string) map[string]bool {
		m := map[string]bool{}
		for _, l := range strings.Split(text, "\n") {
			m[l] = true
		}
		return m
	}
	w, g := in(want), in(got)
	var out []string
	for _, l := range strings.Split(want, "\n") {
		if !g[l] {
			out = append(out, "- "+l)
		}
	}
	for _, l := range strings.Split(got, "\n") {
		if !w[l] {
			out = append(out, "+ "+l)
		}
	}
	return strings.Join(out, "\n")
}

// stubHostTools puts failing mkcert/openssl/docker/nself/nginx stubs first on
// PATH. Each appends its argv to the returned marker file and exits 1, so a
// plan-mode build that runs any of them leaves evidence (and a write-mode
// build that needed them would fail loudly).
func stubHostTools(t *testing.T, root string) string {
	t.Helper()
	bin := filepath.Join(root, "stub-bin")
	marker := filepath.Join(root, "stub-invoked.log")
	for _, tool := range []string{"mkcert", "openssl", "docker", "nself", "nginx"} {
		writeFixtureFile(t, filepath.Join(bin, tool), "#!/bin/sh\necho \""+tool+" $*\" >> '"+marker+"'\nexit 1\n", 0o755)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marker
}

// lockTree makes every directory under root read-only for the test, so a stray
// write fails loudly instead of passing unnoticed; a no-op where chmod does not
// bind (Windows, root).
func lockTree(t *testing.T, root string) (unlock func()) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return func() {}
	}
	var dirs []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	for _, d := range dirs {
		_ = os.Chmod(d, 0o555)
	}
	unlock = func() {
		for _, d := range dirs {
			_ = os.Chmod(d, 0o755)
		}
	}
	t.Cleanup(unlock)
	return unlock
}

// planBuild runs a plan-mode build on f under stubs and a read-only tree and
// returns the result plus the tree lines before and after.
func planBuild(t *testing.T, f *planFixture) (*BuildResult, []string, []string) {
	t.Helper()
	marker := stubHostTools(t, f.root)
	before := planSnapshot(t, f.root)
	unlock := lockTree(t, f.root)
	res, err := Build(f.workdir, BuildOptions{Mode: ModePlan})
	unlock()
	if err != nil {
		t.Fatalf("plan Build: %v", err)
	}
	after := planSnapshot(t, f.root)
	if data, err := os.ReadFile(marker); err == nil {
		t.Errorf("plan mode ran a host tool:\n%s", data)
	}
	if res.Planned == nil {
		t.Fatal("plan mode returned no Planned set")
	}
	return res, before, after
}

// TestPlanModeNoWrites: plan mode leaves the project, HOME, the plugin dir and
// the fronting stack byte-for-byte untouched, never creates .nself/, and never
// runs mkcert, openssl, docker, nself or nginx.
func TestPlanModeNoWrites(t *testing.T) {
	for _, name := range append([]string{"local-tls"}, fixtureNames...) {
		t.Run(name, func(t *testing.T) {
			f := newPlanFixture(t, name)
			_, before, after := planBuild(t, f)
			if d := lineDiff(strings.Join(before, "\n"), strings.Join(after, "\n")); d != "" {
				t.Errorf("plan mode changed the tree:\n%s", d)
			}
			if _, err := os.Stat(filepath.Join(f.workdir, ".nself")); err == nil {
				t.Error("plan mode created .nself/")
			}
		})
	}
}

// plannedDiskPath maps a Planned key back to where write mode puts the file.
func plannedDiskPath(f *planFixture, key string) string {
	switch {
	case strings.HasPrefix(key, frontingPrefix):
		return filepath.Join(f.root, "fronted-stack", "nginx", "sites", strings.TrimPrefix(key, frontingPrefix))
	case filepath.IsAbs(key):
		return key
	}
	return filepath.Join(f.workdir, filepath.FromSlash(key))
}

// TestPlanModeEqualsWrite: for each reference fixture the artifacts plan mode
// returns equal, byte for byte and mode for mode, what a write-mode build then
// leaves on disk, and the write-mode changes are exactly the planned files
// (plus the host effects: backup snapshot, plugin fragments).
func TestPlanModeEqualsWrite(t *testing.T) {
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			f := newPlanFixture(t, name)
			realPath := os.Getenv("PATH")
			res, _, _ := planBuild(t, f)
			t.Setenv("PATH", realPath) // the write run uses the real PATH, without the failing stubs
			if len(res.Planned.Files) < 10 {
				t.Fatalf("plan lists only %d files", len(res.Planned.Files))
			}
			before := planSnapshot(t, f.root)
			if _, err := Build(f.workdir, BuildOptions{}); err != nil {
				t.Fatalf("write Build: %v", err)
			}
			for key, pf := range res.Planned.Files {
				disk := plannedDiskPath(f, key)
				got, err := os.ReadFile(disk)
				if err != nil {
					t.Errorf("planned %s was not written by write mode: %v", key, err)
					continue
				}
				if normalizeFixtureBytes(filepath.Base(disk), string(got), f.root) != normalizeFixtureBytes(filepath.Base(disk), string(pf.Data), f.root) {
					t.Errorf("planned %s differs from write mode", key)
				}
				if info, _ := os.Stat(disk); runtime.GOOS != "windows" && info.Mode().Perm() != pf.Perm {
					t.Errorf("planned %s mode %04o, write mode %04o", key, pf.Perm, info.Mode().Perm())
				}
			}
			planned := map[string]bool{}
			for key := range res.Planned.Files {
				rel, _ := filepath.Rel(f.root, plannedDiskPath(f, key))
				planned[filepath.ToSlash(rel)] = true
			}
			beforeSet := map[string]bool{}
			for _, l := range before {
				beforeSet[l] = true
			}
			for _, e := range res.Planned.Effects {
				if e.Kind == EffectPluginFragment {
					rel, _ := filepath.Rel(f.root, e.Target)
					planned[filepath.ToSlash(rel)] = true // an in-place rewrite is an effect, not an artifact
				}
			}
			for _, l := range planSnapshot(t, f.root) {
				fields := strings.Fields(l)
				if fields[0] != "f" || beforeSet[l] {
					continue
				}
				p := fields[2]
				if !planned[p] && !strings.Contains(p, ".nself/backups/") {
					t.Errorf("write mode changed %s, which the plan does not list", p)
				}
			}
		})
	}
}

// TestPlanModePrune: a marker-bearing site conf the new build no longer
// produces is reported as a removal in plan mode (and stays on disk), and
// write mode deletes exactly that set; a hand-written conf is never removed.
func TestPlanModePrune(t *testing.T) {
	f := newPlanFixture(t, "prod-plugins")
	realPath := os.Getenv("PATH")
	res, _, _ := planBuild(t, f)
	t.Setenv("PATH", realPath)
	if got := strings.Join(res.Planned.Removed, ","); got != "nginx/sites/stale.conf" {
		t.Fatalf("plan Removed = %q, want nginx/sites/stale.conf", got)
	}
	stale := filepath.Join(f.workdir, "nginx", "sites", "stale.conf")
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("plan mode deleted stale.conf: %v", err)
	}
	var backup bool
	for _, e := range res.Planned.Effects {
		backup = backup || e.Kind == EffectNginxSitesBackup
	}
	if !backup {
		t.Errorf("no nginx-sites-backup effect in %+v", res.Planned.Effects)
	}
	if _, err := Build(f.workdir, BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("write mode kept the stale generated conf")
	}
	if _, err := os.Stat(filepath.Join(f.workdir, "nginx", "sites", "hand-written.conf")); err != nil {
		t.Errorf("write mode removed the hand-written conf: %v", err)
	}
}

// TestPlanModeAutoInstall: a declared plugin that is not installed is a
// plugin-install effect; plan mode makes no registry request and does not
// report the plugin missing (apply installs it).
func TestPlanModeAutoInstall(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	var requests atomic.Int32
	lis, err := net.Listen("tcp4", "127.0.0.1:0") // some CI hosts have no IPv6 loopback
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	srv.Listener = lis
	srv.Start()
	defer srv.Close()
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	writeFixtureFile(t, filepath.Join(f.workdir, "nself.yaml"), "app: golden\nplugins:\n  free:\n    - ghost-plugin\n", 0o644)
	res, _, _ := planBuild(t, f)
	var found bool
	for _, e := range res.Planned.Effects {
		found = found || (e.Kind == EffectPluginInstall && e.Target == "ghost-plugin")
	}
	if !found {
		t.Errorf("no plugin-install effect for ghost-plugin in %+v", res.Planned.Effects)
	}
	if n := requests.Load(); n != 0 {
		t.Errorf("plan mode made %d registry requests", n)
	}
	if len(res.MissingPlugins) != 0 {
		t.Errorf("plan mode reported missing plugins %v", res.MissingPlugins)
	}
}

// TestPlanModeHostEffects: a fixture that needs new local certificates records
// certificates (with the domains), hosts, trust-store, secrets-persist (key
// names only), plugin-fragment and build-lock effects, and still renders the
// TLS server blocks write mode produces.
func TestPlanModeHostEffects(t *testing.T) {
	f := newPlanFixture(t, "local-tls")
	res, _, _ := planBuild(t, f)
	byKind := map[string]PlannedEffect{}
	for _, e := range res.Planned.Effects {
		byKind[e.Kind] = e
	}
	for _, k := range []string{EffectCertificates, EffectHosts, EffectTrustStore, EffectSecretsPersist, EffectPluginFragment, EffectBuildLock} {
		if _, ok := byKind[k]; !ok {
			t.Errorf("missing %s effect in %+v", k, effectKinds(res.Planned.Effects))
		}
	}
	if d := byKind[EffectCertificates].Detail; !strings.Contains(d, "*.local.nself.org") || !strings.Contains(d, "localhost") {
		t.Errorf("certificates effect lacks the domains: %q", d)
	}
	if d := byKind[EffectSecretsPersist].Detail; !strings.Contains(d, "PLUGIN_INTERNAL_SECRET") || strings.Contains(d, "=") {
		t.Errorf("secrets-persist detail should name keys only: %q", d)
	}
	var tls bool
	for key, pf := range res.Planned.Files {
		if strings.HasPrefix(key, "nginx/sites/") && strings.Contains(string(pf.Data), "listen 443 ssl") {
			tls = true
		}
	}
	if !tls {
		t.Error("plan lost the TLS server blocks")
	}
	if _, ok := res.Planned.Files["ssl/certificates/app-local-nself-org/fullchain.pem"]; ok {
		t.Error("plan invented certificate bytes")
	}
}

// TestPlanModeAssumedCerts: with SSL_MODE=custom and no certificate on disk,
// nginx omits TLS; WithAssumedCerts keeps the blocks a certificate-creating
// build would emit.
func TestPlanModeAssumedCerts(t *testing.T) {
	cfg, err := config.ApplyDefaults(&config.Config{BaseDomain: "example.org", SSLMode: "custom", ProjectName: "x"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	plain, err := nginx.NewGenerator(cfg, dir).Generate()
	if err != nil {
		t.Fatal(err)
	}
	assumed, err := nginx.NewGenerator(cfg, dir).WithAssumedCerts([]string{"example.org"}).Generate()
	if err != nil {
		t.Fatal(err)
	}
	has := func(m map[string]string) bool {
		for _, v := range m {
			if strings.Contains(v, "listen 443 ssl") {
				return true
			}
		}
		return false
	}
	if has(plain) || !has(assumed) {
		t.Errorf("TLS blocks: without assumed certs %v, with %v", has(plain), has(assumed))
	}
}

// effectKinds lists kind:target for failure output, without long paths.
func effectKinds(effects []PlannedEffect) []string {
	var out []string
	for _, e := range effects {
		out = append(out, e.Kind+":"+filepath.Base(e.Target))
	}
	return out
}
