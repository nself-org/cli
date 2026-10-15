package build

// lockfile_test.go — tests for the project lock in the build pipeline
// (P7-LIVE-09): deterministic and change-only writes, bootstrap from an
// existing compose, v1.4 opt-in, precedence of explicit env over pins, and
// plan mode.

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/version"
)

const (
	hasura240 = "hasura/graphql-engine:v2.40.0"
	testDig   = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
)

func lockBuild(t *testing.T, f *planFixture) {
	t.Helper()
	if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
		t.Fatalf("Build: %v", err)
	}
}

func readLock(t *testing.T, f *planFixture) lock.Lock {
	t.Helper()
	l, err := lock.Load(f.workdir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return *l
}

func composeImagesOf(t *testing.T, f *planFixture) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.workdir, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := lock.ComposeImages(raw)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func withExistingCompose(t *testing.T, f *planFixture, body string) {
	t.Helper()
	writeFixtureFile(t, filepath.Join(f.workdir, "docker-compose.yml"), body, 0o600)
}

func setVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = old })
}

// TestLockDeterministic: two v1.5 builds with unchanged inputs leave
// nself.lock.yaml byte-identical and untouched (mtime too), and a CLI patch
// upgrade does not rewrite it.
func TestLockDeterministic(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, true)
	setVersion(t, "1.4.12")
	lockBuild(t, f)
	path := lock.Path(f.workdir)
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("a v1.5 build must write %s: %v", lock.FileName, err)
	}
	past := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	lockBuild(t, f)
	setVersion(t, "1.4.13") // a patch upgrade
	lockBuild(t, f)
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Fatalf("lock changed between identical builds:\n%s\n---\n%s", first, second)
	}
	if st, _ := os.Stat(path); !st.ModTime().Equal(past) {
		t.Fatalf("an unchanged lock was rewritten: mtime %v, want %v", st.ModTime(), past)
	}
	l := readLock(t, f)
	if l.CLI != "1.4" {
		t.Fatalf("cli = %q, want major.minor 1.4", l.CLI)
	}
	if !strings.Contains(l.Images["hasura"], "@sha256:") {
		t.Fatalf("v1.5 pins hasura by digest: %v", l.Images)
	}
	if got := composeImagesOf(t, f)["hasura"]; got != l.Images["hasura"] {
		t.Fatalf("compose runs %q, lock pins %q", got, l.Images["hasura"])
	}
	// A minor upgrade does rewrite it.
	setVersion(t, "1.5.0")
	lockBuild(t, f)
	if got := readLock(t, f).CLI; got != "1.5" {
		t.Fatalf("cli after a minor upgrade = %q", got)
	}
}

// TestLockBootstrapDigest: the first lock of a project with a docker-compose.yml
// takes its pins from that file.
func TestLockBootstrapDigest(t *testing.T) {
	old := "services:\n  hasura:\n    image: " + hasura240 + "\n  redis:\n    image: redis:7-alpine\n  custom:\n    image: mine/app:1\n"

	t.Run("tag-only without a local image", func(t *testing.T) {
		f := newPlanFixture(t, "dev-minimal")
		compattest.Set(t, true)
		withExistingCompose(t, f, old)
		lockBuild(t, f)
		l := readLock(t, f)
		if l.Unpinned["hasura"] != hasura240 {
			t.Fatalf("hasura stays at the existing version, unpinned: images %v unpinned %v", l.Images, l.Unpinned)
		}
		if got := composeImagesOf(t, f)["hasura"]; got != hasura240 {
			t.Fatalf("the first v1.5 build moved hasura to %q", got)
		}
		if _, ok := l.Images["custom"]; ok || l.Unpinned["custom"] != "" {
			t.Fatalf("a service the CLI does not own is not pinned: %v %v", l.Images, l.Unpinned)
		}
		if !strings.Contains(l.Images["nginx"], "@sha256:") {
			t.Fatalf("a service the old compose lacks takes the CLI default: %v", l.Images)
		}
		lockBuild(t, f) // a second build keeps the pin
		if got := composeImagesOf(t, f)["hasura"]; got != hasura240 {
			t.Fatalf("second build moved hasura to %q", got)
		}
	})

	t.Run("local repo digest", func(t *testing.T) {
		f := newPlanFixture(t, "dev-minimal")
		compattest.Set(t, true)
		prev := localRepoDigest
		localRepoDigest = func(ref string) string {
			if ref == hasura240 {
				return testDig
			}
			return ""
		}
		t.Cleanup(func() { localRepoDigest = prev })
		withExistingCompose(t, f, old)
		lockBuild(t, f)
		want := hasura240 + "@" + testDig
		if got := readLock(t, f).Images["hasura"]; got != want {
			t.Fatalf("lock pins %q, want %q", got, want)
		}
		if got := composeImagesOf(t, f)["hasura"]; got != want {
			t.Fatalf("compose runs %q, want %q", got, want)
		}
	})

	t.Run("pin sources", func(t *testing.T) {
		f := newPlanFixture(t, "dev-minimal")
		st := newBuildState(f.workdir, BuildOptions{})
		redis, _ := compose.LockedRef("redis")
		cases := []struct {
			name, ref, local, want string
		}{
			{"local digest", "redis:7-alpine", testDig, "redis:7-alpine@" + testDig},
			{"CLI lock version matches", "redis:7-alpine", "", redis.String()},
			{"docker.io spelling of the same image", "docker.io/redis:7-alpine", "", redis.String()},
			{"unknown version stays a tag", "redis:6", "", "redis:6"},
		}
		for _, c := range cases {
			withExistingCompose(t, f, "services:\n  redis:\n    image: "+c.ref+"\n")
			prev := localRepoDigest
			localRepoDigest = func(string) string { return c.local }
			pins, err := st.bootstrapPins()
			localRepoDigest = prev
			if err != nil {
				t.Fatal(err)
			}
			if pins["redis"] != c.want {
				t.Errorf("%s: pin %q, want %q", c.name, pins["redis"], c.want)
			}
		}
	})

	t.Run("explicit env beats the pin", func(t *testing.T) {
		f := newPlanFixture(t, "dev-minimal")
		compattest.Set(t, true)
		raw, _ := os.ReadFile(filepath.Join(f.workdir, ".env"))
		writeFixtureFile(t, filepath.Join(f.workdir, ".env"), "HASURA_VERSION=v2.41.0\n"+string(raw), 0o600)
		withExistingCompose(t, f, old)
		lockBuild(t, f)
		if got := composeImagesOf(t, f)["hasura"]; got != "hasura/graphql-engine:v2.41.0" {
			t.Fatalf("explicit HASURA_VERSION lost to the pin: %q", got)
		}
	})

	// nself init writes *_VERSION defaults into .env: a value equal to the pin's
	// reference must keep ref@digest (lens must-fix 2).
	t.Run("env version equal to the pin keeps the digest", func(t *testing.T) {
		f := newPlanFixture(t, "dev-minimal")
		compattest.Set(t, true)
		prev := localRepoDigest
		localRepoDigest = func(ref string) string {
			if ref == hasura240 {
				return testDig
			}
			return ""
		}
		t.Cleanup(func() { localRepoDigest = prev })
		raw, _ := os.ReadFile(filepath.Join(f.workdir, ".env"))
		writeFixtureFile(t, filepath.Join(f.workdir, ".env"), "HASURA_VERSION=v2.40.0\n"+string(raw), 0o600)
		withExistingCompose(t, f, old)
		lockBuild(t, f)
		want := hasura240 + "@" + testDig
		if got := readLock(t, f).Images["hasura"]; got != want {
			t.Fatalf("lock pins %q, want %q", got, want)
		}
		if got := composeImagesOf(t, f)["hasura"]; got != want {
			t.Fatalf("an env default equal to the pin dropped the digest: compose runs %q, want %q", got, want)
		}
		lockBuild(t, f)
		if got := readLock(t, f).Images["hasura"]; got != want {
			t.Fatalf("second build turned the pin into %q", got)
		}
	})
}

// TestLockV14OptIn: v1.4 writes no lock unless build --write-lock opts in; once
// the file exists it is honoured and updated.
func TestLockV14OptIn(t *testing.T) {
	prev := localRepoDigest
	localRepoDigest = func(string) string { return "" }
	t.Cleanup(func() { localRepoDigest = prev })
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, false)
	lockBuild(t, f)
	if _, err := os.Stat(lock.Path(f.workdir)); !os.IsNotExist(err) {
		t.Fatalf("v1.4 build created the lock without --write-lock (stat err %v)", err)
	}
	if _, err := Build(f.workdir, BuildOptions{Force: true, WriteLock: true}); err != nil {
		t.Fatalf("Build --write-lock: %v", err)
	}
	// The earlier v1.4 build left a tag-only compose; the first lock takes its
	// pins from it, with the digest from the CLI image lock (the versions match).
	hasura, _ := compose.LockedRef("hasura-v14") // what v1.4 runs (P7-LIVE-23)
	if l := readLock(t, f); l.Images["hasura"] != hasura.String() {
		t.Fatalf("first v1.4 lock pins hasura %q, want the CLI image lock's %q: %+v", l.Images["hasura"], hasura.String(), l)
	}

	// Honoured: a pin in the existing file moves the render; updated: the file is rewritten.
	old := lock.New()
	old.CLI = "0.1"
	old.Unpinned["hasura"] = hasura240
	raw, err := old.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, lock.Path(f.workdir), string(raw), 0o644)
	lockBuild(t, f)
	if got := composeImagesOf(t, f)["hasura"]; got != hasura240 {
		t.Fatalf("v1.4 build ignored the lock pin: %q", got)
	}
	l := readLock(t, f)
	if l.CLI == "0.1" || l.Unpinned["hasura"] != hasura240 {
		t.Fatalf("lock was not updated or lost its pin: %+v", l)
	}
}

// TestLockPlanMode: a plan-mode build reports the lock as a planned file and
// writes nothing; the apply writes exactly those bytes.
func TestLockPlanMode(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, true)
	res, err := Build(f.workdir, BuildOptions{Mode: ModePlan, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	pf, ok := res.Planned.Files[lock.FileName]
	if !ok {
		t.Fatalf("plan does not list %s", lock.FileName)
	}
	if _, err := os.Stat(lock.Path(f.workdir)); !os.IsNotExist(err) {
		t.Fatalf("plan mode wrote the lock (stat err %v)", err)
	}
	lockBuild(t, f)
	got, _ := os.ReadFile(lock.Path(f.workdir))
	if string(got) != string(pf.Data) {
		t.Fatalf("apply wrote other bytes than the plan:\nplan:\n%s\napply:\n%s", pf.Data, got)
	}
}

// TestLockWriteLockOption: BuildOptions.WriteLock is the only opt-in. A process
// environment variable (the old side channel) does nothing, and a build
// without the field writes no lock under v1.4.
func TestLockWriteLockOption(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, false)
	t.Setenv("NSELF_WRITE_LOCK", "1")
	lockBuild(t, f)
	if _, err := os.Stat(lock.Path(f.workdir)); !os.IsNotExist(err) {
		t.Fatalf("an environment variable opted a v1.4 build into the lock (stat err %v)", err)
	}
	if _, err := Build(f.workdir, BuildOptions{Force: true, WriteLock: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lock.Path(f.workdir)); err != nil {
		t.Fatalf("WriteLock did not write the lock: %v", err)
	}
}

// fakeImageInspect puts a docker first on PATH that answers `image inspect`
// with out and fails every other call (a daemon that is otherwise down).
func fakeImageInspect(t *testing.T, out string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = image ] && [ \"$2\" = inspect ]; then echo '" + out + "'; exit 0; fi\n" +
		"echo 'Cannot connect to the Docker daemon' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestLockBootstrapDigestFunnel: with the real localRepoDigest (no test seam),
// the first lock pins the digest `docker image inspect` reports for the local
// image, and falls back to the CLI image lock when the image has no RepoDigest.
func TestLockBootstrapDigestFunnel(t *testing.T) {
	old := "services:\n  hasura:\n    image: " + hasura240 + "\n"
	t.Run("local image with a repo digest", func(t *testing.T) {
		fakeImageInspect(t, `["hasura/graphql-engine@`+testDig+`"]`)
		f := newPlanFixture(t, "dev-minimal")
		compattest.Set(t, true)
		withExistingCompose(t, f, old)
		lockBuild(t, f)
		want := hasura240 + "@" + testDig
		if got := readLock(t, f).Images["hasura"]; got != want {
			t.Fatalf("lock pins %q, want %q", got, want)
		}
		if got := composeImagesOf(t, f)["hasura"]; got != want {
			t.Fatalf("compose runs %q, want %q", got, want)
		}
	})
	t.Run("no repo digest keeps the fallback order", func(t *testing.T) {
		fakeImageInspect(t, "null")
		f := newPlanFixture(t, "dev-minimal")
		compattest.Set(t, true)
		withExistingCompose(t, f, old)
		lockBuild(t, f)
		l := readLock(t, f)
		if l.Unpinned["hasura"] != hasura240 || l.Images["hasura"] != "" {
			t.Fatalf("an image without a RepoDigest must stay unpinned (v2.40.0 is not in the CLI lock): %v %v", l.Images, l.Unpinned)
		}
	})
}

// TestLockReadsLedgerFromPluginDir: the bundle ledger lives at
// <plugin dir>/.nself-state/bundles.json (D-0284), not in the project. A ledger
// recorded there must show in the lock's plugins and bundles sections.
func TestLockReadsLedgerFromPluginDir(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, true)
	led := ledger.New()
	led.Bundles["task"] = ledger.BundleRecord{InstalledAt: "2026-10-10T00:00:00Z"}
	led.Plugins["notify"] = ledger.PluginRecord{InstalledBy: []string{"task"}, Tier: ledger.TierFree, Version: "9.8.7"}
	data, err := led.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	store := ledger.NewStore(f.plugins)
	if err := os.MkdirAll(store.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path(), data, 0o644); err != nil {
		t.Fatal(err)
	}
	lockBuild(t, f)
	l := readLock(t, f)
	if len(l.Bundles) != 1 || l.Bundles[0] != "task" {
		t.Fatalf("lock bundles %v, want [task] from the plugin-dir ledger", l.Bundles)
	}
	if got := l.Plugins["notify"].Version; got != "9.8.7" {
		t.Fatalf("lock plugin notify version %q, want 9.8.7: plugins %v", got, l.Plugins)
	}
	if _, err := os.Stat(filepath.Join(f.workdir, ledger.StateDirName)); !os.IsNotExist(err) {
		t.Fatalf("the build created a ledger directory in the project (stat err %v)", err)
	}
}

// writeSpySink records which write method wrote each path.
type writeSpySink struct {
	Sink
	plain, atomic []string
}

func (w *writeSpySink) WriteFile(p string, d []byte, m fs.FileMode) error {
	w.plain = append(w.plain, p)
	return w.Sink.WriteFile(p, d, m)
}

func (w *writeSpySink) WriteAtomic(p string, d []byte, m fs.FileMode) error {
	w.atomic = append(w.atomic, p)
	return w.Sink.WriteAtomic(p, d, m)
}

// TestLockWrittenAtomically: nself.lock.yaml goes through Sink.WriteAtomic
// (never the plain WriteFile), and what lands on disk parses (trailer included).
func TestLockWrittenAtomically(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	st := newBuildState(f.workdir, BuildOptions{WriteLock: true})
	spy := &writeSpySink{Sink: newDiskSink(f.workdir)}
	st.sink = spy
	st.pluginDir = f.plugins
	if err := st.writeProjectLock([]byte("services:\n  hasura:\n    image: "+hasura240+"\n"), nil); err != nil {
		t.Fatal(err)
	}
	for _, p := range spy.plain {
		if strings.HasSuffix(p, lock.FileName) {
			t.Fatalf("the lock was written with a plain WriteFile: %v", spy.plain)
		}
	}
	if len(spy.atomic) != 1 || !strings.HasSuffix(spy.atomic[0], lock.FileName) {
		t.Fatalf("atomic writes %v, want exactly the lock", spy.atomic)
	}
	if _, err := lock.Load(f.workdir); err != nil {
		t.Fatalf("the written lock does not load: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(f.workdir, lock.FileName+".tmp-*")); len(left) != 0 {
		t.Fatalf("temp files left behind: %v", left)
	}
}

// TestLockMergesComposeOverride: the lock records, and CurrentLock compares,
// the image start runs: docker-compose.override.yml wins over the base file
// (lens must-fix 1).
func TestLockMergesComposeOverride(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, true)
	withExistingCompose(t, f, "services:\n  hasura:\n    image: "+hasura240+"\n")
	lockBuild(t, f)
	before, err := CurrentLock(f.workdir)
	if err != nil {
		t.Fatal(err)
	}
	if got := before.Unpinned["hasura"] + before.Images["hasura"]; got != hasura240 {
		t.Fatalf("base only: hasura %q, want %q", got, hasura240)
	}
	writeFixtureFile(t, filepath.Join(f.workdir, "docker-compose.override.yml"), "services:\n  hasura:\n    image: hasura/graphql-engine:v2.51.0\n  custom:\n    image: mine/app:2\n", 0o644)
	after, err := CurrentLock(f.workdir)
	if err != nil {
		t.Fatal(err)
	}
	if got := after.Unpinned["hasura"] + after.Images["hasura"]; got != "hasura/graphql-engine:v2.51.0" {
		t.Fatalf("override ignored: hasura %q", got)
	}
	if _, ok := after.Unpinned["custom"]; ok {
		t.Fatalf("a service the CLI does not own entered the lock: %v", after.Unpinned)
	}
	// A broken override file is an error, not a silent skip.
	writeFixtureFile(t, filepath.Join(f.workdir, "docker-compose.override.yml"), "services: [unclosed\n", 0o644)
	if _, err := CurrentLock(f.workdir); err == nil {
		t.Fatal("an unparseable override file must fail the comparison")
	}
}

// TestLockBootstrapDockerIoSpelling: a first lock taken from a compose that
// names an image as docker.io/<repo>:<tag> matches the CLI image lock, and in
// the v1.4 default (legacy) mode an init-style HASURA_VERSION equal to that tag
// keeps the pin instead of dropping it (lens round 3 must-fix).
func TestLockBootstrapDockerIoSpelling(t *testing.T) {
	for _, v15 := range []bool{false, true} {
		f := newPlanFixture(t, "dev-minimal")
		compattest.Set(t, v15)
		// The entry the mode renders: v1.4 hasura-v14 (the init-style default
		// HASURA_VERSION=v2.44.0), v1.5 hasura (an explicit v2.51.0, an override).
		r, ok := compose.LockedRef(compose.HasuraEntry())
		if !ok {
			t.Fatal("no hasura entry in the CLI image lock")
		}
		t.Setenv(compose.EnvImagePinning, compose.PinningLegacy)
		raw, _ := os.ReadFile(filepath.Join(f.workdir, ".env"))
		writeFixtureFile(t, filepath.Join(f.workdir, ".env"), "HASURA_VERSION="+r.Version+"\n"+string(raw), 0o600)
		withExistingCompose(t, f, "services:\n  hasura:\n    image: docker.io/"+r.LegacyRepository()+":"+r.Version+"\n")
		if _, err := Build(f.workdir, BuildOptions{Force: true, WriteLock: true}); err != nil {
			t.Fatalf("Build: %v", err)
		}
		l := readLock(t, f)
		if got := l.Images["hasura"]; got != r.String() {
			t.Fatalf("v15=%v: lock pins %q (unpinned %v), want %q", v15, got, l.Unpinned, r.String())
		}
		if got := composeImagesOf(t, f)["hasura"]; got != r.String() {
			t.Fatalf("v15=%v: compose runs %q, want the pin %q", v15, got, r.String())
		}
	}
}

// writeMirrorOverride writes the file ensureStartImages writes after a mirror fallback.
func writeMirrorOverride(t *testing.T, f *planFixture, ref string) {
	t.Helper()
	writeFixtureFile(t, filepath.Join(f.workdir, docker.ImageOverrideFile), "# GENERATED BY nself\nservices:\n  hasura:\n    image: "+ref+"\n", 0o644)
}

// TestLockMirrorOverride: .nself/state/image-overrides.yml is merged, but an
// entry is accepted only when its digest equals the digest of the reference it
// replaces. A same-digest mirror never enters the lock and never fails the
// frozen comparison; any other override is a difference (CR r3 M1).
func TestLockMirrorOverride(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, true)
	lockBuild(t, f)
	locked := readLock(t, f)
	pin := locked.Images["hasura"]
	digest := lock.RefDigest(pin)
	if digest == "" {
		t.Fatalf("no digest-pinned hasura in the lock: %v", locked.Images)
	}

	// Same digest, registry mirror: the build ignores it, the comparison accepts it.
	writeMirrorOverride(t, f, "mirror.example/hasura/graphql-engine@"+digest)
	lockBuild(t, f)
	if got := readLock(t, f).Images["hasura"]; got != pin {
		t.Fatalf("the mirror reference entered the lock: %q, want %q", got, pin)
	}
	cur, err := CurrentLock(f.workdir)
	if err != nil {
		t.Fatal(err)
	}
	if got := cur.Images["hasura"]; got != pin {
		t.Fatalf("a same-digest mirror override changed the stack: %q, want %q", got, pin)
	}

	// Another digest (a hand edit or a pin that moved): the comparison sees the
	// override; a build does not let it into the lock and then drops it.
	other := "sha256:" + strings.Repeat("9", 64)
	writeMirrorOverride(t, f, "mirror.example/hasura/graphql-engine@"+other)
	cur, err = CurrentLock(f.workdir)
	if err != nil {
		t.Fatal(err)
	}
	if got := cur.Images["hasura"]; got == pin || !strings.Contains(got, other) {
		t.Fatalf("an override with another digest must show in the comparison: %q", got)
	}
	if d := lock.Compare(readLock(t, f), cur); len(d) == 0 {
		t.Fatal("an override with another digest must be a mismatch")
	}
	lockBuild(t, f)
	if got := readLock(t, f).Images["hasura"]; got != pin {
		t.Fatalf("an unlocked override entered the lock: %q", got)
	}

	// A tag-only override has no digest to equal: also a mismatch.
	writeMirrorOverride(t, f, "mirror.example/hasura/graphql-engine:v2.44.0")
	cur, err = CurrentLock(f.workdir)
	if err != nil {
		t.Fatal(err)
	}
	if d := lock.Compare(readLock(t, f), cur); len(d) == 0 {
		t.Fatal("a tag-only override must be a mismatch")
	}
}

// TestLockUsesRediscoveredComposeFiles: the lock is written after the compose
// file list is rediscovered in the same build, so a plugin fragment added in
// this build is in the lock and `start --frozen` agrees (CR/lens should-fix).
func TestLockUsesRediscoveredComposeFiles(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, true)
	lockBuild(t, f)
	frag := filepath.Join(f.plugins, "tweak")
	writeFixtureFile(t, filepath.Join(frag, "plugin.json"), `{"name":"tweak","port":3999,"language":"go"}`, 0o644)
	writeFixtureFile(t, filepath.Join(frag, pluginComposeFilename), "services:\n  hasura:\n    image: hasura/graphql-engine:v2.51.0\n", 0o644)
	lockBuild(t, f)
	l := readLock(t, f)
	if got := l.Unpinned["hasura"] + l.Images["hasura"]; got != "hasura/graphql-engine:v2.51.0" {
		t.Fatalf("a fragment added in this build is missing from the lock: hasura %q", got)
	}
	cur, err := CurrentLock(f.workdir)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range lock.CompareStrict(l, cur) {
		if m.Kind == lock.KindImage {
			t.Fatalf("build and start --frozen disagree: %s", m)
		}
	}
}

// TestLockVariableImageNotPinned: a CLI-owned image built from a variable is not
// pinned in the lock and is reported by name (UnverifiableImages), never
// recorded as if it were verified (adversarial R2).
func TestLockVariableImageNotPinned(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, true)
	writeFixtureFile(t, filepath.Join(f.workdir, "docker-compose.override.yml"), "services:\n  hasura:\n    image: ${HX:-hasura/graphql-engine:v2.44.0}\n", 0o644)
	lockBuild(t, f)
	l := readLock(t, f)
	if v, ok := l.Images["hasura"]; ok {
		t.Fatalf("a variable image was pinned: %q", v)
	}
	if v, ok := l.Unpinned["hasura"]; ok {
		t.Fatalf("a variable image was recorded as tag-only: %q", v)
	}
	bad, err := UnverifiableImages(f.workdir)
	if err != nil || len(bad) != 1 || !strings.Contains(bad[0], "hasura") || !strings.Contains(bad[0], "variable") {
		t.Fatalf("want one entry naming hasura and the variable, got %v %v", bad, err)
	}
	if err := os.Remove(filepath.Join(f.workdir, "docker-compose.override.yml")); err != nil {
		t.Fatal(err)
	}
	if bad, err := UnverifiableImages(f.workdir); err != nil || len(bad) != 0 {
		t.Fatalf("the generated stack has nothing unverifiable: %v %v", bad, err)
	}
}

// TestLockBuildDropsStaleMirror: after the pin moves, a registry-mirror entry
// for the old digest is dropped by the next build (its file is removed when no
// entry is left), an entry that still matches stays, and the stack then agrees
// with the lock (lens r4 should-fix 1).
func TestLockBuildDropsStaleMirror(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, true)
	lockBuild(t, f)
	locked := readLock(t, f)
	pin := locked.Images["hasura"]
	path := filepath.Join(f.workdir, docker.ImageOverrideFile)
	stale := "mirror.example/hasura/graphql-engine@sha256:" + strings.Repeat("4", 64)

	// Only a stale entry: the file goes away and the comparison is clean.
	writeMirrorOverride(t, f, stale)
	if cur, err := CurrentLock(f.workdir); err != nil || len(lock.Compare(locked, cur)) == 0 {
		t.Fatalf("setup: the stale entry must be a mismatch before the build: %v", err)
	}
	lockBuild(t, f)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the stale mirror file survived the build: %v", err)
	}
	cur, err := CurrentLock(f.workdir)
	if err != nil || len(lock.Compare(readLock(t, f), cur)) != 0 {
		t.Fatalf("after the build the stack must match the lock: %v", err)
	}

	// A matching entry stays, a stale one beside it goes.
	var other, otherPin string
	for svc, ref := range locked.Images {
		if svc != "hasura" && lock.RefDigest(ref) != "" {
			other, otherPin = svc, ref
			break
		}
	}
	if other == "" {
		t.Fatal("no second digest-pinned service in the fixture lock")
	}
	body := "# GENERATED BY nself \u2014 DO NOT HAND EDIT\nservices:\n  hasura:\n    image: " + stale + "\n  " + other + ":\n    image: mirror.example/x/y@" + lock.RefDigest(otherPin) + "\n"
	writeFixtureFile(t, path, body, 0o644)
	lockBuild(t, f)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the matching entry must keep the file: %v", err)
	}
	if strings.Contains(string(got), "hasura") || !strings.Contains(string(got), other+":") || !strings.Contains(string(got), lock.RefDigest(otherPin)) {
		t.Fatalf("want only the %s entry left, got:\n%s", other, got)
	}
	if got := readLock(t, f).Images["hasura"]; got != pin {
		t.Fatalf("the lock moved: %q", got)
	}
}

// TestLockUnverifiableKeepsPin: an override whose image cannot be verified (a
// variable, extends, build) does not drop the service's existing lock pin; the
// build keeps the previous entry and warns, so removing the override later
// cannot move the image (CR r4 should-fix 1).
func TestLockUnverifiableKeepsPin(t *testing.T) {
	cases := map[string]string{
		"variable": "services:\n  hasura:\n    image: $HX\n",
		"extends":  "services:\n  hasura:\n    extends:\n      file: other.yml\n      service: hasura\n",
		"build":    "services:\n  hasura:\n    build: ./h\n",
	}
	for name, over := range cases {
		t.Run(name, func(t *testing.T) {
			f := newPlanFixture(t, "dev-minimal")
			compattest.Set(t, true)
			lockBuild(t, f)
			before := readLock(t, f)
			pin := before.Images["hasura"]
			if pin == "" {
				t.Fatalf("no hasura pin in the first lock: %v", before.Images)
			}
			writeFixtureFile(t, filepath.Join(f.workdir, "docker-compose.override.yml"), over, 0o644)
			lockBuild(t, f)
			if got := readLock(t, f).Images["hasura"]; got != pin {
				t.Fatalf("the pin was dropped or moved: %q, want %q", got, pin)
			}
			if err := os.Remove(filepath.Join(f.workdir, "docker-compose.override.yml")); err != nil {
				t.Fatal(err)
			}
			lockBuild(t, f)
			if got := readLock(t, f).Images["hasura"]; got != pin {
				t.Fatalf("the pin moved after the override went away: %q, want %q", got, pin)
			}
		})
	}
}

// captureSlog routes slog output to a buffer for the test.
func captureSlog(t *testing.T) *strings.Builder {
	t.Helper()
	var sb strings.Builder
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&sb, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &sb
}

// TestLockUnverifiableWarnsOnce: one build names each unverifiable image once,
// however many times the lock is computed (CR r4 should-fix 2).
func TestLockUnverifiableWarnsOnce(t *testing.T) {
	f := newPlanFixture(t, "dev-minimal")
	compattest.Set(t, true)
	writeFixtureFile(t, filepath.Join(f.workdir, "docker-compose.override.yml"), "services:\n  hasura:\n    image: $HX\n", 0o644)
	log := captureSlog(t)
	lockBuild(t, f)
	if n := strings.Count(log.String(), "contains a variable"); n != 1 {
		t.Fatalf("the unverifiable image is named %d times in one build, want 1:\n%s", n, log.String())
	}
	st := &buildState{}
	for i := 0; i < 3; i++ {
		st.warnOnce("m", "k", "v")
	}
	if n := strings.Count(log.String(), "msg=m "); n != 1 {
		t.Fatalf("warnOnce printed %d times, want 1", n)
	}
}
