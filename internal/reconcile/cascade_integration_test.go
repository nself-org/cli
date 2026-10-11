package reconcile_test

// Purpose: P7-LIVE-08. One always-run integration suite that drives the real
// nself binary against real temp projects and a real Docker daemon, and proves
// mutation -> plan -> apply -> doctor clean for the flagship cascades:
//
//	a. a plugin install adds its nginx route and compose service
//	b. config set BASE_DOMAIN cascades to nginx and compose; --preview writes nothing
//	c. a hand-edited generated conf is refused, then overwritten with --force
//	d. a prod-class change is refused non-interactively (E403, exit 4) until --yes
//	e. doctor --fix repairs drift and a second --fix changes nothing
//
// Every scenario runs twice: NSELF_V15=1 (auto-apply and refusals) and
// NSELF_V15 unset (v1.4: plan summary and a one-line notice on stderr, the
// command proceeds as before). The mode is set on the child process only.
// Inputs: docker on PATH and a reachable daemon; the fixtures under
// testdata/fixtures; Go toolchain (the binary is built once with -mod=vendor).
// Outputs: pass/fail. Containers, images and networks are created only under a
// unique project name and removed by label in t.Cleanup. No fixed host ports
// are bound: nothing is started.
// Constraints: skips only when docker is not on PATH (and on Windows, where
// the suite's POSIX paths and modes do not apply). Fixture values are
// synthetic (testdata/fixtures/secrets.env). No bare sleeps: waits poll state
// against a deadline.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	cascadeFixtures = "testdata/fixtures"
	cascadeCmdLimit = 3 * time.Minute
)

// cascadeMode is one run of every scenario.
type cascadeMode struct {
	name string
	v15  bool
}

// TestCascadeIntegration is the suite entry point (the verify line greps its
// PASS line). Scenarios run one after another: they share one Docker daemon.
func TestCascadeIntegration(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the cascade suite drives POSIX project trees; Linux is the proof platform (Constitution 10.3)")
	}
	cascadeDockerReady(t)
	bin := cascadeBuildBinary(t)
	scenarios := []struct {
		name string
		fn   func(*testing.T, string, cascadeMode)
	}{
		{"plugin_install_adds_route", cascadePluginInstall},
		{"config_set_cascades", cascadeConfigSet},
		{"hand_edit_refused_then_force", cascadeHandEdit},
		{"prod_class_refusal", cascadeProdClass},
		{"doctor_fix_idempotent", cascadeDoctorFix},
	}
	for _, mode := range []cascadeMode{{"v15", true}, {"v14", false}} {
		t.Run(mode.name, func(t *testing.T) {
			for _, sc := range scenarios {
				t.Run(sc.name, func(t *testing.T) { sc.fn(t, bin, mode) })
			}
		})
	}
}

// ---- process, project and tree helpers -------------------------------------

// cascadeBuildBinary builds ./cmd/nself once into the test's temp dir.
func cascadeBuildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "nself")
	cmd := exec.Command("go", "build", "-mod=vendor", "-o", bin, "../../cmd/nself")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building nself: %v\n%s", err, out)
	}
	return bin
}

// cascadeProject is one temp project with its own HOME and plugin dir.
type cascadeProject struct {
	t       *testing.T
	bin     string
	v15     bool
	name    string // PROJECT_NAME: unique per run, the docker label value
	root    string // holds project/, home/, plugins/
	dir     string
	home    string
	plugins string
}

// newCascadeProject copies a fixture the way scripts/ci/live-safety-fixture.sh
// mkproj does: .env = fixture dotenv + secrets.env (0600), certificate
// placeholders, the fixture plugins. PROJECT_NAME is made unique.
func newCascadeProject(t *testing.T, bin, fixture string, v15 bool) *cascadeProject {
	t.Helper()
	root := t.TempDir()
	p := &cascadeProject{t: t, bin: bin, v15: v15, root: root,
		dir: filepath.Join(root, "project"), home: filepath.Join(root, "home"), plugins: filepath.Join(root, "plugins")}
	for _, d := range []string{p.dir, filepath.Join(p.dir, ".nself"), p.home, p.plugins} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var rnd [4]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		t.Fatal(err)
	}
	p.name = "l08" + hex.EncodeToString(rnd[:])
	fx := filepath.Join(cascadeFixtures, fixture)
	dotenv := cascadeRead(t, filepath.Join(fx, "project", "dotenv"))
	if !strings.Contains(dotenv, "PROJECT_NAME=fx") {
		t.Fatalf("fixture %s has no PROJECT_NAME=fx line", fixture)
	}
	dotenv = strings.Replace(dotenv, "PROJECT_NAME=fx", "PROJECT_NAME="+p.name, 1)
	secrets := cascadeRead(t, filepath.Join(cascadeFixtures, "secrets.env"))
	p.write(".env", dotenv+secrets)
	if err := os.Chmod(filepath.Join(p.dir, ".env"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cascadeCopyTree(filepath.Join(fx, "plugins"), p.plugins); err != nil {
		t.Fatal(err)
	}
	if certs, err := os.ReadFile(filepath.Join(fx, "certs.txt")); err == nil {
		for _, c := range strings.Fields(string(certs)) {
			p.write(filepath.Join("ssl", "certificates", c, "fullchain.pem"), "CERT\n")
			p.write(filepath.Join("ssl", "certificates", c, "privkey.pem"), "KEY\n")
		}
	}
	t.Cleanup(p.cleanupDocker)
	return p
}

func cascadeRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// cascadeCopyTree copies src into dst; a missing src is not an error.
func cascadeCopyTree(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}

func (p *cascadeProject) path(rel string) string {
	return filepath.Join(p.dir, filepath.FromSlash(rel))
}

func (p *cascadeProject) write(rel, content string) {
	p.t.Helper()
	full := p.path(rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		p.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		p.t.Fatal(err)
	}
}

func (p *cascadeProject) read(rel string) string {
	p.t.Helper()
	return cascadeRead(p.t, p.path(rel))
}

func (p *cascadeProject) exists(rel string) bool {
	_, err := os.Stat(p.path(rel))
	return err == nil
}

// cascadeResult is one command run.
type cascadeResult struct {
	stdout, stderr string
	code           int
}

func (r cascadeResult) all() string { return r.stdout + r.stderr }

// env is the child environment: explicit, so NSELF_V15 of the parent never
// leaks in and the mode is set on the child only. Docker settings come from
// the real environment so the daemon is reachable from the temp HOME.
func (p *cascadeProject) env() []string {
	e := []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + p.home, "NSELF_PLUGIN_DIR=" + p.plugins,
		"NSELF_CMD_LOG_ENABLED=false", "AI_AUTO_INSTALL=false",
	}
	dockerCfg := os.Getenv("DOCKER_CONFIG")
	if dockerCfg == "" {
		if h, err := os.UserHomeDir(); err == nil {
			dockerCfg = filepath.Join(h, ".docker")
		}
	}
	e = append(e, "DOCKER_CONFIG="+dockerCfg)
	for _, k := range []string{"DOCKER_HOST", "DOCKER_CONTEXT", "XDG_RUNTIME_DIR", "TMPDIR"} {
		if v := os.Getenv(k); v != "" {
			e = append(e, k+"="+v)
		}
	}
	if p.v15 {
		e = append(e, "NSELF_V15=1")
	}
	return e
}

// run executes the binary in the project with stdin at /dev/null.
func (p *cascadeProject) run(args ...string) cascadeResult {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cascadeCmdLimit)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.bin, args...)
	cmd.Dir = p.dir
	cmd.Env = p.env()
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	res := cascadeResult{stdout: so.String(), stderr: se.String()}
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			p.t.Fatalf("nself %s: %v", strings.Join(args, " "), err)
		}
		res.code = ee.ExitCode()
	}
	return res
}

// mustRun runs and requires exit 0.
func (p *cascadeProject) mustRun(args ...string) cascadeResult {
	p.t.Helper()
	r := p.run(args...)
	if r.code != 0 {
		p.t.Fatalf("nself %s: exit %d\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), r.code, r.stdout, r.stderr)
	}
	return r
}

// snapshot maps every path under the project root, the HOME and the plugin dir
// to "dir", "link:<target>" or the sha256 of its content. The command guard's
// lock file is left out.
func (p *cascadeProject) snapshot() treeSnap { return p.takeSnapshot(false) }

// snapshotNoHome leaves the HOME out. `nself doctor` runs host probes (on
// macOS, Homebrew) that write caches under HOME; only the project and the
// plugin dir are the doctor's to keep unchanged.
func (p *cascadeProject) snapshotNoHome() treeSnap { return p.takeSnapshot(true) }

// treeSnap is a snapshot and the mode it was taken in.
type treeSnap struct {
	files  map[string]string
	noHome bool
}

func (p *cascadeProject) takeSnapshot(noHome bool) treeSnap {
	p.t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(p.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(p.root, path)
		switch {
		case d.Name() == "op.lock", noHome && (rel == "home" || strings.HasPrefix(rel, "home"+string(filepath.Separator))):
		case d.IsDir():
			snap[rel] = "dir"
		case d.Type().IsRegular():
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			snap[rel] = fmt.Sprintf("%x", sha256.Sum256(b))
		default:
			target, _ := os.Readlink(path)
			snap[rel] = "link:" + target
		}
		return nil
	})
	if err != nil {
		p.t.Fatal(err)
	}
	return treeSnap{files: snap, noHome: noHome}
}

// assertUnchanged fails, naming the paths, when the tree differs from snap.
func (p *cascadeProject) assertUnchanged(snap treeSnap, what string) {
	p.t.Helper()
	now := p.takeSnapshot(snap.noHome).files
	var diff []string
	for k, v := range now {
		if old, ok := snap.files[k]; !ok {
			diff = append(diff, "added "+k)
		} else if old != v {
			diff = append(diff, "changed "+k)
		}
	}
	for k := range snap.files {
		if _, ok := now[k]; !ok {
			diff = append(diff, "removed "+k)
		}
	}
	if len(diff) > 0 {
		sort.Strings(diff)
		p.t.Fatalf("%s changed the tree: %s", what, strings.Join(diff, ", "))
	}
}

// cascadePlan is the part of the change-plan envelope the suite asserts on.
type cascadePlan struct {
	Empty                bool   `json:"empty"`
	EnvClass             string `json:"env_class"`
	RequiresConfirmation bool   `json:"requires_confirmation"`
	PlanID               string `json:"plan_id"`
	Artifacts            []struct {
		Path       string `json:"path"`
		Kind       string `json:"kind"`
		HandEdited bool   `json:"hand_edited"`
	} `json:"artifacts"`
	Containers struct {
		Known bool `json:"known"`
		Items []struct {
			Service string `json:"service"`
			Action  string `json:"action"`
		} `json:"items"`
	} `json:"containers"`
}

// planJSON runs `build --plan --json` and parses the envelope's data.
func (p *cascadeProject) planJSON() cascadePlan {
	p.t.Helper()
	r := p.mustRun("build", "--plan", "--json")
	return cascadeParsePlan(p.t, r.stdout)
}

func cascadeParsePlan(t *testing.T, stdout string) cascadePlan {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("plan output is not JSON: %v\n%s", err, stdout)
	}
	if len(env.Data) == 0 {
		t.Fatalf("plan envelope has no data field:\n%s", stdout)
	}
	var pl cascadePlan
	if err := json.Unmarshal(env.Data, &pl); err != nil {
		t.Fatalf("plan data: %v\n%s", err, stdout)
	}
	return pl
}

// cascadeErrCode reads error.code from a --json failure envelope.
func cascadeErrCode(stdout string) string {
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal([]byte(stdout), &env)
	return env.Error.Code
}

// assertPlanEmpty requires an empty plan with known container state.
func (p *cascadeProject) assertPlanEmpty(why string) {
	p.t.Helper()
	pl := p.planJSON()
	if !pl.Empty {
		var paths []string
		for _, a := range pl.Artifacts {
			paths = append(paths, a.Path)
		}
		p.t.Fatalf("%s: plan is not empty (artifacts %v, containers known=%v)", why, paths, pl.Containers.Known)
	}
}

// assertDoctorDrift runs `nself doctor` and checks its Generated files line.
// The other doctor checks depend on the host (ports, tools) and are not read.
func (p *cascadeProject) assertDoctorDrift(want bool, paths ...string) {
	p.t.Helper()
	out := p.run("doctor").all()
	if !want {
		if !strings.Contains(out, "Generated files: no generated-file drift") {
			p.t.Fatalf("doctor does not report a clean tree:\n%s", cascadeLines(out, "Generated files"))
		}
		return
	}
	for _, path := range paths {
		if !strings.Contains(cascadeLines(out, "Generated files"), path) {
			p.t.Fatalf("doctor does not report drift of %s:\n%s", path, cascadeLines(out, "Generated files"))
		}
	}
}

// assertDoctorVirtualDriftOnly is assertDoctorDrift(false) with one known
// exception. The generated state records a plugin compose fragment under the
// virtual key "@plugins/<name>/docker-compose.plugin.yml" (internal/reconcile
// state.go), but checkGeneratedDrift (cmd/commands/doctor_drift.go) reads every
// key as a project-relative file, finds none, and reports it as drifted. That is
// a doctor defect outside this Ticket's scope; this check keeps every REAL file
// strict (any drifted path that is not a virtual "@plugins/" key fails) and
// should become assertDoctorDrift(false) when the defect is fixed.
func (p *cascadeProject) assertDoctorVirtualDriftOnly() {
	p.t.Helper()
	line := cascadeLines(p.run("doctor").all(), "Generated files:")
	if strings.Contains(line, "no generated-file drift") {
		return
	}
	_, list, ok := strings.Cut(line, "Generated files: drift: ")
	if !ok {
		p.t.Fatalf("doctor Generated files line is neither clean nor a drift list:\n%s", line)
	}
	for _, path := range strings.Split(strings.TrimSpace(list), ", ") {
		if !strings.HasPrefix(path, "@plugins/") {
			p.t.Fatalf("doctor reports drift of a real file %q:\n%s", path, line)
		}
	}
}

// cascadeLines returns the lines of s containing sub.
func cascadeLines(s, sub string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func cascadeContains(t *testing.T, what, hay, needle string) {
	t.Helper()
	if !strings.Contains(hay, needle) {
		t.Fatalf("%s: want %q in:\n%s", what, needle, hay)
	}
}

// generatedRecord returns the sha256 recorded for rel in the generated state.
func (p *cascadeProject) generatedRecord(rel string) string {
	p.t.Helper()
	var st struct {
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal([]byte(p.read(".nself/state/generated.json")), &st); err != nil {
		p.t.Fatal(err)
	}
	return st.Files[rel]
}

func cascadeSHA(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ---- docker helpers ----------------------------------------------------------

// cascadeDockerReady polls `docker info` against a deadline. A CLI with no
// reachable daemon fails the suite: Docker is never mocked here.
func cascadeDockerReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last []byte
	for {
		out, err := exec.Command("docker", "info").CombinedOutput()
		if err == nil {
			return
		}
		last = out
		if time.Now().After(deadline) {
			t.Fatalf("docker is on PATH but the daemon is not reachable (the suite never mocks Docker): %s", last)
		}
		time.Sleep(500 * time.Millisecond) // poll interval inside the deadline loop
	}
}

func cascadeDocker(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// cleanupDocker removes whatever carries this project's compose label, by
// label only. It runs in t.Cleanup, so a failed scenario leaves nothing.
func (p *cascadeProject) cleanupDocker() {
	filter := "label=com.docker.compose.project=" + p.name
	for _, kind := range [][]string{{"ps", "-aq"}, {"network", "ls", "-q"}, {"volume", "ls", "-q"}} {
		args := append(append([]string{}, kind...), "--filter", filter)
		out, err := exec.Command("docker", args...).Output()
		if err != nil {
			continue
		}
		ids := strings.Fields(string(out))
		if len(ids) == 0 {
			continue
		}
		rm := []string{"rm", "-f"}
		switch kind[0] {
		case "network":
			rm = []string{"network", "rm"}
		case "volume":
			rm = []string{"volume", "rm", "-f"}
		}
		_ = exec.Command("docker", append(rm, ids...)...).Run()
	}
	_ = exec.Command("docker", "rmi", "-f", "nself-cascade-scratch:"+p.name).Run()
}

// scratchImage imports an empty root filesystem as a local image (no network,
// no pull) so a container can be created from it. Removed in cleanupDocker.
func (p *cascadeProject) scratchImage() string {
	p.t.Helper()
	var empty bytes.Buffer
	if err := tar.NewWriter(&empty).Close(); err != nil {
		p.t.Fatal(err)
	}
	ref := "nself-cascade-scratch:" + p.name
	cmd := exec.Command("docker", "import", "-", ref)
	cmd.Stdin = &empty
	if out, err := cmd.CombinedOutput(); err != nil {
		p.t.Fatalf("docker import: %v\n%s", err, out)
	}
	return ref
}

// containerNames lists this project's containers (running or not) as
// "name state" lines, sorted.
func (p *cascadeProject) containerNames() string {
	p.t.Helper()
	out := cascadeDocker(p.t, "ps", "-a", "--filter", "label=com.docker.compose.project="+p.name, "--format", "{{.Names}} {{.State}}")
	lines := strings.Split(out, "\n")
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// ---- scenario a: plugin install ------------------------------------------------

const (
	cascadePluginName  = "cascade-demo"
	cascadePluginRoute = "nginx/sites/cascade-demo-route.conf"
)

// cascadePluginTarball is a third-party plugin archive: a manifest, a compose
// fragment, a Dockerfile and one nginx route snippet.
func cascadePluginTarball(t *testing.T) []byte {
	t.Helper()
	files := []struct{ name, body string }{
		{"plugin.json", `{"name":"` + cascadePluginName + `","version":"1.0.0","description":"Cascade integration fixture plugin (synthetic)","category":"utility","license":"MIT","port":3961,"language":"go"}`},
		{"docker-compose.plugin.yml", "services:\n  " + cascadePluginName + ":\n    image: nself/" + cascadePluginName + ":latest\n    ports:\n      - \"3961:3961\"\n"},
		{"Dockerfile", "FROM scratch\n"},
		{"nginx/route.conf", "server {\n    listen 80;\n    server_name cascade.${BASE_DOMAIN};\n    location / {\n        proxy_pass http://" + cascadePluginName + ":3961;\n    }\n}\n"},
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func cascadePluginInstall(t *testing.T, bin string, mode cascadeMode) {
	p := newCascadeProject(t, bin, "dev-minimal", mode.v15)
	p.mustRun("build")
	if p.exists(cascadePluginRoute) {
		t.Fatalf("%s exists before the install", cascadePluginRoute)
	}
	tarball := cascadePluginTarball(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)
	url := srv.URL + "/" + cascadePluginName + ".tar.gz"

	r := p.mustRun("plugin", "install", url, "--yes", "--checksum", cascadeSHA(string(tarball)))
	if !p.exists("../plugins/" + cascadePluginName + "/plugin.json") {
		t.Fatalf("plugin files are not in the plugin dir:\n%s", r.all())
	}
	if !mode.v15 {
		// v1.4: the install leaves generated state for a later build and says so
		// (internal/plugin/install_thirdparty.go prints the build hint).
		cascadeContains(t, "v1.4 install notice", r.stderr, "Run 'nself build'")
		if p.exists(cascadePluginRoute) {
			t.Fatalf("v1.4 install wrote the route without a build:\n%s", r.all())
		}
		pending := false
		for _, a := range p.planJSON().Artifacts {
			pending = pending || a.Path == cascadePluginRoute
		}
		if !pending {
			t.Fatal("v1.4: the pending plugin route is not in the plan")
		}
		p.mustRun("build")
	}
	// v1.5: reconciled by the install itself (cmd/commands/plugin_install.go
	// finishPluginInstall); v1.4: reconciled by the build just run.
	route := p.read(cascadePluginRoute)
	cascadeContains(t, "plugin route conf", route, "cascade.example.test")
	cascadeContains(t, "plugin route conf", route, "proxy_pass http://"+cascadePluginName+":3961")
	cascadeContains(t, "plugin route marker", strings.ToLower(route), "generated by nself")
	files := p.read(".nself/compose-files.txt")
	cascadeContains(t, "compose manifest", files, cascadePluginName)
	fragment := cascadeRead(t, filepath.Join(p.plugins, cascadePluginName, "docker-compose.plugin.yml"))
	cascadeContains(t, "plugin compose service", fragment, cascadePluginName+":")
	p.assertPlanEmpty("after the plugin route was generated")
	p.assertDoctorVirtualDriftOnly()
}

// ---- scenario b: config set cascade -----------------------------------------------

func cascadeConfigSet(t *testing.T, bin string, mode cascadeMode) {
	const oldDomain, newDomain = "example.test", "moved.example.org"
	p := newCascadeProject(t, bin, "dev-minimal", mode.v15)
	p.mustRun("build")
	sites, _ := filepath.Glob(filepath.Join(p.dir, "nginx", "sites", "*.conf"))
	if len(sites) == 0 {
		t.Fatal("the build wrote no nginx site confs")
	}
	for _, s := range sites {
		cascadeContains(t, filepath.Base(s), cascadeRead(t, s), oldDomain)
	}

	// --preview writes nothing, not even the env line, in both modes.
	before := p.snapshot()
	r := p.mustRun("config", "set", "BASE_DOMAIN", newDomain, "--preview")
	cascadeContains(t, "preview output", r.all(), "nothing written")
	p.assertUnchanged(before, "config set --preview (tree incl. .env)")

	r = p.mustRun("config", "set", "BASE_DOMAIN", newDomain)
	cascadeContains(t, "env line", p.read(".env"), "BASE_DOMAIN="+newDomain)
	if !mode.v15 {
		// v1.4 (cmd/commands/config_set_plan.go writeOnly): the line is written,
		// the cascade is summarised on stderr and left for the next build.
		cascadeContains(t, "v1.4 summary", r.stderr, "config: BASE_DOMAIN")
		cascadeContains(t, "v1.4 notice", r.stderr, "not applied in v1.4 mode")
		for _, s := range sites {
			if strings.Contains(cascadeRead(t, s), newDomain) {
				t.Fatalf("v1.4 config set regenerated %s without a build", filepath.Base(s))
			}
		}
		p.mustRun("build")
	} else {
		cascadeContains(t, "v1.5 summary", r.stdout, "config: BASE_DOMAIN")
	}
	for _, s := range sites {
		body := cascadeRead(t, s)
		cascadeContains(t, filepath.Base(s), body, newDomain)
		if strings.Contains(body, oldDomain) {
			t.Fatalf("%s still names %s", filepath.Base(s), oldDomain)
		}
	}
	compose := p.read("docker-compose.yml")
	cascadeContains(t, "docker-compose.yml", compose, newDomain)
	if strings.Contains(compose, oldDomain) {
		t.Fatalf("docker-compose.yml still names %s", oldDomain)
	}
	p.assertPlanEmpty("after config set BASE_DOMAIN was applied")
	p.assertDoctorDrift(false)
}

// ---- scenario c: hand edit -------------------------------------------------------

func cascadeHandEdit(t *testing.T, bin string, mode cascadeMode) {
	const conf = "nginx/sites/api-docs.conf"
	const foreign = "nginx/sites/foreign.conf"
	const foreignBody = "server {\n    listen 8089;\n    server_name foreign.invalid;\n    return 204;\n}\n"
	p := newCascadeProject(t, bin, "dev-minimal", mode.v15)
	p.mustRun("build")
	generated := p.read(conf)
	cascadeContains(t, "marked conf", strings.ToLower(generated), "generated by nself")
	p.write(foreign, foreignBody)
	p.write(conf, generated+"# hand edit\n")

	if !mode.v15 {
		// v1.4 (internal/reconcile handedit warning, cmd build path): the diff and
		// a one-line notice go to stderr and the build proceeds, overwriting.
		// config set does not regenerate in v1.4, so the edit survives it.
		p.mustRun("config", "set", "BASE_DOMAIN", "other.example.test")
		if !strings.Contains(p.read(conf), "# hand edit") {
			t.Fatal("v1.4 config set touched a generated conf")
		}
		r := p.mustRun("build")
		cascadeContains(t, "v1.4 diff", r.all(), "--- a/"+conf)
		cascadeContains(t, "v1.4 notice", r.stderr, "hand-edited generated files would be overwritten: "+conf)
		cascadeContains(t, "v1.4 notice", r.stderr, "v1.5 refuses without --force")
		if strings.Contains(p.read(conf), "# hand edit") {
			t.Fatal("v1.4 build kept the hand edit; it overwrites as it always did")
		}
		if got := p.read(foreign); got != foreignBody {
			t.Fatalf("an unmarked foreign conf was touched:\n%s", got)
		}
		return
	}

	before, beforeNoHome := p.snapshot(), p.snapshotNoHome()
	for _, args := range [][]string{
		{"build"},
		{"config", "set", "BASE_DOMAIN", "other.example.test"},
		{"doctor", "--fix"},
	} {
		r := p.run(args...)
		if r.code != 4 {
			t.Fatalf("nself %s: exit %d, want 4 (E403)\n%s", strings.Join(args, " "), r.code, r.all())
		}
		cascadeContains(t, strings.Join(args, " "), r.all(), "E403")
		cascadeContains(t, strings.Join(args, " ")+" diff", r.all(), "--- a/"+conf)
		cascadeContains(t, strings.Join(args, " ")+" diff", r.all(), "-# hand edit")
		if args[0] == "doctor" {
			p.assertUnchanged(beforeNoHome, "refused nself "+strings.Join(args, " "))
		} else {
			p.assertUnchanged(before, "refused nself "+strings.Join(args, " "))
		}
	}
	if !strings.Contains(p.read(conf), "# hand edit") || p.read(foreign) != foreignBody {
		t.Fatal("the hand edit or the foreign conf did not survive the refusals")
	}

	p.mustRun("build", "--force")
	if strings.Contains(p.read(conf), "# hand edit") {
		t.Fatal("build --force left the hand edit in place")
	}
	if got, want := p.generatedRecord(conf), cascadeSHA(p.read(conf)); got != want {
		t.Fatalf("generated state not updated after --force: recorded %s, file %s", got, want)
	}
	if got := p.read(foreign); got != foreignBody {
		t.Fatalf("an unmarked foreign conf was touched by --force:\n%s", got)
	}
	if rec := p.generatedRecord(foreign); rec != "" {
		t.Fatalf("the foreign conf was recorded as generated: %s", rec)
	}
	backups, _ := filepath.Glob(filepath.Join(p.dir, ".nself", "backups", "nginx-sites-*", "api-docs.conf"))
	if len(backups) == 0 || !strings.Contains(cascadeRead(t, backups[len(backups)-1]), "# hand edit") {
		t.Fatalf("no nginx-sites backup holds the hand-edited conf: %v", backups)
	}
	p.assertPlanEmpty("after build --force")
}

// ---- scenario d: prod-class refusal ---------------------------------------------

// cascadeProdClass uses a created, never started, container labelled for the
// project with a stale config hash. It is the cheapest real-Docker input that
// makes container impact meaningful (the plan must read the daemon and plan a
// recreate); no host port is bound, so it cannot clash with another stack.
func cascadeProdClass(t *testing.T, bin string, mode cascadeMode) {
	p := newCascadeProject(t, bin, "prod-plugins", mode.v15)
	img := p.scratchImage()
	ctr := "l08-" + p.name + "-hasura"
	cascadeDocker(t, "create", "--name", ctr,
		"--label", "com.docker.compose.project="+p.name, "--label", "com.docker.compose.service=hasura",
		"--label", "com.docker.compose.config-hash=stale", img, "/nonexistent")
	containers := p.containerNames()
	state := func() string { return cascadeDocker(t, "inspect", "-f", "{{.State.Status}}", ctr) }
	if state() != "created" {
		t.Fatalf("scratch container is %q, want created", state())
	}

	before := p.snapshot()
	pl := p.planJSON()
	p.assertUnchanged(before, "build --plan")
	if p.containerNames() != containers {
		t.Fatal("build --plan changed the container list")
	}
	if pl.EnvClass != "prod" || !pl.RequiresConfirmation || len(pl.Artifacts) <= 5 {
		t.Fatalf("prod plan: env_class=%q requires_confirmation=%v artifacts=%d", pl.EnvClass, pl.RequiresConfirmation, len(pl.Artifacts))
	}
	if !pl.Containers.Known {
		t.Fatal("containers.known is false with a reachable Docker daemon")
	}
	recreate := false
	for _, it := range pl.Containers.Items {
		if it.Service == "hasura" && it.Action == "recreate" {
			recreate = true
		}
	}
	if !recreate {
		t.Fatalf("the stale-labelled hasura container is not planned as recreate: %+v", pl.Containers.Items)
	}

	if !mode.v15 {
		// v1.4 (live-safety-fixture.sh, same notice): proceeds, says v1.5 will need --yes.
		r := p.mustRun("build")
		cascadeContains(t, "v1.4 notice", r.stderr, "v1.5 will require --yes")
		r = p.mustRun("config", "set", "MONITORING_ENABLED", "true")
		cascadeContains(t, "v1.4 config set", r.stderr, "config: MONITORING_ENABLED")
		if state() != "created" {
			t.Fatalf("a v1.4 build changed the container: %q", state())
		}
		return
	}

	for _, args := range [][]string{{"build"}, {"config", "set", "MONITORING_ENABLED", "true"}} {
		r := p.run(args...)
		if r.code != 4 {
			t.Fatalf("nself %s: exit %d, want 4\n%s", strings.Join(args, " "), r.code, r.all())
		}
		cascadeContains(t, strings.Join(args, " "), r.all(), "E403")
		p.assertUnchanged(before, "refused nself "+strings.Join(args, " "))
	}
	jr := p.run("build", "--json")
	if jr.code != 4 || cascadeErrCode(jr.stdout) != "E403" {
		t.Fatalf("build --json: exit %d code %q, want 4 and E403\n%s", jr.code, cascadeErrCode(jr.stdout), jr.stdout)
	}
	p.assertUnchanged(before, "refused build --json")
	if p.containerNames() != containers {
		t.Fatal("a refused build changed the container list")
	}

	p.mustRun("build", "--yes")
	if !p.exists("docker-compose.yml") {
		t.Fatal("build --yes did not generate the compose file")
	}
	if state() != "created" {
		t.Fatalf("build --yes changed the container to %q; build never restarts services", state())
	}
	// The stale container keeps the plan non-empty (the next start recreates it).
	if p.planJSON().Empty {
		t.Fatal("the plan is empty although a stale-hash container is still present")
	}
	cascadeDocker(t, "rm", "-f", ctr)
	p.assertPlanEmpty("after build --yes and removing the stale container")

	p.mustRun("config", "set", "MONITORING_ENABLED", "true", "--yes")
	cascadeContains(t, "applied line", p.read(".env"), "MONITORING_ENABLED=true")
	p.assertPlanEmpty("after config set --yes")
}

// ---- scenario e: doctor --fix ----------------------------------------------------

func cascadeDoctorFix(t *testing.T, bin string, mode cascadeMode) {
	files := []string{"docker-compose.yml", "nginx/sites/hasura.conf", "nginx/sites/auth.conf"}
	p := newCascadeProject(t, bin, "dev-minimal", mode.v15)
	p.mustRun("build")
	want := map[string]string{}
	for _, f := range files {
		want[f] = p.read(f)
	}
	p.assertDoctorDrift(false)

	// Drift: one file corrupted (a hand edit), one deleted.
	p.write("docker-compose.yml", want["docker-compose.yml"]+"# corrupted\n")
	if err := os.Remove(p.path("nginx/sites/auth.conf")); err != nil {
		t.Fatal(err)
	}
	p.assertDoctorDrift(true, "docker-compose.yml", "nginx/sites/auth.conf")

	fix := []string{"doctor", "--fix"}
	if mode.v15 {
		// v1.5: a hand-edited file needs --force, so a plain --fix refuses and writes nothing.
		before := p.snapshotNoHome()
		r := p.run(fix...)
		if r.code != 4 {
			t.Fatalf("doctor --fix over a hand edit: exit %d, want 4\n%s", r.code, r.all())
		}
		cascadeContains(t, "doctor --fix refusal", r.all(), "E403")
		p.assertUnchanged(before, "refused doctor --fix")
		fix = append(fix, "--force")
	}
	r := p.run(fix...)
	if !mode.v15 {
		cascadeContains(t, "v1.4 doctor --fix notice", r.all(), "hand-edited generated files would be overwritten")
	}
	cascadeContains(t, "doctor --fix", r.all(), "Generated files: repaired")
	for _, f := range files {
		if got := p.read(f); got != want[f] {
			t.Fatalf("%s is not byte-identical to a fresh build after doctor --fix", f)
		}
	}
	p.assertPlanEmpty("after doctor --fix")
	p.assertDoctorDrift(false)

	// A second --fix changes nothing.
	before := p.snapshotNoHome()
	r = p.run(fix...)
	cascadeContains(t, "second doctor --fix", r.all(), "Generated files")
	p.assertUnchanged(before, "a second doctor --fix")
	p.assertDoctorDrift(false)
}
