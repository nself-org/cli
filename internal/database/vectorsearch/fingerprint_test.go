package vectorsearch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiskFingerprint(t *testing.T) {
	if fp, err := DiskFingerprint(t.TempDir()); err != nil || fp != "" {
		t.Fatalf("no metadata: %q, %v", fp, err)
	}
	for _, layout := range []string{"json", "yaml"} {
		t.Run(layout, func(t *testing.T) {
			dir, file := writeFixture(t, layout)
			first, err := DiskFingerprint(dir)
			if err != nil || first == "" {
				t.Fatalf("fingerprint: %q, %v", first, err)
			}
			if again, _ := DiskFingerprint(dir); again != first {
				t.Fatal("fingerprint is not stable")
			}
			if err := os.WriteFile(file, []byte("changed"), 0o644); err != nil {
				t.Fatal(err)
			}
			if got, _ := DiskFingerprint(dir); got == first {
				t.Fatal("a changed file kept the fingerprint")
			}
		})
	}
	// In the directory layout, a change to any file counts, not only sources.yaml.
	dir, _ := writeFixture(t, "yaml")
	first, _ := DiskFingerprint(dir)
	tables := filepath.Join(dir, "hasura", "metadata", "databases", "default", "tables", "public_docs.yaml")
	if err := os.MkdirAll(filepath.Dir(tables), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tables, []byte("table: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := DiskFingerprint(dir); got == first {
		t.Fatal("a new file under hasura/metadata kept the fingerprint")
	}
}

// TestMirrorIfUnchangedRefusesStaleDisk: a fingerprint taken before another
// writer changed disk makes the mirror write nothing.
func TestMirrorIfUnchangedRefusesStaleDisk(t *testing.T) {
	for _, layout := range []string{"json", "yaml"} {
		t.Run(layout, func(t *testing.T) {
			dir, file := writeFixture(t, layout)
			stale, err := DiskFingerprint(dir)
			if err != nil {
				t.Fatal(err)
			}
			other, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			other = append(other, '\n') // a concurrent export that changed only formatting
			if err := os.WriteFile(file, other, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, _, err := MirrorFunctionIfUnchanged(dir, "public", "docs_nearest_embedding", []string{"user"}, stale); !errors.Is(err, ErrDiskChanged) {
				t.Fatalf("err = %v, want ErrDiskChanged", err)
			}
			if got, _ := os.ReadFile(file); string(got) != string(other) {
				t.Fatal("the mirror wrote over the other writer's file")
			}
			fresh, _ := DiskFingerprint(dir)
			if _, changed, err := MirrorFunctionIfUnchanged(dir, "public", "docs_nearest_embedding", []string{"user"}, fresh); err != nil || !changed {
				t.Fatalf("fresh fingerprint: changed=%v err=%v", changed, err)
			}
		})
	}
}

// TestRunTrackSkipsMirrorWhenDiskChanges covers D-0369: disk changes after the
// drift check (here, during the live track call), so track leaves it alone.
func TestRunTrackSkipsMirrorWhenDiskChanges(t *testing.T) {
	h := newHarness()
	dir, file := writeFixture(t, "json")
	// A concurrent export that rewrote the file with the default source intact,
	// so only the fingerprint can tell the mirror that disk moved.
	orig, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	concurrent := string(orig) + "\n"
	track := h.deps.Track
	h.deps.Track = func(ctx context.Context, schema, name, table, column string, roles []string) error {
		if err := os.WriteFile(file, []byte(concurrent), 0o644); err != nil {
			t.Fatal(err)
		}
		return track(ctx, schema, name, table, column, roles)
	}
	res, err := Run(t.Context(), h.deps, opts(dir, true))
	if err != nil || !res.Tracked || res.DiskMetadata != "skipped" || !strings.Contains(res.Note, "changed while the function was being tracked") {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if got, _ := os.ReadFile(file); string(got) != concurrent {
		t.Fatalf("disk was rewritten: %s", got)
	}
}
