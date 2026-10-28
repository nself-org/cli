package entitlement

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/nself-org/cli/internal/license"
)

const rejectedHelperEnv = "NSELF_REJECTED_HELPER"

// The child records its own 8 hashes; it is the other process the lock must serialise.
func TestRejectedHelperProcess(t *testing.T) {
	worker := os.Getenv(rejectedHelperEnv)
	if worker == "" {
		t.Skip("helper process only")
	}
	for i := 0; i < 8; i++ {
		if err := recordVerdict(fmt.Sprintf("hash-%s-%d", worker, i), true); err != nil {
			t.Fatal(err)
		}
	}
}

func useRejectedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(dir, "license.json"))
	return dir
}

// Concurrent refreshes in separate processes must not drop each other's rejections.
func TestRecordVerdictAcrossProcessesKeepsEveryRejection(t *testing.T) {
	useRejectedDir(t)
	const workers = 8
	cmds := make([]*exec.Cmd, workers)
	for w := range cmds {
		cmds[w] = exec.Command(os.Args[0], "-test.run=^TestRejectedHelperProcess$")
		cmds[w].Env = append(os.Environ(), rejectedHelperEnv+"="+strconv.Itoa(w+1))
		if err := cmds[w].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("helper: %v", err)
		}
	}
	set, err := loadRejected()
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != workers*8 {
		t.Fatalf("kept %d of %d rejections", len(set), workers*8)
	}
}

func TestLoadRejectedSeparatesAbsentFromCorrupt(t *testing.T) {
	dir := useRejectedDir(t)
	set, err := loadRejected()
	if err != nil || len(set) != 0 {
		t.Fatalf("absent record = %v, %v; want empty and no error", set, err)
	}
	if err := os.WriteFile(filepath.Join(dir, rejectedFile), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRejected(); err == nil {
		t.Fatal("a corrupt record read as empty")
	}
	if _, err := selectToken(nil, nil, nil); err == nil {
		t.Fatal("selection proceeded on an unreadable rejection record")
	}
	if err := recordVerdict("h1", true); err != nil {
		t.Fatalf("a fresh verdict must repair a corrupt record: %v", err)
	}
	if set, err := loadRejected(); err != nil || !set["h1"] {
		t.Fatalf("repaired record = %v, %v", set, err)
	}
}

func TestRecordVerdictReportsWriteFailure(t *testing.T) {
	dir := useRejectedDir(t)
	// A directory at the record path cannot be replaced by the atomic rename, as root or not.
	if err := os.Mkdir(filepath.Join(dir, rejectedFile), 0700); err != nil {
		t.Fatal(err)
	}
	// The marker fallback holds the rejection, so success is honest.
	if err := recordVerdict("h1", true); err != nil {
		t.Fatalf("marker fallback should remember the rejection: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, markerPrefix+"h1"+markerSuffix)); err != nil {
		t.Fatalf("no marker written: %v", err)
	}
	// With the marker path blocked too, nothing remembers the key: the error must surface.
	if err := os.Mkdir(filepath.Join(dir, markerPrefix+"h2"+markerSuffix), 0700); err != nil {
		t.Fatal(err)
	}
	if err := recordVerdict("h2", true); err == nil {
		t.Fatal("a rejection that was not persisted reported success")
	}
}

// A lock that cannot be taken must not lose the rejection: the marker is loaded, and a
// fresh acceptance clears it.
func TestRecordVerdictLockFailureFallsBackToMarker(t *testing.T) {
	dir := useRejectedDir(t)
	if err := os.Mkdir(filepath.Join(dir, rejectedLockFile), 0700); err != nil {
		t.Fatal(err)
	}
	if err := recordVerdict("h1", true); err != nil {
		t.Fatal(err)
	}
	if set, err := loadRejected(); err != nil || !set["h1"] {
		t.Fatalf("rejection lost after lock failure: %v, %v", set, err)
	}
	if err := recordVerdict("h1", false); err == nil {
		t.Log("acceptance recorded despite the lock") // lock failure may surface; marker must still clear
	}
	if set, _ := loadRejected(); set["h1"] {
		t.Fatal("fresh acceptance did not clear the marker")
	}
}

// Recovering from a corrupt record must not forgive the other configured keys.
func TestCorruptRecordKeepsOtherKeysRejectedUntilFreshlyAccepted(t *testing.T) {
	dir := useRejectedDir(t)
	keyA, keyB := "synthetic-key-a", "synthetic-key-b"
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", keyA)
	t.Setenv("NSELF_LICENSE_KEY_1", keyB)
	hA, hB := license.HashKey(keyA), license.HashKey(keyB)
	if err := recordVerdict(hB, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, rejectedFile), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := recordVerdict(hA, false); err != nil {
		t.Fatal(err)
	}
	set, err := loadRejected()
	if err != nil || set[hA] || !set[hB] {
		t.Fatalf("after accepting A: %v, %v; want only B rejected", set, err)
	}
}

// A cache directory whose name holds glob metacharacters must still yield its markers.
func TestLoadRejectedReadsMarkersInDirectoryWithGlobMetacharacters(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache[x")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(dir, "license.json"))
	if err := os.Mkdir(filepath.Join(dir, rejectedLockFile), 0700); err != nil {
		t.Fatal(err)
	}
	if err := recordVerdict("h1", true); err != nil {
		t.Fatal(err)
	}
	if set, err := loadRejected(); err != nil || !set["h1"] {
		t.Fatalf("marker in a bracketed directory was not loaded: %v, %v", set, err)
	}
}

// A cache directory that cannot be listed is unknown state: fail closed, never "no markers".
func TestLoadRejectedFailsClosedWhenDirectoryCannotBeListed(t *testing.T) {
	dir := useRejectedDir(t)
	if err := os.Chmod(dir, 0300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	if _, err := os.ReadDir(dir); err == nil {
		t.Skip("directory listing is not denied for this user (root)")
	}
	if _, err := loadRejected(); err == nil {
		t.Fatal("an unlistable cache directory was read as having no rejection markers")
	}
}

// A marker whose name is exactly prefix+suffix (no hash) must not be loaded:
// len(name) <= len(prefix)+len(suffix) is the guard. Mutation at rejected.go:83:29
// flips <= to <, which would accept an empty-hash marker and produce a "" key in
// the rejection set.
func TestLoadRejectedIgnoresMarkerWithEmptyHash(t *testing.T) {
	dir := useRejectedDir(t)
	empty := filepath.Join(dir, markerPrefix+markerSuffix)
	if err := os.WriteFile(empty, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	set, err := loadRejected()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if set[""] {
		t.Fatal("empty-hash marker was loaded as a rejection")
	}
}

// writeMarker must propagate OpenFile errors: mutating `if err != nil` to
// `if err == nil` at rejected.go:99:9 would swallow the failure and return nil.
func TestWriteMarkerReportsOpenFailure(t *testing.T) {
	dir := useRejectedDir(t)
	// Block the marker path with a directory so OpenFile cannot create it.
	blocked := filepath.Join(dir, markerPrefix+"blocked"+markerSuffix)
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeMarker(filepath.Join(dir, rejectedFile), "blocked"); err == nil {
		t.Fatal("writeMarker did not report OpenFile failure")
	}
}
