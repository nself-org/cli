package reconcile

// planner_test.go — fixtures and tests for Compute (P7-LIVE-03).
//
// Purpose: prove the plan is read-only (the project tree, the plugin dir and the
// container list are identical before and after), is complete (apply then plan
// is empty), and carries the right container impact.
// Inputs: hermetic fixture projects copied from testdata/fixtures into a temp
// root that also holds HOME and the plugin dir, so one hash covers every place
// a build may write.
// Constraints: no real host, no real project, no docker daemon needed (a fake
// ContainerRuntime stands in); the process environment is restored after each
// test because config.Load writes the cascade into it.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/joho/godotenv"
	nbuild "github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat/compattest"
	"gopkg.in/yaml.v3"
)

// fixtureNames lists the five reference projects.
var fixtureNames = []string{"dev-minimal", "dev-plugin", "prod-ssl", "prod-plugins", "staging-hand"}

// fixture is one hermetic project.
type fixture struct {
	root    string
	project string
	home    string
	plugins string
}

// buildEnvKey reports whether k is a setting a build reads or sets, which a
// test must not leak into the next one.
func buildEnvKey(k string) bool {
	switch k {
	case "PATH", "HOME", "TMPDIR", "USER", "SHELL", "TERM", "LANG", "PWD", "TZ", "UPDATE_GOLDEN":
		return false
	}
	for _, p := range []string{"GO", "CGO", "LC_", "XPC_", "__", "COLIMA", "DOCKER", "RTK", "NSELF_V15", "GITHUB_", "RUNNER_", "CI"} {
		if strings.HasPrefix(k, p) {
			return false
		}
	}
	return true
}

// isolateEnv unsets every build setting now and restores the environment on
// cleanup.
func isolateEnv(t *testing.T) {
	t.Helper()
	saved := os.Environ()
	clear := func() {
		for _, kv := range os.Environ() {
			if k, _, ok := strings.Cut(kv, "="); ok && buildEnvKey(k) {
				_ = os.Unsetenv(k)
			}
		}
	}
	clear()
	t.Cleanup(func() {
		clear()
		for _, kv := range saved {
			if k, v, ok := strings.Cut(kv, "="); ok && buildEnvKey(k) {
				_ = os.Setenv(k, v)
			}
		}
	})
}

// loadFixture copies testdata/fixtures/<name> into a fresh root and points
// HOME and NSELF_PLUGIN_DIR at it.
func loadFixture(t *testing.T, name string) *fixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixture projects use unix modes and paths")
	}
	isolateEnv(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{root: root, project: filepath.Join(root, "project"), home: filepath.Join(root, "home"), plugins: filepath.Join(root, "plugins")}
	for _, d := range []string{f.project, f.home, f.plugins} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", f.home)
	t.Setenv("NSELF_PLUGIN_DIR", f.plugins)
	src := filepath.Join("testdata", "fixtures", name)
	copyTree(t, filepath.Join(src, "project"), f.project)
	copyTree(t, filepath.Join(src, "plugins"), f.plugins)
	env, err := os.ReadFile(filepath.Join(f.project, "dotenv"))
	if err != nil {
		t.Fatal(err)
	}
	secrets, err := os.ReadFile(filepath.Join("testdata", "fixtures", "secrets.env"))
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(f.project, "dotenv"))
	writeFile(t, filepath.Join(f.project, ".env"), string(env)+string(secrets), 0o600)
	if certs, err := os.ReadFile(filepath.Join(src, "certs.txt")); err == nil {
		for _, dir := range strings.Fields(string(certs)) {
			writeFile(t, filepath.Join(f.project, "ssl", "certificates", dir, "fullchain.pem"), "CERT\n", 0o644)
			writeFile(t, filepath.Join(f.project, "ssl", "certificates", dir, "privkey.pem"), "KEY\n", 0o600)
		}
	}
	return f
}

// copyTree copies dir into dst when dir exists.
func copyTree(t *testing.T, dir, dst string) {
	t.Helper()
	if _, err := os.Stat(dir); err != nil {
		return
	}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		writeFile(t, filepath.Join(dst, rel), string(data), 0o644)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, body string, perm fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), perm); err != nil {
		t.Fatal(err)
	}
}

// treeHash is one sorted line per file and directory under root (kind, mode,
// path, sha256 of the content), so any write, create or chmod changes it. The
// lock file the command guard writes is not project content and is skipped.
func treeHash(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if rel == "project/.nself/op.lock" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			lines = append(lines, fmt.Sprintf("d %04o %s", info.Mode().Perm(), rel))
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		lines = append(lines, fmt.Sprintf("f %04o %s %s", info.Mode().Perm(), rel, hex.EncodeToString(sum[:])))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// changedPaths lists the project-relative files that differ between two
// snapshots taken by fileHashes.
func fileHashes(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// planOf computes the plan of f without a container runtime.
func planOf(t *testing.T, f *fixture, mut func(*Request)) *Plan {
	t.Helper()
	req := Request{ProjectDir: f.project, Seed: []byte("fixture-seed")}
	if mut != nil {
		mut(&req)
	}
	p, err := Compute(context.Background(), req)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	return p
}

// fakeRuntime is a ContainerRuntime that computes config hashes the way compose
// does for the purposes of the plan: per service, the service block with every
// ${VAR} replaced from the --env-file values (later files win). It records the
// files it was given, so a test can prove the planned bytes reached it, and it
// counts calls so a test can prove nothing else ran.
type fakeRuntime struct {
	running  []RunningContainer
	runErr   error
	hashErr  error
	noHash   bool // answer with no hashes without reading the files
	files    []string
	envFiles []string
	dir      string
	calls    int
}

func (f *fakeRuntime) Running(context.Context, string) ([]RunningContainer, error) {
	f.calls++
	return f.running, f.runErr
}

var varRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)[^}]*\}`)

func (f *fakeRuntime) ConfigHashes(_ context.Context, files, envFiles []string, dir string) (map[string]string, error) {
	f.calls++
	f.files, f.envFiles, f.dir = files, envFiles, dir
	if f.hashErr != nil {
		return nil, f.hashErr
	}
	if f.noHash {
		return map[string]string{}, nil
	}
	return fakeHashes(files, envFiles)
}

// fakeHashes hashes each service of the merged compose files.
func fakeHashes(files, envFiles []string) (map[string]string, error) {
	env := map[string]string{}
	for _, e := range envFiles {
		m, err := godotenv.Read(e)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			env[k] = v
		}
	}
	blocks := map[string]string{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var doc struct {
			Services map[string]yaml.Node `yaml:"services"`
		}
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return nil, err
		}
		for name, n := range doc.Services {
			out, err := yaml.Marshal(&n)
			if err != nil {
				return nil, err
			}
			blocks[name] = string(out)
		}
	}
	hashes := map[string]string{}
	for name, b := range blocks {
		b = varRE.ReplaceAllStringFunc(b, func(m string) string { return env[varRE.FindStringSubmatch(m)[1]] })
		sum := sha256.Sum256([]byte(b))
		hashes[name] = hex.EncodeToString(sum[:])
	}
	return hashes, nil
}

// runningFromDisk reports every service of the project as running with the
// hash its on-disk configuration has now: the state after the last start.
func runningFromDisk(t *testing.T, f *fixture) []RunningContainer {
	t.Helper()
	var files, envs []string
	files = append(files, filepath.Join(f.project, "docker-compose.yml"))
	if raw, err := os.ReadFile(filepath.Join(f.project, ".nself", "compose-files.txt")); err == nil {
		files = nil
		for _, l := range strings.Fields(string(raw)) {
			files = append(files, l)
		}
	}
	for _, e := range []string{".env", ".nself/compose.env"} {
		if _, err := os.Stat(filepath.Join(f.project, e)); err == nil {
			envs = append(envs, filepath.Join(f.project, e))
		}
	}
	h, err := fakeHashes(files, envs)
	if err != nil {
		t.Fatal(err)
	}
	var out []RunningContainer
	for svc, hash := range h {
		out = append(out, RunningContainer{Name: "fx_" + svc, Service: svc, State: "running", ConfigHash: hash})
	}
	return out
}

// applyFixture builds the fixture for real (write mode).
func applyFixture(t *testing.T, f *fixture) {
	t.Helper()
	if _, err := Apply(context.Background(), Request{ProjectDir: f.project, Seed: []byte("fixture-seed")}, ApplyOptions{Yes: true}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

func itemFor(p *Plan, service string) (ContainerItem, bool) {
	for _, it := range p.Containers.Items {
		if it.Service == service {
			return it, true
		}
	}
	return ContainerItem{}, false
}

// setEnvValue rewrites KEY=... in the project's .env.
func setEnvValue(t *testing.T, f *fixture, key, value string) {
	t.Helper()
	path := filepath.Join(f.project, ".env")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^` + key + `=.*$`)
	if !re.Match(raw) {
		t.Fatalf("%s not in .env", key)
	}
	writeFile(t, path, string(re.ReplaceAll(raw, []byte(key+"="+value))), 0o600)
}

// TestPlanIsReadOnly: Compute leaves the whole root (project, HOME, plugin dir)
// byte for byte as it was, for every fixture, and its only docker questions are
// the two read-only ones.
func TestPlanIsReadOnly(t *testing.T) {
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			f := loadFixture(t, name)
			rt := &fakeRuntime{}
			before := treeHash(t, f.root)
			p := planOf(t, f, func(r *Request) { r.Containers, r.Runtime = true, rt })
			if p.Empty {
				t.Fatal("a first plan of a fresh project must not be empty")
			}
			if after := treeHash(t, f.root); after != before {
				t.Fatalf("Compute changed the tree:\n%s", lineDiff(before, after))
			}
			if rt.calls != 2 {
				t.Fatalf("container impact made %d docker calls, want 2 (ps, config --hash)", rt.calls)
			}
			if _, err := os.Stat(filepath.Join(f.project, ".nself")); err == nil {
				t.Fatal("Compute created .nself")
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
	for l := range w {
		if !g[l] {
			out = append(out, "- "+l)
		}
	}
	for l := range g {
		if !w[l] {
			out = append(out, "+ "+l)
		}
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

// TestPlanDeterministic: two plans of one unchanged project have one plan_id.
func TestPlanDeterministic(t *testing.T) {
	f := loadFixture(t, "prod-plugins")
	a, b := planOf(t, f, nil), planOf(t, f, nil)
	if a.PlanID != b.PlanID || len(a.PlanID) != 64 {
		t.Fatalf("plan ids differ or are malformed: %s %s", a.PlanID, b.PlanID)
	}
}

// TestPlanApplyProperty: for each fixture, the files an apply changes are
// exactly the plan's artifacts, and a plan after the apply is empty.
func TestPlanApplyProperty(t *testing.T) {
	skip := func(p string) bool { return p == ".nself/op.lock" || strings.HasPrefix(p, ".nself/backups/") }
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			f := loadFixture(t, name)
			p := planOf(t, f, nil)
			before := fileHashes(t, f.project)
			applyFixture(t, f)
			after := fileHashes(t, f.project)
			changed := map[string]bool{}
			for k, v := range after {
				if before[k] != v && !skip(k) {
					changed[k] = true
				}
			}
			for k := range before {
				if _, ok := after[k]; !ok && !skip(k) {
					changed[k] = true
				}
			}
			planned := map[string]bool{}
			for _, a := range p.Artifacts {
				planned[a.Path] = true
			}
			if fmt.Sprint(sortedKeys(changed)) != fmt.Sprint(sortedKeys(planned)) {
				t.Fatalf("apply changed %v, plan listed %v", sortedKeys(changed), sortedKeys(planned))
			}
			again := planOf(t, f, nil)
			if !again.Empty {
				var sb strings.Builder
				_ = RenderHuman(&sb, *again)
				t.Fatalf("plan after apply is not empty:\n%s", sb.String())
			}
		})
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestContainerImpactEnvValue: with a running stack, changing an env value that
// feeds a service's environment plans recreate for that service (and not for
// one it does not feed); a change to a plugin fragment plans recreate for the
// plugin service; nothing changed plans nothing.
func TestContainerImpactEnvValue(t *testing.T) {
	f := loadFixture(t, "prod-plugins")
	applyFixture(t, f)
	rt := &fakeRuntime{running: runningFromDisk(t, f)}
	plan := func() *Plan {
		return planOf(t, f, func(r *Request) { r.Containers, r.Runtime = true, rt })
	}
	base := plan()
	if !base.Containers.Known || len(base.Containers.Items) != 0 || !base.Empty {
		t.Fatalf("unchanged project: known=%v items=%v empty=%v", base.Containers.Known, base.Containers.Items, base.Empty)
	}
	if len(rt.files) < 2 || !strings.Contains(strings.Join(rt.files, " "), "docker-compose.plugin.yml") {
		t.Fatalf("hash invocation did not include the plugin fragment: %v", rt.files)
	}
	if rt.dir != f.project {
		t.Fatalf("--project-directory = %q, want %q", rt.dir, f.project)
	}
	setEnvValue(t, f, "HASURA_GRAPHQL_ADMIN_SECRET", "Zq7Wx3Tn9Rk5Hb2Vc8Lm4Pd6Sa1FgJu0Ye8")
	p := plan()
	it, ok := itemFor(p, "hasura")
	if !ok || it.Action != ContainerRecreate || it.AppliedBy != AppliedNextStart {
		t.Fatalf("hasura after the secret changed: %+v (items %+v)", it, p.Containers.Items)
	}
	if _, ok := itemFor(p, "postgres"); ok {
		t.Fatalf("postgres does not use that value but is planned: %+v", p.Containers.Items)
	}
	if p.Empty || !p.RequiresConfirmation {
		t.Fatalf("plan empty=%v requires_confirmation=%v", p.Empty, p.RequiresConfirmation)
	}
	// A postgres password feeds postgres, a stateful (named volume) service.
	setEnvValue(t, f, "HASURA_GRAPHQL_ADMIN_SECRET", fixtureValue(t, "HASURA_GRAPHQL_ADMIN_SECRET"))
	setEnvValue(t, f, "POSTGRES_PASSWORD", "Vb3Nc7Xz9Wt5Kq2Hy8Zp4Lm6Jd1Sa0Fe")
	pg, ok := itemFor(plan(), "postgres")
	if !ok || pg.Action != ContainerRecreate || !pg.Stateful {
		t.Fatalf("postgres after its password changed: %+v", pg)
	}
	setEnvValue(t, f, "POSTGRES_PASSWORD", fixtureValue(t, "POSTGRES_PASSWORD"))
	// A plugin fragment change.
	frag := filepath.Join(f.plugins, "nself-alpha", "docker-compose.plugin.yml")
	raw, err := os.ReadFile(frag)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(raw), `"3901:3901"`, `"3999:3901"`, 1)
	if changed == string(raw) {
		t.Fatalf("fragment has no 3901 port line:\n%s", raw)
	}
	writeFile(t, frag, changed, 0o644)
	pa, ok := itemFor(plan(), "nself-alpha")
	if !ok || pa.Action != ContainerRecreate {
		t.Fatalf("plugin service after its fragment changed: %+v", pa)
	}
}

// fixtureValue returns the value of key in the shared fixture secrets.
func fixtureValue(t *testing.T, key string) string {
	t.Helper()
	m, err := godotenv.Read(filepath.Join("testdata", "fixtures", "secrets.env"))
	if err != nil || m[key] == "" {
		t.Fatalf("fixture secret %s: %v", key, err)
	}
	return m[key]
}

// TestContainerImpactUnknown: a daemon that cannot be asked, a compose that
// fails, and a container without the label are reported, never guessed.
func TestContainerImpactUnknown(t *testing.T) {
	f := loadFixture(t, "prod-plugins")
	applyFixture(t, f)
	var notes strings.Builder
	for _, rt := range []*fakeRuntime{{runErr: errors.New("daemon down")}, {hashErr: errors.New("compose too old")}} {
		notes.Reset()
		p := planOf(t, f, func(r *Request) { r.Containers, r.Runtime, r.Stderr = true, rt, &notes })
		if p.Containers.Known || len(p.Containers.Items) != 0 || !strings.Contains(notes.String(), "container impact unknown") {
			t.Fatalf("known=%v items=%v notice=%q", p.Containers.Known, p.Containers.Items, notes.String())
		}
	}
	running := runningFromDisk(t, f)
	running[0].ConfigHash = ""
	p := planOf(t, f, func(r *Request) { r.Containers, r.Runtime = true, &fakeRuntime{running: running} })
	if it, ok := itemFor(p, running[0].Service); !ok || it.Action != ContainerUnknown {
		t.Fatalf("a container without a hash label: %+v", p.Containers.Items)
	}
}

// TestPlanRemoveOrphans: a running container whose service the planned compose
// does not define is an orphan-remove effect only when asked, and destructive.
func TestPlanRemoveOrphans(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	applyFixture(t, f)
	running := append(runningFromDisk(t, f), RunningContainer{Name: "fx_ghost", Service: "ghost", State: "running"})
	rt := &fakeRuntime{running: running}
	without := planOf(t, f, func(r *Request) { r.Containers, r.Runtime = true, rt })
	if !without.Empty {
		t.Fatalf("an orphan alone must not change the plan unless removal is asked: %+v", without.Effects)
	}
	with := planOf(t, f, func(r *Request) { r.Containers, r.Runtime, r.RemoveOrphans = true, rt, true })
	if len(with.Effects) != 1 || with.Effects[0].Kind != EffectOrphanRemove || with.Effects[0].Target != "fx_ghost" || !with.Destructive {
		t.Fatalf("effects %+v destructive=%v", with.Effects, with.Destructive)
	}
}

// TestComputeDiffOut: the unified diffs go to DiffOut, a removal shows its
// lines, and no env value appears.
func TestComputeDiffOut(t *testing.T) {
	f := loadFixture(t, "staging-hand")
	var diff strings.Builder
	p := planOf(t, f, func(r *Request) { r.DiffOut = &diff })
	d := diff.String()
	for _, want := range []string{"--- a/nginx/sites/stale.conf", "--- a/.env.computed", "[REDACTED]"} {
		if !strings.Contains(d, want) {
			t.Errorf("diff lacks %q", want)
		}
	}
	for _, secret := range []string{fixtureValue(t, "POSTGRES_PASSWORD"), fixtureValue(t, "HASURA_GRAPHQL_ADMIN_SECRET")} {
		if strings.Contains(d, secret) {
			t.Errorf("the diff leaks an env value")
		}
	}
	var removed bool
	for _, a := range p.Artifacts {
		removed = removed || (a.Path == "nginx/sites/stale.conf" && a.Action == ActionRemove)
	}
	if !removed {
		t.Fatalf("the stale generated conf is not planned for removal: %+v", p.Artifacts)
	}
	var bkp bool
	for _, e := range p.Effects {
		bkp = bkp || e.Kind == EffectNginxSitesBkp
	}
	if !bkp {
		t.Fatal("a build that removes a site conf must list the nginx/sites snapshot")
	}
}

// TestComputeErrors: failures are returned, never turned into an empty plan.
func TestComputeErrors(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	if _, err := Compute(context.Background(), Request{ProjectDir: f.project, Build: nbuild.BuildOptions{Check: true}}); err == nil {
		t.Fatal("--check has no plan; Compute must say so")
	}
	setEnvValue(t, f, "POSTGRES_PASSWORD", "password")
	if p, err := Compute(context.Background(), Request{ProjectDir: f.project}); err == nil {
		t.Fatalf("a project that fails validation must fail, got a plan: %+v", p)
	}
}

// dockerStub puts a `docker` script first on PATH. ps prints the given lines;
// `compose ... config --hash` prints the given hashes.
func dockerStub(t *testing.T, ps, hashes string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in\n  ps) printf '%s' \"$STUB_PS\"; exit 0 ;;\n  compose) printf '%s' \"$STUB_HASH\"; exit 0 ;;\nesac\nexit 1\n"
	writeFile(t, filepath.Join(dir, "docker"), script, 0o755)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("STUB_PS", ps)
	t.Setenv("STUB_HASH", hashes)
}

// TestDefaultRuntimeUsesDocker: with no injected runtime the plan asks the
// docker CLI (here a stub) and plans recreate from its answers.
func TestDefaultRuntimeUsesDocker(t *testing.T) {
	f := loadFixture(t, "prod-plugins")
	applyFixture(t, f)
	dockerStub(t, "fx_hasura\trunning\thasura\told\nfx_nginx\trunning\tnginx\tsame\n", "hasura new\nnginx same\n")
	p := planOf(t, f, func(r *Request) { r.Containers = true })
	if !p.Containers.Known || len(p.Containers.Items) != 1 || p.Containers.Items[0].Service != "hasura" {
		t.Fatalf("containers: %+v", p.Containers)
	}
}

// TestShadowInvocationFiles: the user override is last in the file list, a
// manifest line for a file that no longer exists is dropped, and the planned
// env files are passed.
func TestShadowInvocationFiles(t *testing.T) {
	f := loadFixture(t, "dev-plugin")
	writeFile(t, filepath.Join(f.project, "docker-compose.override.yml"), "services:\n  nself-alpha:\n    restart: always\n", 0o644)
	applyFixture(t, f)
	frag := filepath.Join(f.plugins, "nself-alpha", "docker-compose.plugin.yml")
	rt := &fakeRuntime{}
	planOf(t, f, func(r *Request) { r.Containers, r.Runtime = true, rt })
	if n := len(rt.files); n != 3 || !strings.HasSuffix(rt.files[2], "docker-compose.override.yml") || !strings.HasSuffix(rt.files[1], "docker-compose.plugin.yml") {
		t.Fatalf("files %v", rt.files)
	}
	if len(rt.envFiles) != 2 {
		t.Fatalf("env files %v", rt.envFiles)
	}
	if err := os.Remove(frag); err != nil {
		t.Fatal(err)
	}
	planOf(t, f, func(r *Request) { r.Containers, r.Runtime = true, rt })
	for _, file := range rt.files {
		if strings.HasSuffix(file, "docker-compose.plugin.yml") {
			t.Fatalf("a fragment that no longer exists reached compose: %v", rt.files)
		}
	}
}

// failWriter fails every write.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

// TestContainerImpactBrokenCompose: a user override that is not valid YAML makes
// the impact unknown (with a notice), not an error and not "no impact".
func TestContainerImpactBrokenCompose(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	writeFile(t, filepath.Join(f.project, "docker-compose.override.yml"), "services: [unterminated", 0o644)
	var notes strings.Builder
	p := planOf(t, f, func(r *Request) { r.Containers, r.Runtime, r.Stderr = true, &fakeRuntime{noHash: true}, &notes })
	if p.Containers.Known || !strings.Contains(notes.String(), "container impact unknown") {
		t.Fatalf("known=%v notice=%q", p.Containers.Known, notes.String())
	}
}

// TestComputeWriteFailures: a diff or summary that cannot be written is an error.
func TestComputeWriteFailures(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	if _, err := Compute(context.Background(), Request{ProjectDir: f.project, DiffOut: failWriter{}}); err == nil {
		t.Fatal("a failing DiffOut must fail the plan")
	}
	g := loadFixture(t, "prod-ssl")
	before := treeHash(t, g.root)
	compattest.Set(t, true)
	if _, err := Apply(context.Background(), Request{ProjectDir: g.project, Stderr: failWriter{}}, ApplyOptions{Yes: true}); err == nil {
		t.Fatal("a failing summary writer must fail the apply before it writes")
	}
	if after := treeHash(t, g.root); after != before {
		t.Fatalf("an apply that could not show its summary wrote:\n%s", lineDiff(before, after))
	}
}

// TestOverlay: planned bytes win, a planned removal hides the disk, the fronting
// prefix and absolute keys resolve to their own roots, and a relative manifest
// line is read from the project.
func TestOverlay(t *testing.T) {
	dir, fronting, outside := t.TempDir(), t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(dir, "on-disk.txt"), "disk", 0o644)
	writeFile(t, filepath.Join(dir, "gone.txt"), "disk", 0o644)
	writeFile(t, filepath.Join(fronting, "sites", "a.conf"), "fronting", 0o644)
	writeFile(t, filepath.Join(outside, "frag.yml"), "outside", 0o644)
	pb := &nbuild.PlannedBuild{Files: map[string]nbuild.PlannedFile{"planned.txt": {Data: []byte("planned")}}, Removed: []string{"gone.txt"}}
	ov := newOverlay(dir, fronting, pb)
	want := map[string]string{"planned.txt": "planned", "on-disk.txt": "disk", FrontingPrefix + "sites/a.conf": "fronting", filepath.ToSlash(filepath.Join(outside, "frag.yml")): "outside"}
	for key, body := range want {
		got, ok, err := ov.read(key)
		if err != nil || !ok || string(got) != body {
			t.Errorf("read(%q) = %q, %v, %v; want %q", key, got, ok, err, body)
		}
	}
	for _, key := range []string{"gone.txt", "never.txt"} {
		if _, ok, err := ov.read(key); ok || err != nil {
			t.Errorf("read(%q) must be absent without error: %v %v", key, ok, err)
		}
	}
	// Reading through a file is ENOTDIR on unix (an error) but ENOENT on Windows.
	if _, _, err := ov.read("on-disk.txt/child"); err == nil && runtime.GOOS != "windows" {
		t.Error("a read through a file must be an error, not an absent file")
	}
	if got, ok, _ := ov.readAbs(filepath.Join(dir, "planned.txt")); !ok || string(got) != "planned" {
		t.Errorf("readAbs of a project path = %q %v", got, ok)
	}
	shadow := t.TempDir()
	writeFile(t, filepath.Join(dir, ".nself", "compose-files.txt"), "\non-disk.txt\nmissing.yml\n"+filepath.Join(outside, "frag.yml")+"\n", 0o644)
	files, _, err := shadowInvocation(shadow, dir, ov)
	if err != nil || len(files) != 2 {
		t.Fatalf("files %v, err %v (a relative line is read from the project, a missing one is dropped)", files, err)
	}
}
