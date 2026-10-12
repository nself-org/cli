package build

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// treeEntries lists every file and directory under dir with its content (files)
// or "dir", so an empty directory created by a failed build is visible too.
// .nself/op.lock is the build lock, taken before any build step (ruling R7).
func treeEntries(t *testing.T, dir string) string {
	t.Helper()
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(dir, path)
		switch {
		case rel == "." || rel == ".nself" || rel == filepath.Join(".nself", "op.lock"):
			return nil
		case info.IsDir():
			out = append(out, rel+" dir")
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out = append(out, rel+" "+info.Mode().Perm().String()+" "+string(data))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

// TestObjectStoragePreflightNoWrites: a never-built v1.5 project that asks for
// MinIO without the minio plugin fails with E138 before the build persists
// secrets or creates dirs, SSL or nginx files (P7-PLUG-60 acceptance:
// "changes nothing"). v1.4 builds the same project.
func TestObjectStoragePreflightNoWrites(t *testing.T) {
	for _, v15 := range []bool{false, true} {
		t.Run(map[bool]string{false: "v1.4", true: "v1.5"}[v15], func(t *testing.T) {
			f := newPlanFixture(t, "dev-minimal")
			compattest.Set(t, v15) // after the fixture: newPlanFixture isolates the env
			envPath := filepath.Join(f.workdir, ".env")
			raw, err := os.ReadFile(envPath)
			if err != nil {
				t.Fatal(err)
			}
			withMinio := strings.Replace(string(raw), "MINIO_ENABLED=false\n", "MINIO_ENABLED=true\n", 1)
			if withMinio == string(raw) {
				t.Fatal("fixture has no MINIO_ENABLED=false line to flip")
			}
			writeFixtureFile(t, envPath, withMinio, 0o600)
			before := treeEntries(t, f.workdir)
			_, err = Build(f.workdir, BuildOptions{Force: true})
			if !v15 {
				if err != nil {
					t.Fatalf("v1.4 build with core MinIO: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "E138") {
				t.Fatalf("v1.5 build with MinIO and no plugin: want E138, got %v", err)
			}
			if after := treeEntries(t, f.workdir); after != before {
				t.Fatalf("E138 build changed the project tree\n--- before\n%s\n--- after\n%s", before, after)
			}
		})
	}
}
