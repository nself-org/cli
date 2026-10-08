package database

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	if string(got) != "old\n" || string(working) != "new\n" {
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
