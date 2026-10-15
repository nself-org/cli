package build

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nself-org/cli/internal/version"
)

func TestExpectTransactionWriteFileRespectsReadOnlyDestination(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "compose")
	if err := os.WriteFile(path, []byte("original"), 0400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	sink := newExpectSink(dir, &PlannedBuild{Files: map[string]PlannedFile{
		"compose": {Data: []byte("new"), Perm: 0400},
	}})
	if err := sink.WriteFile(path, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := sink.commit(); err == nil {
		t.Fatal("WriteFile replaced a read-only destination")
	}
	assertArtifact(t, path, "original", 0400)
}

func TestHeldFreshCacheOnlySkipsMatchingDisk(t *testing.T) {
	for _, chmodPending := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "pending-chmod"}[chmodPending], func(t *testing.T) {
			if chmodPending && runtime.GOOS == "windows" {
				t.Skip("Windows does not enforce POSIX permission modes")
			}
			defer SnapshotEnv()()
			clearSecretEnv(t)
			t.Setenv("ENV", "dev")
			dir := t.TempDir()
			testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=held-cache\n", 0600)
			initial := newBuildState(dir, BuildOptions{Force: true})
			if _, err := initial.loadValidateConfig(); err != nil {
				t.Fatal(err)
			}
			testWriteFile(t, filepath.Join(dir, "docker-compose.yml"), "services: {}\n", 0600)
			testWriteFile(t, filepath.Join(dir, buildVersionFile), version.GetVersion(), 0644)
			testWriteFile(t, filepath.Join(dir, buildProfileFile), "app", 0644)
			testWriteFile(t, filepath.Join(dir, ".nself", "generated", "routes.json"), "{}", 0644)
			exp := &PlannedBuild{Files: map[string]PlannedFile{"docker-compose.yml": {Data: []byte("services: {}\n"), Perm: 0600}}}
			if runtime.GOOS == "windows" {
				// Windows reports writable env files as 0666, so loading still
				// requests these permission fixes even after the initial build.
				exp.Modes = map[string]os.FileMode{".env": 0600, ".env.secrets": 0600}
			}
			if chmodPending {
				testWriteFile(t, filepath.Join(dir, ".env.computed"), "existing\n", 0400)
				exp.Modes = map[string]os.FileMode{".env.computed": 0600}
			}
			held := newBuildState(dir, BuildOptions{Expect: exp})
			early, err := held.loadValidateConfig()
			if err != nil {
				t.Fatal(err)
			}
			if (early == nil) != chmodPending {
				t.Fatalf("early=%v, pending chmod=%v", early, chmodPending)
			}
		})
	}
}

func TestExpectFreshCacheChecksPublishedRenderWithoutStaging(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose")
	if err := os.WriteFile(path, []byte("planned"), 0600); err != nil {
		t.Fatal(err)
	}
	sink := newExpectSink(dir, &PlannedBuild{Files: map[string]PlannedFile{
		"compose": {Data: []byte("planned"), Perm: 0600},
	}})
	if err := sink.verifyPublished(); err != nil {
		t.Fatal(err)
	}
	if err := sink.verify(); err == nil {
		t.Fatal("cached bytes must not count as staged output")
	}
	if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := sink.verifyPublished(); err == nil {
		t.Fatal("fresh-cache check accepted changed output")
	}
}

func TestExpectTransactionChmodEnablesReadOnlyWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env")
	if err := os.WriteFile(path, []byte("original"), 0400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	sink := newExpectSink(dir, &PlannedBuild{Files: map[string]PlannedFile{
		"env": {Data: []byte("new"), Perm: 0600},
	}})
	if err := sink.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := sink.WriteFile(path, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := sink.commit(); err != nil {
		t.Fatal(err)
	}
	assertArtifact(t, path, "new", 0600)
}
