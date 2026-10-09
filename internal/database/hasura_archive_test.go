package database

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHasuraArchiveRoundTrip(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "metadata"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "metadata", "config.yaml"), []byte("version: 3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := Pack(filepath.Join(source, "metadata"), &b); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "dest", "metadata")
	if err := Unpack(&b, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "config.yaml"))
	if err != nil || string(got) != "version: 3\n" {
		t.Fatalf("round trip: %q, %v", got, err)
	}
	info, err := os.Stat(dest)
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0755) {
		t.Fatalf("metadata directory mode: %v, %v", info, err)
	}
}

func TestHasuraArchiveJSONExportIsApplicable(t *testing.T) {
	var b bytes.Buffer
	if err := packMetadataJSON([]byte(`{"version":3,"sources":[]}`), &b); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "metadata")
	if err := Unpack(&b, dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "sources:") || !strings.Contains(string(data), "version: 3") {
		t.Fatalf("not a self-contained Hasura config: %s", data)
	}
}

func TestHasuraArchiveRejectsUnsafeInclude(t *testing.T) {
	for _, tc := range []struct{ name, value string }{{"escape", "sources: !include ../../secret\n"}, {"absolute", "sources: !include /etc/passwd\n"}, {"cycle", "sources: !include config.yaml\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(tc.value), 0644); err != nil {
				t.Fatal(err)
			}
			if err := validateArchiveIncludes(dir); err == nil {
				t.Fatal("unsafe include accepted")
			}
		})
	}
}

func TestHasuraArchiveRefLeavesCheckout(t *testing.T) {
	root := t.TempDir()
	for _, argv := range [][]string{{"init", root}, {"-C", root, "config", "user.email", "test@example.invalid"}, {"-C", root, "config", "user.name", "Test"}} {
		if out, err := exec.Command("git", argv...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", argv, out, err)
		}
	}
	dir := filepath.Join(root, "hasura", "metadata")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "tables.yaml")
	if err := os.WriteFile(file, []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, argv := range [][]string{{"-C", root, "add", "hasura/metadata"}, {"-C", root, "commit", "-m", "fixture"}} {
		if out, err := exec.Command("git", argv...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", argv, out, err)
		}
	}
	if err := os.WriteFile(file, []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	archive, err := ArchiveMetadataRef(context.Background(), root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "metadata")
	if err := Unpack(bytes.NewReader(archive), dest); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dest, "tables.yaml"))
	working, _ := os.ReadFile(file)
	if strings.TrimSpace(string(got)) != "old" || string(working) != "new\n" {
		t.Fatalf("archive=%q checkout=%q", got, working)
	}
}

func TestHasuraArchiveRejects(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		typ        byte
		size       int64
	}{
		{"absolute", "/metadata/evil", tar.TypeReg, 1},
		{"traversal", "metadata/../evil", tar.TypeReg, 1},
		{"link", "metadata/link", tar.TypeSymlink, 0},
		{"outside", "other/evil", tar.TypeReg, 1},
		{"oversize", "metadata/big", tar.TypeReg, 65 << 20},
		{"dot-git", "metadata/.git/config", tar.TypeReg, 1},
		{"dot-git-case", "metadata/.GIT/HEAD", tar.TypeReg, 1},
		{"dot-hidden", "metadata/.hidden", tar.TypeReg, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			tw := tar.NewWriter(&b)
			if err := tw.WriteHeader(&tar.Header{Name: tc.path, Typeflag: tc.typ, Size: tc.size, Mode: 0644}); err != nil {
				t.Fatal(err)
			}
			if tc.size > 0 {
				if _, err := io.CopyN(tw, strings.NewReader(strings.Repeat("x", int(tc.size))), tc.size); err != nil {
					t.Fatal(err)
				}
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(t.TempDir(), "metadata")
			if err := os.MkdirAll(dest, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dest, "keep"), []byte("safe"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := Unpack(&b, dest); err == nil {
				t.Fatal("accepted malicious archive")
			}
			got, err := os.ReadFile(filepath.Join(dest, "keep"))
			if err != nil || string(got) != "safe" {
				t.Fatalf("destination changed: %q %v", got, err)
			}
		})
	}
}

func TestHasuraArchiveRejectsEmpty(t *testing.T) {
	var b bytes.Buffer
	if err := tar.NewWriter(&b).Close(); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "metadata")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "keep"), []byte("safe"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Unpack(&b, dest); err == nil {
		t.Fatal("empty archive accepted")
	}
	if data, err := os.ReadFile(filepath.Join(dest, "keep")); err != nil || string(data) != "safe" {
		t.Fatalf("destination changed: %q %v", data, err)
	}
}

func TestHasuraArchiveBackupCleanupFailureStillSucceeds(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	dest := filepath.Join(root, "metadata")
	for _, dir := range []string{source, dest} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "new.yaml"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "old.yaml"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := Pack(source, &b); err != nil {
		t.Fatal(err)
	}
	old := removeMetadataBackup
	removeMetadataBackup = func(string) error { return fmt.Errorf("simulated backup cleanup failure") }
	t.Cleanup(func() { removeMetadataBackup = old })
	if err := Unpack(&b, dest); err != nil {
		t.Fatalf("successful swap must remain committable: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, "new.yaml")); err != nil || string(data) != "new" {
		t.Fatalf("swap failed: %q %v", data, err)
	}
}

func TestNormalizeRefArchiveRejectsDotComponents(t *testing.T) {
	for _, name := range []string{"metadata/.git/config", "metadata/.GIT/HEAD", "metadata/nested/.hidden"} {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			w := tar.NewWriter(&b)
			if err := w.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Size: 1}); err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := normalizeRefArchive(&b); err == nil {
				t.Fatal("dot component accepted")
			}
		})
	}
}

// TestNormalizeRefArchiveLimits: the ref archive stream is refused at the
// first entry past the limits, from its header alone (a 65 MiB entry whose
// bytes never arrive, or entry 10001), so nothing large is buffered first.
func TestNormalizeRefArchiveLimits(t *testing.T) {
	var big bytes.Buffer
	tw := tar.NewWriter(&big)
	if err := tw.WriteHeader(&tar.Header{Name: "metadata/huge.yaml", Mode: 0o644, Typeflag: tar.TypeReg, Size: 65 << 20}); err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeRefArchive(bytes.NewReader(big.Bytes())); err == nil || !strings.Contains(err.Error(), "64 MiB") {
		t.Fatalf("a 65 MiB entry must be refused from its header, got %v", err)
	}

	var many bytes.Buffer
	tw = tar.NewWriter(&many)
	for i := 0; i <= maxMetadataEntries; i++ {
		if err := tw.WriteHeader(&tar.Header{Name: fmt.Sprintf("metadata/f%05d.yaml", i), Mode: 0o644, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeRefArchive(bytes.NewReader(many.Bytes())); err == nil || !strings.Contains(err.Error(), "entries") {
		t.Fatalf("entry %d must be refused, got %v", maxMetadataEntries+1, err)
	}

	var ok bytes.Buffer
	tw = tar.NewWriter(&ok)
	_ = tw.WriteHeader(&tar.Header{Name: "metadata/version.yaml", Mode: 0o600, Typeflag: tar.TypeReg, Size: 11})
	_, _ = tw.Write([]byte("version: 3\n"))
	_ = tw.Close()
	if out, err := normalizeRefArchive(bytes.NewReader(ok.Bytes())); err != nil || len(out) == 0 {
		t.Fatalf("a small archive must normalize, got %v", err)
	}
}

// TestApplyMetadataFromGitRejectsOptionRef: a ref git would parse as an
// option (-f, -m) is refused before git runs.
func TestApplyMetadataFromGitRejectsOptionRef(t *testing.T) {
	for _, ref := range []string{"", "-f", "-m", "--orphan=x"} {
		if err := ApplyMetadataFromGit(context.Background(), nil, t.TempDir(), ref); err == nil || !strings.Contains(err.Error(), "invalid git ref") {
			t.Errorf("ref %q: want an invalid git ref error, got %v", ref, err)
		}
	}
}
