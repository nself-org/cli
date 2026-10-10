package build

// lockfile_test.go — tests for the project lock in the build pipeline
// (P7-LIVE-09): deterministic and change-only writes, bootstrap from an
// existing compose, v1.4 opt-in, precedence of explicit env over pins, and
// plan mode.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
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
}

// TestLockV14OptIn: v1.4 writes no lock unless build --write-lock opts in; once
// the file exists it is honoured and updated.
func TestLockV14OptIn(t *testing.T) {
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
	hasura, _ := compose.LockedRef("hasura")
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
