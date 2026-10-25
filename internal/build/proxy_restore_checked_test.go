package build

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestProxyRestoreCheckedApplyRollback(t *testing.T) {
	workdir, dir := restoreProject(t, testRendererMarker)
	before := proxyHashes(t, workdir)
	plan := newMemSink(workdir, "")
	if err := cleanupProviderOutput(plan, workdir, ProviderNginx); err != nil {
		t.Fatal(err)
	}
	apply := newExpectSink(workdir, plan.snapshot())
	if err := cleanupProviderOutput(apply, workdir, ProviderNginx); err != nil {
		t.Fatal(err)
	}
	failed := errors.New("validation failed")
	if err := apply.commit(func() error { return failed }); !errors.Is(err, failed) {
		t.Fatalf("commit error: %v", err)
	}
	after := proxyHashes(t, workdir)
	if len(after) != len(before) {
		t.Fatalf("rollback changed output tree under %s: before=%v after=%v", dir, before, after)
	}
	for path, hash := range before {
		if after[path] != hash {
			t.Fatalf("rollback did not restore %s", path)
		}
	}
}

type proxySwapSink struct {
	Sink
	t                     *testing.T
	trigger, path, target string
}

func (s *proxySwapSink) Remove(path string) error {
	s.swap(path)
	return s.Sink.Remove(path)
}

func (s *proxySwapSink) RemoveWithin(root *os.Root, relative, path string) error {
	s.swap(path)
	return s.Sink.RemoveWithin(root, relative, path)
}

func (s *proxySwapSink) swap(path string) {
	if path == s.trigger {
		if err := os.Rename(s.path, s.path+"-original"); err != nil {
			s.t.Fatal(err)
		}
		if err := os.Symlink(s.target, s.path); err != nil {
			s.t.Fatal(err)
		}
		s.trigger = ""
	}
}

func TestProxyRestoreDirectorySwapKeepsSibling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	workdir, dir := restoreProject(t, testRendererMarker)
	outside := filepath.Join(workdir, "unrelated")
	keep := filepath.Join(outside, "static.yml")
	writeFixtureFile(t, keep, "keep me\n", 0644)
	sink := &proxySwapSink{Sink: newDiskSink(workdir), t: t,
		trigger: filepath.Join(dir, "sub", "static.yml"), path: filepath.Join(dir, "sub"), target: outside}
	if err := cleanupProviderOutput(sink, workdir, ProviderNginx); err == nil {
		t.Fatal("cleanup accepted a directory swapped outside the generated tree")
	}
	data, err := os.ReadFile(keep)
	if err != nil || string(data) != "keep me\n" {
		t.Fatalf("cleanup changed unrelated project file: %q, %v", data, err)
	}
}

func TestProxyRestoreCheckedApplyDirectorySwapKeepsSibling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	workdir, dir := restoreProject(t, testRendererMarker)
	outside := filepath.Join(workdir, "unrelated")
	keep := filepath.Join(outside, "static.yml")
	writeFixtureFile(t, keep, "keep me\n", 0644)
	plan := newMemSink(workdir, "")
	if err := cleanupProviderOutput(plan, workdir, ProviderNginx); err != nil {
		t.Fatal(err)
	}
	apply := newExpectSink(workdir, plan.snapshot())
	if err := cleanupProviderOutput(apply, workdir, ProviderNginx); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Rename(sub, sub+"-original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, sub); err != nil {
		t.Fatal(err)
	}
	if err := apply.commit(); err == nil {
		t.Fatal("checked apply accepted a symlink escaping the cleanup directory")
	}
	data, err := os.ReadFile(keep)
	if err != nil || string(data) != "keep me\n" {
		t.Fatalf("checked apply changed unrelated project file: %q, %v", data, err)
	}
}

func TestProxyRestoreDanglingLinkCheckedApply(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	workdir, dir := restoreProject(t, testRendererMarker)
	link := filepath.Join(dir, "dangling")
	if err := os.Symlink(filepath.Join(workdir, "gone"), link); err != nil {
		t.Fatal(err)
	}
	plan := newMemSink(workdir, "")
	if err := cleanupProviderOutput(plan, workdir, ProviderNginx); err != nil {
		t.Fatal(err)
	}
	if !plan.removed[plan.key(link)] {
		t.Fatal("plan omitted dangling symlink removal")
	}
	apply := newExpectSink(workdir, plan.snapshot())
	if err := cleanupProviderOutput(apply, workdir, ProviderNginx); err != nil {
		t.Fatal(err)
	}
	if err := apply.commit(); err != nil {
		t.Fatal(err)
	}
	if exists(dir) {
		t.Fatal("checked apply left generated output behind")
	}
}
