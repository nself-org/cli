package build

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Earlier valid changes must stay invisible when a later render diverges or
// omits an expected artifact, including writes, deletions, modes and mkdirs.
func TestExpectTransactionRejectsWholeRender(t *testing.T) {
	for _, late := range []string{"mismatch", "missing"} {
		t.Run(late, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range []string{"rewrite", "remove", "chmod"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("original"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			exp := &PlannedBuild{Files: map[string]PlannedFile{
				"rewrite":  {Data: []byte("new"), Perm: 0644},
				"new/file": {Data: []byte("created"), Perm: 0600},
				"late":     {Data: []byte("planned"), Perm: 0600},
			}, Removed: []string{"remove"}, Modes: map[string]fs.FileMode{"chmod": 0600}}
			sink := newExpectSink(dir, exp)
			for _, err := range []error{
				sink.MkdirAll("new", 0755), sink.WriteFile("rewrite", []byte("new"), 0644),
				sink.WriteAtomic("new/file", []byte("created"), 0600), sink.Remove("remove"), sink.Chmod("chmod", 0600),
			} {
				if err != nil {
					t.Fatal(err)
				}
			}
			if late == "mismatch" && sink.WriteFile("late", []byte("changed"), 0600) == nil {
				t.Fatal("accepted changed render")
			}
			if err := sink.commit(); err == nil {
				t.Fatal("accepted incomplete render")
			}
			for _, name := range []string{"rewrite", "remove", "chmod"} {
				assertArtifact(t, filepath.Join(dir, name), "original", 0644)
			}
			if _, err := os.Stat(filepath.Join(dir, "new")); !os.IsNotExist(err) {
				t.Fatalf("staged directory leaked: %v", err)
			}
		})
	}
}

func TestExpectTransactionOverlayAndRollback(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "mode"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("original"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	sink := newExpectSink(dir, &PlannedBuild{Files: map[string]PlannedFile{
		"a": {Data: []byte("new"), Perm: 0600}, "created/file": {Data: []byte("new"), Perm: 0600},
	}, Removed: []string{"b"}, Modes: map[string]fs.FileMode{"mode": 0600}})
	for _, err := range []error{sink.WriteAtomic("a", []byte("new"), 0600), sink.WriteAtomic("created/file", []byte("new"), 0600), sink.Remove("b"), sink.Chmod("mode", 0600)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := sink.ReadFile("a"); string(got) != "new" {
		t.Fatal("overlay lost write")
	}
	if _, err := sink.Stat("b"); !os.IsNotExist(err) {
		t.Fatal("overlay lost removal")
	}
	if info, err := sink.Stat("mode"); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("overlay lost chmod")
	}
	cause := errors.New("postvalidation rejected render")
	err := sink.commit(func() error {
		assertArtifact(t, filepath.Join(dir, "a"), "new", 0600)
		return cause
	})
	if !errors.Is(err, cause) {
		t.Fatalf("lost validation error: %v", err)
	}
	for _, name := range []string{"a", "b", "mode"} {
		assertArtifact(t, filepath.Join(dir, name), "original", 0644)
	}
	if _, err := os.Stat(filepath.Join(dir, "created")); !os.IsNotExist(err) {
		t.Fatalf("rollback left directory: %v", err)
	}
}

func TestExpectTransactionPromotionFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "z"), []byte("blocks directory"), 0644); err != nil {
		t.Fatal(err)
	}
	sink := newExpectSink(dir, &PlannedBuild{Files: map[string]PlannedFile{
		"a": {Data: []byte("new"), Perm: 0600}, "z/file": {Data: []byte("new"), Perm: 0600},
	}})
	if err := sink.WriteAtomic("a", []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := sink.WriteAtomic("z/file", []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := sink.commit(); err == nil {
		t.Fatal("expected promotion failure")
	}
	assertArtifact(t, filepath.Join(dir, "a"), "original", 0644)
	assertArtifact(t, filepath.Join(dir, "z"), "blocks directory", 0644)
}

func assertArtifact(t *testing.T, path, want string, mode fs.FileMode) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("%s: got %q, %v; want %q", path, data, err, want)
	}
	info, err := os.Stat(path)
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != mode {
		t.Fatalf("%s: incorrect mode: %v, %v", path, info, err)
	}
}

func TestExpectTransactionSymlinkWritesAndModeRollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require privileges")
	}
	for _, external := range []bool{false, true} {
		t.Run(map[bool]string{false: "internal", true: "external"}[external], func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "target")
			if external {
				target = filepath.Join(t.TempDir(), "target")
			}
			if err := os.WriteFile(target, []byte("original"), 0644); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dir, "link")
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			sink := newExpectSink(dir, &PlannedBuild{Files: map[string]PlannedFile{"link": {Data: []byte("new"), Perm: 0644}}})
			if err := sink.WriteFile("link", []byte("new"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := sink.commit(); err != nil {
				t.Fatal(err)
			}
			assertArtifact(t, target, "new", 0644)
			if got, err := os.Readlink(link); err != nil || got != target {
				t.Fatalf("link replaced: %q %v", got, err)
			}
		})
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	sink := newExpectSink(dir, &PlannedBuild{Modes: map[string]fs.FileMode{"link": 0600}})
	if err := sink.Chmod("link", 0600); err != nil {
		t.Fatal(err)
	}
	if err := sink.commit(func() error { return errors.New("reject") }); err == nil {
		t.Fatal("accepted rejected publication")
	}
	assertArtifact(t, target, "original", 0644)
}

func TestExpectTransactionPermissionBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.computed")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	plan := newMemSink(dir, "")
	if err := plan.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := plan.WriteFile(path, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	exp := plan.snapshot()
	if len(exp.Modes) != 0 || exp.Files[".env.computed"].Perm != 0600 {
		t.Fatalf("conflicting planned modes: %+v", exp)
	}
	sink := newExpectSink(dir, exp)
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
