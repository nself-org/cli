package commands

// build_plan_test.go — `nself build --plan | --yes | --plan-id` (P7-LIVE-03).
//
// Purpose: drive the real binary on hermetic fixture projects (copied from
// internal/reconcile/testdata/fixtures into a temp root that also holds HOME and
// the plugin dir) and prove, end to end:
//   - --plan writes nothing and its --json output is a v1 envelope whose data
//     validates against the generated schema;
//   - a prod-class change without --yes is refused (E403, exit 4) in v1.5 mode
//     and proceeds with a notice in v1.4 mode; --yes applies; a plan after the
//     apply is empty;
//   - --plan-id binds an apply to the plan that was shown (E450 when stale);
//   - a dev build writes exactly the tree origin/main wrote (golden).
// A docker stub on PATH answers the two read-only questions container impact
// asks (`ps`, `compose config --hash`) from files the test writes, so the
// running-stack cases need no daemon. Skipped under -short (builds the binary).
// UPDATE_GOLDEN=1 with NSELF_L03_REF_BIN=<origin/main binary> rewrites the golden.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const l03Fixtures = "../../internal/reconcile/testdata/fixtures"

// l03Stub is the docker stub: ps and compose config --hash print the files named
// by STUB_PS and STUB_HASH; anything else fails like a daemon that is down.
const l03Stub = `#!/bin/sh
case "$1" in
  ps) [ -n "$STUB_PS" ] && [ -f "$STUB_PS" ] && cat "$STUB_PS"; exit 0 ;;
  compose) for a in "$@"; do
      if [ "$a" = "--hash" ]; then [ -n "$STUB_HASH" ] && [ -f "$STUB_HASH" ] && cat "$STUB_HASH"; exit 0; fi
    done ;;
esac
echo "Cannot connect to the Docker daemon" >&2
exit 1
`

// l03Project is one hermetic fixture project.
type l03Project struct {
	root, project, home, plugins, stub string
	// cmdLog leaves the invocation log (~/.nself/logs/nself.log) on; by default
	// it is off so the root can be hashed.
	cmdLog bool
}

func newL03Project(t *testing.T, fixture string) *l03Project {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the docker stub is a POSIX shell script")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := &l03Project{root: root, project: filepath.Join(root, "project"), home: filepath.Join(root, "home"),
		plugins: filepath.Join(root, "plugins"), stub: filepath.Join(root, "stub")}
	// .nself exists up front: the command guard's lock file lives there.
	for _, d := range []string{p.project, p.home, p.plugins, p.stub, filepath.Join(p.project, ".nself")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(p.stub, "docker"), []byte(l03Stub), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(l03Fixtures, fixture)
	l03Copy(t, filepath.Join(src, "project"), p.project)
	l03Copy(t, filepath.Join(src, "plugins"), p.plugins)
	env, _ := os.ReadFile(filepath.Join(p.project, "dotenv"))
	secrets, err := os.ReadFile(filepath.Join(l03Fixtures, "secrets.env"))
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(p.project, "dotenv"))
	l03Write(t, filepath.Join(p.project, ".env"), string(env)+string(secrets), 0o600)
	if certs, err := os.ReadFile(filepath.Join(src, "certs.txt")); err == nil {
		for _, d := range strings.Fields(string(certs)) {
			l03Write(t, filepath.Join(p.project, "ssl", "certificates", d, "fullchain.pem"), "CERT\n", 0o644)
			l03Write(t, filepath.Join(p.project, "ssl", "certificates", d, "privkey.pem"), "KEY\n", 0o600)
		}
	}
	return p
}

func l03Write(t *testing.T, path, body string, perm fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), perm); err != nil {
		t.Fatal(err)
	}
}

func l03Copy(t *testing.T, dir, dst string) {
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
		l03Write(t, filepath.Join(dst, rel), string(data), 0o644)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// l03Binary is the binary under test: the reference binary when the golden is
// being recorded, else the one built from this tree.
func l03Binary(t *testing.T) string {
	if ref := os.Getenv("NSELF_L03_REF_BIN"); ref != "" {
		return ref
	}
	return pilotBinary(t)
}

// run runs the binary in the project. v15 sets NSELF_V15=1.
func (p *l03Project) run(t *testing.T, v15 bool, args ...string) pilotRun {
	t.Helper()
	bin := l03Binary(t)
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "NSELF_") && !strings.HasPrefix(kv, "HOME=") && !strings.HasPrefix(kv, "PATH=") && !strings.HasPrefix(kv, "STUB_") {
			env = append(env, kv)
		}
	}
	env = append(env, "HOME="+p.home, "NSELF_PLUGIN_DIR="+p.plugins, "AI_AUTO_INSTALL=false",
		"PATH="+p.stub+string(os.PathListSeparator)+os.Getenv("PATH"),
		"STUB_PS="+filepath.Join(p.root, "stub-ps.txt"), "STUB_HASH="+filepath.Join(p.root, "stub-hash.txt"))
	if v15 {
		env = append(env, "NSELF_V15=1")
	}
	if !p.cmdLog {
		env = append(env, "NSELF_CMD_LOG_ENABLED=false")
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir, cmd.Env = p.project, env
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return pilotRun{code: code, stdout: o.String(), stderr: e.String()}
}

// running tells the docker stub which containers are running and which hashes
// the planned compose would give.
func (p *l03Project) running(t *testing.T, ps, hashes string) {
	t.Helper()
	l03Write(t, filepath.Join(p.root, "stub-ps.txt"), ps, 0o644)
	l03Write(t, filepath.Join(p.root, "stub-hash.txt"), hashes, 0o644)
}

// tree is one sorted line per file and directory under the root: kind, mode,
// path, sha256 of the content with the root replaced by <ROOT>. The command
// guard's lock file is not project content and is skipped.
func (p *l03Project) tree(t *testing.T, under string) string {
	t.Helper()
	var lines []string
	base := filepath.Join(p.root, under)
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == base {
			return err
		}
		rel, _ := filepath.Rel(base, path)
		rel = filepath.ToSlash(rel)
		if rel == ".nself/op.lock" || strings.HasSuffix(rel, "/.nself/op.lock") {
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
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256([]byte(strings.ReplaceAll(string(data), p.root, "<ROOT>")))
		lines = append(lines, fmt.Sprintf("f %04o %s %s", info.Mode().Perm(), rel, hex.EncodeToString(sum[:])))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

// whole is the tree of the root: project, HOME, plugin dir and stub.
func (p *l03Project) whole(t *testing.T) string { return p.tree(t, ".") }

// planData runs `build --plan --json` and returns the validated envelope data.
func (p *l03Project) planData(t *testing.T, v15 bool) map[string]any {
	t.Helper()
	r := p.run(t, v15, "build", "--plan", "--json")
	if r.code != 0 {
		t.Fatalf("build --plan --json exited %d\n%s\n%s", r.code, r.stdout, r.stderr)
	}
	return validateEnvelope(t, r.stdout, "build")
}

func errorEnvelope(t *testing.T, stdout string) (code string, exit float64) {
	t.Helper()
	doc := singleDoc(t, stdout)
	e, _ := doc["error"].(map[string]any)
	if e == nil {
		t.Fatalf("stdout is not an error envelope:\n%s", stdout)
	}
	c, _ := e["code"].(string)
	x, _ := e["exit_code"].(float64)
	return c, x
}

// TestBuildPlanProdFixture: a prod fixture with a running stack plans without
// changing anything, and lists artifacts, effects and container items.
func TestBuildPlanProdFixture(t *testing.T) {
	for _, v15 := range []bool{false, true} {
		t.Run(fmt.Sprintf("v15=%v", v15), func(t *testing.T) {
			p := newL03Project(t, "prod-plugins")
			p.running(t, "fx_hasura\trunning\thasura\tstale-hash\nfx_nginx\trunning\tnginx\tsame-hash\n", "hasura fresh-hash\nnginx same-hash\n")
			before := p.whole(t)
			data := p.planData(t, v15)
			if after := p.whole(t); after != before {
				t.Fatalf("build --plan changed the root:\n%s", l03LineDiff(before, after))
			}
			if data["env_class"] != "prod" || data["requires_confirmation"] != true || data["empty"] != false {
				t.Fatalf("class/confirmation/empty wrong: %v %v %v", data["env_class"], data["requires_confirmation"], data["empty"])
			}
			if arts, _ := data["artifacts"].([]any); len(arts) < 5 {
				t.Fatalf("artifacts not listed: %v", data["artifacts"])
			}
			var kinds []string
			for _, e := range data["effects"].([]any) {
				kinds = append(kinds, e.(map[string]any)["kind"].(string))
			}
			if !strings.Contains(strings.Join(kinds, ","), "plugin-fragment") {
				t.Fatalf("effects lack plugin-fragment: %v", kinds)
			}
			c := data["containers"].(map[string]any)
			items, _ := c["items"].([]any)
			if c["known"] != true || len(items) != 1 {
				t.Fatalf("containers: %v", c)
			}
			if it := items[0].(map[string]any); it["service"] != "hasura" || it["action"] != "recreate" || it["applied_by"] != "next-start" {
				t.Fatalf("container item: %v", it)
			}
		})
	}
}

// TestBuildPlanWritesOnlyTheInvocationLog: with the command log on (the default),
// `build --plan` still changes exactly one file outside the project: the
// invocation log line every command writes. Nothing else moves.
func TestBuildPlanWritesOnlyTheInvocationLog(t *testing.T) {
	p := newL03Project(t, "prod-plugins")
	p.cmdLog = true
	before := p.whole(t)
	p.planData(t, true)
	got := l03LineDiff(before, p.whole(t))
	for _, want := range []string{"+ d 0755 home/.nself", "+ d 0755 home/.nself/logs", "+ f 0644 home/.nself/logs/nself.log "} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in the diff:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "\n") + 1; n != 3 {
		t.Fatalf("build --plan changed %d paths, want only the 3 of the invocation log:\n%s", n, got)
	}
}

func l03LineDiff(want, got string) string {
	set := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, l := range strings.Split(s, "\n") {
			m[l] = true
		}
		return m
	}
	w, g := set(want), set(got)
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

// TestBuildPlanUnknownContainers: docker down is reported as known=false with a
// stderr notice, and the plan still succeeds.
func TestBuildPlanUnknownContainers(t *testing.T) {
	p := newL03Project(t, "prod-ssl")
	_ = os.Remove(filepath.Join(p.stub, "docker"))
	l03Write(t, filepath.Join(p.stub, "docker"), "#!/bin/sh\necho 'Cannot connect to the Docker daemon' >&2\nexit 1\n", 0o755)
	r := p.run(t, true, "build", "--plan", "--json")
	if r.code != 0 || !strings.Contains(r.stderr, "container impact unknown") {
		t.Fatalf("exit %d, stderr lacks the notice:\n%s", r.code, r.stderr)
	}
	data := validateEnvelope(t, r.stdout, "build")
	if c := data["containers"].(map[string]any); c["known"] != false {
		t.Fatalf("containers.known = %v", c["known"])
	}
}

// TestBuildProdClassRefusal: v1.5, not interactive, no --yes: E403 exit 4 with
// nothing written; --yes applies; the plan afterwards is empty. v1.4 proceeds
// with a notice.
func TestBuildProdClassRefusal(t *testing.T) {
	p := newL03Project(t, "prod-plugins")
	before := p.whole(t)
	r := p.run(t, true, "build", "--json")
	if r.code != 4 {
		t.Fatalf("exit %d, want 4\n%s\n%s", r.code, r.stdout, r.stderr)
	}
	if code, exit := errorEnvelope(t, r.stdout); code != "E403" || exit != 4 {
		t.Fatalf("error %s exit %v, want E403 4", code, exit)
	}
	if after := p.whole(t); after != before {
		t.Fatalf("the refused build changed the root:\n%s", l03LineDiff(before, after))
	}
	r = p.run(t, true, "build", "--yes", "--json")
	if r.code != 0 {
		t.Fatalf("build --yes exited %d\n%s\n%s", r.code, r.stdout, r.stderr)
	}
	applied := validateEnvelope(t, r.stdout, "build")
	if applied["empty"] != false {
		t.Fatalf("the applied plan must be the plan that was applied: %v", applied["empty"])
	}
	if data := p.planData(t, true); data["empty"] != true {
		t.Fatalf("plan after apply is not empty: %v", data)
	}
	if _, err := os.Stat(filepath.Join(p.project, "docker-compose.yml")); err != nil {
		t.Fatalf("--yes did not build: %v", err)
	}

	q := newL03Project(t, "prod-plugins")
	r = q.run(t, false, "build")
	if r.code != 0 || !strings.Contains(r.stderr, "v1.5 will require --yes") || !strings.Contains(r.stderr, "Confirmation required") {
		t.Fatalf("v1.4 must print the summary and a notice, then build: exit %d\n%s", r.code, r.stderr)
	}
}

// TestBuildPlanIDMismatch: --plan-id from a plan whose inputs have since changed
// is refused with E450 (exit 1) and writes nothing; the new id applies.
func TestBuildPlanIDMismatch(t *testing.T) {
	p := newL03Project(t, "prod-plugins")
	shown := p.planData(t, true)["plan_id"].(string)
	// Monitoring adds three generated files, so the plan changes.
	l03Append(t, filepath.Join(p.project, ".env"), "MONITORING_ENABLED=true\n")
	before := p.whole(t)
	r := p.run(t, true, "build", "--yes", "--plan-id", shown, "--json")
	if r.code != 1 {
		t.Fatalf("exit %d, want 1\n%s\n%s", r.code, r.stdout, r.stderr)
	}
	if code, exit := errorEnvelope(t, r.stdout); code != "E450" || exit != 1 {
		t.Fatalf("error %s exit %v, want E450 1", code, exit)
	}
	if after := p.whole(t); after != before {
		t.Fatalf("E450 build changed the root:\n%s", l03LineDiff(before, after))
	}
	fresh := p.planData(t, true)["plan_id"].(string)
	if fresh == shown {
		t.Fatal("the changed input did not change the plan id: the test input is not exercising the binding")
	}
	if r := p.run(t, true, "build", "--yes", "--plan-id", fresh); r.code != 0 {
		t.Fatalf("the current plan id must apply: exit %d\n%s", r.code, r.stderr)
	}
}

func l03Append(t *testing.T, path, add string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	l03Write(t, path, string(b)+add, 0o600)
}

// TestBuildPlanDiffAndFlags: --diff prints unified diffs on stderr with no env
// value in them, and --plan with --check is a usage error.
func TestBuildPlanDiffAndFlags(t *testing.T) {
	p := newL03Project(t, "dev-minimal")
	r := p.run(t, false, "build", "--plan", "--diff")
	if r.code != 0 || !strings.Contains(r.stderr, "--- a/docker-compose.yml") || !strings.Contains(r.stdout, "Plan ") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stderr, "[REDACTED]") || strings.Contains(r.stdout+r.stderr, "Kq7vZp3Lw9XnR2tYb8MdQa4s") {
		t.Fatal("an env value reached the diff output, or it was not redacted")
	}
	if r := p.run(t, false, "build", "--plan", "--check"); r.code == 0 {
		t.Fatal("--plan --check must be refused")
	}
}

// goldenPath holds the tree origin/main wrote for the dev fixture.
const goldenPath = l03Fixtures + "/dev-minimal/golden-tree.txt"

// TestBuildPlanDevGolden: `nself build` on a dev fixture writes exactly the tree
// origin/main wrote (no confirmation, no new file besides the lock), and its
// --plan needs no confirmation.
func TestBuildPlanDevGolden(t *testing.T) {
	p := newL03Project(t, "dev-minimal")
	r := p.run(t, true, "build")
	if r.code != 0 || !strings.Contains(r.stdout, "Build Complete") || strings.Contains(r.stderr, "Confirmation required") {
		t.Fatalf("dev build: exit %d\n%s\n%s", r.code, r.stdout, r.stderr)
	}
	got := p.tree(t, "project")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		l03Write(t, goldenPath, got, 0o644)
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatalf("the dev tree differs from the origin/main golden:\n%s", l03LineDiff(string(want), got))
	}
	data := p.planData(t, true)
	if data["requires_confirmation"] != false || data["empty"] != true {
		t.Fatalf("dev plan after build: requires_confirmation=%v empty=%v", data["requires_confirmation"], data["empty"])
	}
}

// TestBuildPlanIDBindsContent: swapping one env value for another of the same
// shape changes the plan id, so the old --plan-id is refused (E450) and nothing
// is written.
func TestBuildPlanIDBindsContent(t *testing.T) {
	p := newL03Project(t, "prod-ssl")
	shown := p.planData(t, true)["plan_id"].(string)
	if again := p.planData(t, true)["plan_id"].(string); again != shown {
		t.Fatal("identical inputs gave different plan ids")
	}
	b, _ := os.ReadFile(filepath.Join(p.project, ".env"))
	l03Write(t, filepath.Join(p.project, ".env"), strings.Replace(string(b), "MINIO_ROOT_USER=minio-user-Tq8Zk", "MINIO_ROOT_USER=minio-user-Tq8Zj", 1), 0o600)
	before := p.whole(t)
	r := p.run(t, true, "build", "--yes", "--plan-id", shown, "--json")
	if code, exit := errorEnvelope(t, r.stdout); r.code != 1 || code != "E450" || exit != 1 {
		t.Fatalf("exit %d error %s: want E450 exit 1\n%s", r.code, code, r.stderr)
	}
	if p.whole(t) != before {
		t.Fatal("the E450 build wrote")
	}
}

// l03LifecycleStore writes a lifecycle store whose plugin "ghost" is past its
// grace period (an auto-removal candidate) and not installed, so removing it
// fails.
func (p *l03Project) l03LifecycleStore(t *testing.T) {
	t.Helper()
	l03Write(t, filepath.Join(p.home, ".config", "nself", "plugin-lifecycle.json"), `{"version":1,"records":{"ghost":{"name":"ghost","state":"dormant",
"license_expiry":"2020-01-01T00:00:00Z","dormant_since":"2020-02-01T00:00:00Z","grace_period":1000000000}}}`, 0o600)
}

// TestBuildPluginRemovalFailureIsFatal (Codex 4): a planned removal of an
// expired plugin that fails stops the build before any write; --plan lists the
// removal and writes nothing; --check never runs it.
func TestBuildPluginRemovalFailureIsFatal(t *testing.T) {
	p := newL03Project(t, "dev-minimal")
	p.l03LifecycleStore(t)
	store := filepath.Join(p.home, ".config", "nself", "plugin-lifecycle.json")
	before, _ := os.ReadFile(store)
	data := p.planData(t, false)
	var listed bool
	for _, e := range data["effects"].([]any) {
		listed = listed || e.(map[string]any)["kind"] == "plugin-remove"
	}
	if !listed || data["destructive"] != true {
		t.Fatalf("the plan must list the removal as destructive: %v", data["effects"])
	}
	if after, _ := os.ReadFile(store); string(after) != string(before) {
		t.Fatal("--plan changed the lifecycle store")
	}
	if r := p.run(t, false, "build", "--check"); r.code != 0 {
		t.Fatalf("--check exited %d\n%s", r.code, r.stderr)
	}
	if after, _ := os.ReadFile(store); string(after) != string(before) {
		t.Fatal("--check removed plugins")
	}
	r := p.run(t, false, "build", "--yes")
	if r.code == 0 || !strings.Contains(r.stderr, "expired plugin removal failed") {
		t.Fatalf("a failed planned removal must be fatal: exit %d\n%s", r.code, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(p.project, "docker-compose.yml")); err == nil {
		t.Fatal("the build wrote after a failed removal")
	}
}

// TestBuildDevNeedsNoDocker (review S2): a dev apply without --plan, --json or
// --plan-id never asks Docker, so a machine without Docker sees no new notice.
func TestBuildDevNeedsNoDocker(t *testing.T) {
	p := newL03Project(t, "dev-minimal")
	l03Write(t, filepath.Join(p.stub, "docker"), "#!/bin/sh\necho 'Cannot connect to the Docker daemon' >&2\nexit 1\n", 0o755)
	r := p.run(t, false, "build")
	if r.code != 0 || strings.Contains(r.stderr, "container impact") {
		t.Fatalf("exit %d, dev build mentioned Docker:\n%s", r.code, r.stderr)
	}
}

// TestBuildUnknownDockerIsNotEmpty (review M5): with Docker unreachable the plan
// is not empty and says so on stdout, human and JSON.
func TestBuildUnknownDockerIsNotEmpty(t *testing.T) {
	p := newL03Project(t, "prod-ssl")
	l03Write(t, filepath.Join(p.stub, "docker"), "#!/bin/sh\nexit 1\n", 0o755)
	if r := p.run(t, true, "build", "--yes"); r.code != 0 {
		t.Fatalf("setup build exited %d\n%s", r.code, r.stderr)
	}
	r := p.run(t, true, "build", "--plan")
	if strings.Contains(r.stdout, "(no changes)") || !strings.Contains(r.stdout, "state unknown") || !strings.Contains(r.stdout, "Confirmation required") {
		t.Fatalf("unknown Docker state read as empty:\n%s", r.stdout)
	}
	data := p.planData(t, true)
	if data["empty"] != false || data["requires_confirmation"] != true || data["containers"].(map[string]any)["known"] != false {
		t.Fatalf("data: empty=%v requires=%v", data["empty"], data["requires_confirmation"])
	}
}

// TestBuildProjectEnvCannotToggleGate (review M1b) end to end: NSELF_V15=0 in
// the project's env files does not switch the v1.5 refusal off.
func TestBuildProjectEnvCannotToggleGate(t *testing.T) {
	p := newL03Project(t, "prod-ssl")
	l03Append(t, filepath.Join(p.project, ".env"), "NSELF_V15=0\n")
	l03Write(t, filepath.Join(p.project, ".env.local"), "NSELF_V15=false\n", 0o600)
	before := p.whole(t)
	r := p.run(t, true, "build", "--json")
	if code, exit := errorEnvelope(t, r.stdout); r.code != 4 || code != "E403" || exit != 4 {
		t.Fatalf("exit %d %s: want E403 exit 4\n%s", r.code, code, r.stderr)
	}
	if p.whole(t) != before {
		t.Fatal("the refused build wrote")
	}
}

// TestBuildFreshProjectPlanIDRefused: a first build generates secrets, so
// --plan-id is refused with E451; confirming without it applies and persists
// .env.secrets with the bytes that were planned.
func TestBuildFreshProjectPlanIDRefused(t *testing.T) {
	p := newL03Project(t, "prod-ssl")
	env, _ := os.ReadFile(filepath.Join(p.project, ".env"))
	var keep []string
	for _, l := range strings.Split(string(env), "\n") {
		if !strings.HasPrefix(l, "PLUGIN_INTERNAL_SECRET") && !strings.HasPrefix(l, "NOTIFY_INTERNAL_SECRET") &&
			!strings.HasPrefix(l, "CRON_INTERNAL_SECRET") && !strings.HasPrefix(l, "HASURA_GRAPHQL_JWT_SECRET") {
			keep = append(keep, l)
		}
	}
	l03Write(t, filepath.Join(p.project, ".env"), strings.Join(keep, "\n"), 0o600)
	data := p.planData(t, true)
	var listed bool
	for _, a := range data["artifacts"].([]any) {
		listed = listed || a.(map[string]any)["path"] == ".env.secrets"
	}
	if !listed {
		t.Fatal(".env.secrets is not in the plan")
	}
	r := p.run(t, true, "build", "--yes", "--plan-id", data["plan_id"].(string), "--json")
	if code, exit := errorEnvelope(t, r.stdout); r.code != 1 || code != "E451" || exit != 1 {
		t.Fatalf("exit %d %s: want E451\n%s", r.code, code, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(p.project, ".env.secrets")); err == nil {
		t.Fatal("E451 wrote")
	}
	if r := p.run(t, true, "build", "--yes"); r.code != 0 {
		t.Fatalf("build --yes exited %d\n%s", r.code, r.stderr)
	}
}

// TestBuildPluginRemovalDoesNotLeakEnv (review R2): the lifecycle step's
// config.Load must not export the project's env into the write. With ENV=prod in
// .env.local and an expired plugin that is removed, the apply renders what the
// plan showed, so the plan afterwards is empty.
func TestBuildPluginRemovalDoesNotLeakEnv(t *testing.T) {
	p := newL03Project(t, "dev-minimal")
	l03Write(t, filepath.Join(p.project, ".env.local"), "ENV=prod\n", 0o600)
	l03Write(t, filepath.Join(p.project, ".env.prod"), "BASE_DOMAIN=prodonly.example.org\nSSL_MODE=none\n", 0o600)
	l03Write(t, filepath.Join(p.plugins, "oldplug", "plugin.json"), `{"name":"oldplug","port":3920,"language":"go"}`, 0o644)
	p.l03LifecycleStoreFor(t, "oldplug")
	r := p.run(t, true, "build", "--yes")
	if r.code != 0 {
		t.Fatalf("build --yes exited %d\n%s", r.code, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(p.plugins, "oldplug")); err == nil {
		t.Fatal("the expired plugin was not removed")
	}
	if data := p.planData(t, true); data["empty"] != true {
		t.Fatalf("the apply rendered something other than the plan (env leaked from the lifecycle step): %v", data["artifacts"])
	}
}

// l03LifecycleStoreFor writes a lifecycle store with one expired plugin.
func (p *l03Project) l03LifecycleStoreFor(t *testing.T, name string) {
	t.Helper()
	l03Write(t, filepath.Join(p.home, ".config", "nself", "plugin-lifecycle.json"), `{"version":1,"records":{"`+name+`":{"name":"`+name+`","state":"dormant",
"license_expiry":"2020-01-01T00:00:00Z","dormant_since":"2020-02-01T00:00:00Z","grace_period":1000000000}}}`, 0o600)
}
