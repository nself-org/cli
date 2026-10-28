package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestVersionMismatchCannotProjectMetadata(t *testing.T) {
	for _, c := range []struct{ tier, slug string }{{"free", "ai-cli"}, {"licensed", "claw"}} {
		t.Run(c.tier, func(t *testing.T) {
			dir := tree(t, c.tier, c.slug)
			path := filepath.Join(dir, c.slug, "plugin.json")
			m := decodeMap(t, readFile(t, path))
			m["version"], m["dependencies"], m["tables"] = "99.0.0", []string{"unreleased"}, []string{"np_unreleased"}
			body, err := canonical(m)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, body, 0o644); err != nil {
				t.Fatal(err)
			}
			out := t.TempDir()
			args := tierArgs(c.tier, dir, out, "", "-partial")
			expectProblem(t, "manifest version 99.0.0 differs from released version", args...)
			reg := decodeMap(t, readFile(t, filepath.Join(out, "registry.json")))
			if _, ok := reg["plugins"].(map[string]any)[c.slug]; ok {
				t.Fatal("mismatched manifest metadata must never describe released bytes, even with -partial")
			}
		})
	}
}

func catalogGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=Catalog Test", "GIT_AUTHOR_EMAIL=catalog@example.invalid",
		"GIT_COMMITTER_NAME=Catalog Test", "GIT_COMMITTER_EMAIL=catalog@example.invalid")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestCheckRefusesDeletedReleaseRowFromMain(t *testing.T) {
	for _, base := range []string{"refs/heads/main", "refs/remotes/origin/main"} {
		t.Run(base, func(t *testing.T) {
			dir := tree(t, "free", "ai-cli")
			catalogGit(t, dir, "init", "-q")
			catalogGit(t, dir, "add", ".")
			catalogGit(t, dir, "commit", "-qm", "Release fixture")
			catalogGit(t, dir, "update-ref", base, "HEAD")
			writeReleases(t, dir, `{"schema_version":1,"releases":{}}`)
			// Regenerate first so a byte comparison alone cannot detect delisting.
			out := t.TempDir()
			args := tierArgs("free", dir, out, "")
			mustGen(t, args...)
			before := readFile(t, filepath.Join(out, "registry.json"))
			for _, flags := range [][]string{{"-check"}, {"-check", "-partial"}} {
				expectProblem(t, "ai-cli: release row present on main was removed", append(args, flags...)...)
			}
			if !bytes.Equal(before, readFile(t, filepath.Join(out, "registry.json"))) {
				t.Fatal("-check wrote the registry")
			}
		})
	}
}

func TestCheckAllowsNewUnreleasedManifest(t *testing.T) {
	dir := tree(t, "free", "ai-cli")
	writeReleases(t, dir, `{"schema_version":1,"releases":{}}`)
	catalogGit(t, dir, "init", "-q")
	catalogGit(t, dir, "add", ".")
	catalogGit(t, dir, "commit", "-qm", "Unreleased fixture")
	catalogGit(t, dir, "update-ref", "refs/heads/main", "HEAD")
	out := t.TempDir()
	args := tierArgs("free", dir, out, "")
	mustGen(t, args...)
	if code, se := gen(t, append(args, "-check")...); code != 0 || !strings.Contains(se, "has no row") {
		t.Fatalf("new unreleased plugin: exit %d\n%s", code, se)
	}
}
