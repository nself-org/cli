package reconcile

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	nbuild "github.com/nself-org/cli/internal/build"
)

func TestStateFaultInjection(t *testing.T) {
	f := loadFixture(t, "dev-minimal")

	// Ensure no state to start
	os.RemoveAll(filepath.Join(f.project, ".nself", "state"))

	oldBuild := buildBuild
	t.Cleanup(func() { buildBuild = oldBuild })

	buildBuild = func(projectDir string, opts nbuild.BuildOptions) (*nbuild.BuildResult, error) {
		// Simulate half-writing a file that nself generates
		os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("half-written"), 0644)
		return nil, errors.New("injected failing sink after N writes")
	}

	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("seed")}
	_, err := Apply(context.Background(), req, ApplyOptions{Yes: true})
	if err == nil {
		t.Fatal("expected apply to fail")
	}

	state, err := LoadGeneratedState(f.project)
	if err != nil {
		t.Fatal(err)
	}
	if state != nil {
		t.Fatal("expected generated-state record to be intact (nil/missing)")
	}

	// Next plan reports it as changed, not hand-edited: the callback is the
	// real one Apply builds from the (absent) record.
	req.HandEdited = state.HandEditedFn(f.project)
	p, err := Compute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	hand := HandEditedPaths(*p)
	if len(hand) > 0 {
		t.Fatalf("expected no hand-edited files, got %v", hand)
	}
}

func TestStateFirstRun(t *testing.T) {
	f := loadFixture(t, "dev-minimal")

	// Ensure no state
	os.RemoveAll(filepath.Join(f.project, ".nself", "state"))

	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("seed")}
	_, err := Apply(context.Background(), req, ApplyOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}

	state, err := LoadGeneratedState(f.project)
	if err != nil || state == nil {
		t.Fatal("expected state to be recorded on first run")
	}
}

// TestStateRefusesRecordWithoutSchema: a record with no schema_version was
// not written by nself (every Save pins it), so loading it fails closed
// instead of trusting an unversioned map of digests.
func TestStateRefusesRecordWithoutSchema(t *testing.T) {
	dir := t.TempDir()
	p := GeneratedStatePath(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"files":{"docker-compose.yml":"00"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s, err := LoadGeneratedState(dir); err == nil {
		t.Fatalf("a record without schema_version must be refused, got %+v", s)
	}
}

// TestHandEditedFnReadFailures: a deleted recorded file is not a hand-edit
// (no user bytes to lose, the build recreates it); any other read failure,
// here a directory in the file's place, counts as hand-edited (fail closed).
func TestHandEditedFnReadFailures(t *testing.T) {
	dir := t.TempDir()
	orig := []byte("services: {}\n")
	sum := sha256.Sum256(orig)
	s := &GeneratedState{Files: map[string]string{"docker-compose.yml": hex.EncodeToString(sum[:])}}
	fn := s.HandEditedFn(dir)
	target := filepath.Join(dir, "docker-compose.yml")

	if fn("docker-compose.yml") {
		t.Fatal("a deleted recorded file must not count as hand-edited")
	}
	if err := os.WriteFile(target, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	if fn("docker-compose.yml") {
		t.Fatal("unchanged bytes must not count as hand-edited")
	}
	if err := os.WriteFile(target, []byte("services: {x: 1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !fn("docker-compose.yml") {
		t.Fatal("changed bytes must count as hand-edited")
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if !fn("docker-compose.yml") {
		t.Fatal("an unreadable recorded path (a directory) must fail closed as hand-edited")
	}
}

// TestStateFaultInjectionWithRecord: with a record already present, an apply
// killed after its file writes but before the record leaves generated.json
// byte-identical, and the next plan (real callback over that record) reports
// no hand-edits: files nself wrote equal what the plan would write, so they
// are not write targets. A file torn mid-write is a different case: its bytes
// match neither side, and it is reported hand-edited (fail closed, --force).
func TestStateFaultInjectionWithRecord(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("seed")}
	if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	rec0, err := os.ReadFile(GeneratedStatePath(f.project))
	if err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(f.project, ".env")
	env, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(env), "BASE_DOMAIN=example.test", "BASE_DOMAIN=changed.test", 1)
	if changed == string(env) {
		t.Fatal("fixture .env has no BASE_DOMAIN=example.test line to change")
	}
	if err := os.WriteFile(envPath, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}

	oldBuild := buildBuild
	t.Cleanup(func() { buildBuild = oldBuild })
	buildBuild = func(projectDir string, opts nbuild.BuildOptions) (*nbuild.BuildResult, error) {
		if _, err := oldBuild(projectDir, opts); err != nil {
			return nil, err
		}
		return nil, errors.New("injected kill after the writes, before the record")
	}
	if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err == nil {
		t.Fatal("expected the injected apply to fail")
	}
	buildBuild = oldBuild

	rec1, err := os.ReadFile(GeneratedStatePath(f.project))
	if err != nil {
		t.Fatal(err)
	}
	if string(rec1) != string(rec0) {
		t.Fatal("an interrupted apply must leave the record byte-identical")
	}
	state, err := LoadGeneratedState(f.project)
	if err != nil || state == nil {
		t.Fatalf("record must load after the interrupted apply: %v", err)
	}
	req.HandEdited = state.HandEditedFn(f.project)
	p, err := Compute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if hand := HandEditedPaths(*p); len(hand) > 0 {
		t.Fatalf("files nself itself wrote must not read as hand-edited, got %v", hand)
	}
}

// TestStateDanglingLinks: a dangling symlink in place of the record is an
// error, not a first run; in place of a recorded file it counts as
// hand-edited.
func TestStateDanglingLinks(t *testing.T) {
	dir := t.TempDir()
	p := GeneratedStatePath(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "nowhere.json"), p); err != nil {
		t.Fatal(err)
	}
	if s, err := LoadGeneratedState(dir); err == nil {
		t.Fatalf("a dangling record link must be refused, got %+v", s)
	}
	s := &GeneratedState{Files: map[string]string{"docker-compose.yml": strings.Repeat("0", 64)}}
	if err := os.Symlink(filepath.Join(dir, "gone.yml"), filepath.Join(dir, "docker-compose.yml")); err != nil {
		t.Fatal(err)
	}
	if !s.HandEditedFn(dir)("docker-compose.yml") {
		t.Fatal("a dangling link in place of a recorded file must count as hand-edited")
	}
	var none *GeneratedState
	if none.HandEditedFn(dir)("docker-compose.yml") {
		t.Fatal("no record means nothing is hand-edited")
	}
}

// TestStateSaveRoundTrip: Save pins the schema, sorts and dedupes host
// entries, writes 0644 and leaves no temp file; Load reads back the same set.
func TestStateSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &GeneratedState{
		Files: map[string]string{"docker-compose.yml": strings.Repeat("a", 64)},
		Host: []GeneratedHostEntry{
			{Kind: "trust-store", Target: "b"}, {Kind: "hosts", Target: "z"},
			{Kind: "hosts", Target: "a"}, {Kind: "hosts", Target: "z"},
		},
	}
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	got, err := LoadGeneratedState(dir)
	if err != nil || got == nil {
		t.Fatalf("load: %v", err)
	}
	want := []GeneratedHostEntry{{Kind: "hosts", Target: "a"}, {Kind: "hosts", Target: "z"}, {Kind: "trust-store", Target: "b"}}
	if len(got.Host) != len(want) {
		t.Fatalf("host entries %v, want %v", got.Host, want)
	}
	for i := range want {
		if got.Host[i] != want[i] {
			t.Fatalf("host entries %v, want %v", got.Host, want)
		}
	}
	if got.SchemaVersion != generatedStateSchema || got.Files["docker-compose.yml"] != strings.Repeat("a", 64) {
		t.Fatalf("round trip lost data: %+v", got)
	}
	info, err := os.Stat(GeneratedStatePath(dir))
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o644) { // Windows has no unix modes
		t.Fatalf("record mode %v (%v), want 0644", info.Mode().Perm(), err)
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(GeneratedStatePath(dir)), "generated.*.tmp"))
	if len(left) != 0 {
		t.Fatalf("temp files left behind: %v", left)
	}
}

// TestStateSaveAndLoadErrors: an unwritable state location and an
// unparsable or future-schema record each surface an error, never a nil
// record that would read as a first run.
func TestStateSaveAndLoadErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".nself"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (&GeneratedState{}).Save(dir); err == nil {
		t.Fatal("Save must fail when .nself is a file")
	}
	// On unix a file in place of .nself is ENOTDIR, a read error; Windows
	// reports "path not found", which is simply no record.
	if _, err := LoadGeneratedState(dir); err == nil && runtime.GOOS != "windows" {
		t.Fatal("Load must fail when the record path cannot be read")
	}
	for name, body := range map[string]string{"corrupt": "{not json", "future": `{"schema_version":"99","files":{}}`} {
		d := t.TempDir()
		p := GeneratedStatePath(d)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if s, err := LoadGeneratedState(d); err == nil {
			t.Fatalf("%s record must be refused, got %+v", name, s)
		}
	}
}

// TestHandEditedFnDisplayPaths: @plugins/, @abs/ and @fronting/ display
// paths resolve to their disk locations; a fronting path with no fronting
// stack configured resolves to nothing (never hand-edited).
func TestHandEditedFnDisplayPaths(t *testing.T) {
	isolateEnv(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	front := filepath.Join(root, "front")
	project := filepath.Join(front, "proj")
	plugins := filepath.Join(root, "plugins")
	for _, d := range []string{project, plugins, filepath.Join(front, "nginx")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NSELF_PLUGIN_DIR", plugins)
	files := map[string]string{
		filepath.Join(plugins, "p.yml"):         "plugin\n",
		filepath.Join(root, "abs.conf"):         "abs\n",
		filepath.Join(front, "nginx", "f.conf"): "front\n",
	}
	for p, body := range files {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	other := sha256.Sum256([]byte("something else\n"))
	digest := hex.EncodeToString(other[:])
	abs := AbsPrefix + strings.TrimPrefix(filepath.ToSlash(filepath.Join(root, "abs.conf")), "/")
	s := &GeneratedState{Files: map[string]string{
		PluginsPrefix + "p.yml": digest, abs: digest, FrontingPrefix + "nginx/f.conf": digest,
	}}

	// No NGINX_FRONTED_BY: the fronting path resolves to nothing.
	fn := s.HandEditedFn(project)
	if fn(FrontingPrefix + "nginx/f.conf") {
		t.Fatal("a fronting path without a fronting stack must not count as hand-edited")
	}
	if !fn(PluginsPrefix+"p.yml") || !fn(abs) {
		t.Fatal("@plugins/ and @abs/ paths must resolve and compare against the record")
	}

	// NGINX_FRONTED_BY in the project env names the parent: it resolves.
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte("NGINX_FRONTED_BY=front\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !s.HandEditedFn(project)(FrontingPrefix + "nginx/f.conf") {
		t.Fatal("a fronting path must resolve under the fronting stack root")
	}
}

// TestStatePlannedRecordFolds: PlannedRecord copies the current record, adds
// every planned file with the sha256 of its bytes, drops removed paths and
// records only hosts / trust-store effects; the input record is not mutated.
func TestStatePlannedRecordFolds(t *testing.T) {
	cur := &GeneratedState{Files: map[string]string{"old.conf": "x", "keep.conf": "y"}}
	pb := &nbuild.PlannedBuild{
		Files:   map[string]nbuild.PlannedFile{"new.conf": {Data: []byte("n\n")}},
		Removed: []string{"old.conf"},
		Effects: []nbuild.PlannedEffect{{Kind: string(EffectHosts), Target: "fx.test"}, {Kind: "build-lock", Target: "ignored"}, {Kind: string(EffectTrustStore), Target: "ca"}},
	}
	next, body, err := PlannedRecord(cur, pb)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("n\n"))
	if next.Files["new.conf"] != hex.EncodeToString(sum[:]) || next.Files["keep.conf"] != "y" {
		t.Fatalf("planned record files %v", next.Files)
	}
	if _, ok := next.Files["old.conf"]; ok {
		t.Fatal("a removed path must leave the record")
	}
	if len(next.Host) != 2 || next.Host[0].Kind != string(EffectHosts) || next.Host[1].Kind != string(EffectTrustStore) {
		t.Fatalf("host entries %v: only hosts and trust-store effects are recorded, sorted", next.Host)
	}
	if _, ok := cur.Files["new.conf"]; ok || cur.Files["old.conf"] != "x" {
		t.Fatal("PlannedRecord must not mutate the current record")
	}
	if !strings.HasSuffix(string(body), "\n") || !strings.Contains(string(body), `"schema_version"`) {
		t.Fatalf("record body is not the canonical encoding: %q", body)
	}
}

// TestStateWriteRecordFailures: a record path occupied by a non-empty
// directory and a read-only state directory both fail the write with an
// error and leave no temp file behind.
func TestStateWriteRecordFailures(t *testing.T) {
	dir := t.TempDir()
	p := GeneratedStatePath(dir)
	if err := os.MkdirAll(filepath.Join(p, "occupied"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeRecord(dir, []byte("{}\n")); err == nil {
		t.Fatal("rename onto a non-empty directory must fail")
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(p), "generated.*.tmp"))
	if len(left) != 0 {
		t.Fatalf("temp files left behind: %v", left)
	}
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("root and Windows ignore unix directory permissions")
	}
	ro := t.TempDir()
	stateDir := filepath.Dir(GeneratedStatePath(ro))
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stateDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(stateDir, 0o755) })
	if err := writeRecord(ro, []byte("{}\n")); err == nil {
		t.Fatal("a read-only state directory must fail the write")
	}
}

// TestStateApplyNoticeAndRecordErrors: the first recording apply prints the
// one-time notice and the second does not; a corrupt record or an
// unwritable record location fails the apply instead of proceeding silently.
func TestStateApplyNoticeAndRecordErrors(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	var errb bytes.Buffer
	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("seed"), Stderr: &errb}
	if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errb.String(), strings.TrimSpace(firstRunNotice)) {
		t.Fatalf("first apply must print the first-run notice, stderr: %q", errb.String())
	}
	errb.Reset()
	if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(errb.String(), strings.TrimSpace(firstRunNotice)) {
		t.Fatal("the second apply must not repeat the first-run notice")
	}

	if err := os.WriteFile(GeneratedStatePath(f.project), []byte("{corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err == nil {
		t.Fatal("a corrupt record must fail the apply")
	}

	if err := os.Remove(GeneratedStatePath(f.project)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(GeneratedStatePath(f.project), "occupied"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err == nil {
		t.Fatal("an unwritable record location must fail the apply")
	}
}

// TestStatePlanRefusesCorruptRecord: Compute (the plan without an apply)
// refuses a corrupt record too; it never plans as if no record existed.
func TestStatePlanRefusesCorruptRecord(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	p := GeneratedStatePath(f.project)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("seed")}
	if _, err := Compute(context.Background(), req); err == nil {
		t.Fatal("a plan over a corrupt record must fail")
	}
}

// TestStateApplyRecordWriteFails: when the build succeeds but the record
// cannot be written (read-only state directory), the apply returns the error
// and the old record stays in place, byte for byte.
func TestStateApplyRecordWriteFails(t *testing.T) {
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("root ignores directory permissions")
	}
	f := loadFixture(t, "dev-minimal")
	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("seed")}
	if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	rec0, err := os.ReadFile(GeneratedStatePath(f.project))
	if err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(f.project, ".env")
	env, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte(strings.Replace(string(env), "BASE_DOMAIN=example.test", "BASE_DOMAIN=ro.test", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Dir(GeneratedStatePath(f.project))
	if err := os.Chmod(stateDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(stateDir, 0o755) })
	if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err == nil {
		t.Fatal("a record that cannot be written must fail the apply")
	}
	rec1, err := os.ReadFile(GeneratedStatePath(f.project))
	if err != nil || string(rec1) != string(rec0) {
		t.Fatalf("the old record must stay in place (%v)", err)
	}
}
