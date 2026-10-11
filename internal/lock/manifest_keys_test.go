package lock

// manifest_keys_test.go — LoadSources honours the compose manifest exactly as
// start runs it, and Parse rejects a lock that lacks a required key
// (P7-LIVE-09 round 10). Files only under t.TempDir.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCompose(t *testing.T, dir, name, image string) {
	t.Helper()
	body := "services:\n  hasura:\n    image: " + image + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func hasuraOf(t *testing.T, files []SourceFile) string {
	t.Helper()
	m, err := MergeDetailed(files, func(s string) bool { return s == "hasura" }, true)
	if err != nil {
		t.Fatal(err)
	}
	return m.Images["hasura"]
}

// TestLoadSourcesFollowsManifestOrder: a manifest [extra, root] merges extra
// first and root last, so root wins, exactly as start runs it; the build-written
// shape [root, extra] still lets extra win.
func TestLoadSourcesFollowsManifestOrder(t *testing.T) {
	dir := t.TempDir()
	writeCompose(t, dir, "docker-compose.yml", "h/root:1@"+digA)
	writeCompose(t, dir, "extra.yml", "h/extra:1@"+digB)
	root, extra := filepath.Join(dir, "docker-compose.yml"), filepath.Join(dir, "extra.yml")

	files, err := LoadSources(dir, nil, []string{extra, root}, "")
	if err != nil || len(files) != 2 || files[0].Name != "extra.yml" || files[1].Name != "docker-compose.yml" {
		t.Fatalf("hand-edited order: %v %v", files, err)
	}
	if got := hasuraOf(t, files); got != "h/root:1@"+digA {
		t.Errorf("winner = %s, want the root image (last wins, as start runs)", got)
	}

	files, err = LoadSources(dir, nil, []string{root, extra}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := hasuraOf(t, files); got != "h/extra:1@"+digB {
		t.Errorf("build-written order winner = %s, want the extra image", got)
	}
}

// TestLoadSourcesManifestWithoutRoot: a manifest that omits docker-compose.yml is
// an error; an empty one falls back to the root compose alone.
func TestLoadSourcesManifestWithoutRoot(t *testing.T) {
	dir := t.TempDir()
	writeCompose(t, dir, "docker-compose.yml", "h/root:1@"+digA)
	writeCompose(t, dir, "extra.yml", "h/extra:1@"+digB)
	_, err := LoadSources(dir, nil, []string{filepath.Join(dir, "extra.yml")}, "")
	if err == nil || !strings.Contains(err.Error(), "does not include docker-compose.yml") {
		t.Errorf("manifest without root: %v", err)
	}
	files, err := LoadSources(dir, nil, nil, "")
	if err != nil || len(files) != 1 || files[0].Name != "docker-compose.yml" {
		t.Errorf("empty manifest: %v %v", files, err)
	}
}

// TestLoadSourcesMatchesRootByFileIdentity: a manifest that spells the project
// dir through a symlink still finds the root compose (no spurious error), and
// the base override replaces the root bytes, not a second read of the file.
func TestLoadSourcesMatchesRootByFileIdentity(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "proj")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeCompose(t, real, "docker-compose.yml", "h/disk:1@"+digA)
	viaLink := filepath.Join(link, "docker-compose.yml")
	base := []byte("services:\n  hasura:\n    image: h/base:1@" + digB + "\n")
	files, err := LoadSources(real, base, []string{viaLink}, "")
	if err != nil {
		t.Fatalf("symlinked manifest: %v", err)
	}
	if len(files) != 1 || hasuraOf(t, files) != "h/base:1@"+digB {
		t.Errorf("base override not applied to symlinked root: %v", files)
	}
}

// TestParseRequiresEveryKey: removing any required top-level key (or leaving it
// null) is a parse error naming the key; a valid lock and empty collections parse.
func TestParseRequiresEveryKey(t *testing.T) {
	good, err := sample().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(good); err != nil {
		t.Fatalf("valid lock: %v", err)
	}
	empty, err := New().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(empty); err != nil {
		t.Fatalf("empty collections: %v", err)
	}
	for _, key := range []string{"schema_version", "cli", "images", "unpinned", "plugins", "bundles"} {
		var kept, nulled []string
		for _, line := range strings.Split(string(empty), "\n") {
			if strings.HasPrefix(line, key+":") {
				nulled = append(nulled, key+":")
				continue
			}
			kept = append(kept, line)
			nulled = append(nulled, line)
		}
		for name, body := range map[string]string{"absent": strings.Join(kept, "\n"), "null": strings.Join(nulled, "\n")} {
			if _, err := Parse([]byte(body)); err == nil || !strings.Contains(err.Error(), key) {
				t.Errorf("%s %s: err = %v", key, name, err)
			}
		}
	}
}
