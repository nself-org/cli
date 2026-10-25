package entitlement

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
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
	if err := recordVerdict("h1", true); err == nil {
		t.Fatal("a rejection that was not persisted reported success")
	}
}
