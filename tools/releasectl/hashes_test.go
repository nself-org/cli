package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	tagAPI  = "gh api repos/nself-org/cli/commits/v1.5.0 --jq .sha"
	runList = "gh run list -R nself-org/cli --workflow release.yml --branch v1.5.0"
	relDown = "gh release download v1.5.0 -R nself-org/cli -p checksums.txt -D "
	runDown = "gh run download 99 -R nself-org/cli -n release-checksums -D "
	formApi = "gh api -H Accept: application/vnd.github.raw repos/nself-org/homebrew-nself/contents/Formula/nself.rb"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "distfix", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// writeSums returns a hook that drops content into the -D directory.
func writeSums(content string) func([]string) {
	return func(args []string) {
		for i, a := range args {
			if a == "-D" {
				os.WriteFile(filepath.Join(args[i+1], "checksums.txt"), []byte(content), 0o644)
			}
		}
	}
}

// TestReleaseLibManifestGolden compares the Go output byte for byte with the output the
// shell functions of hashes.sh produced for the same fixtures.
func TestReleaseLibManifestGolden(t *testing.T) {
	e := newRel(t, nil)
	sums, alt, formula := fixture(t, "checksums.txt"), fixture(t, "checksums-alt.txt"), fixture(t, "nself.rb")
	var b strings.Builder
	b.WriteString("arm64=" + e.r.SumsHash(sums, "arm64") + "\n")
	b.WriteString("amd64=" + e.r.SumsHash(sums, "amd64") + "\n")
	b.WriteString("alt-arm64=" + e.r.SumsHash(alt, "arm64") + "\n")
	b.WriteString("alt-amd64=" + e.r.SumsHash(alt, "amd64") + "\n")
	b.WriteString("formula-version=" + formulaVersion(formula) + "\n")
	b.WriteString("formula-arm=" + FormulaBlockSHA(formula, "arm") + "\n")
	b.WriteString("formula-intel=" + FormulaBlockSHA(formula, "intel") + "\n")
	want, err := os.ReadFile(filepath.Join("testdata", "hashes.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if b.String() != string(want) {
		t.Fatalf("manifest differs from golden\ngot:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestReleaseLibSumsHashUnknownArchIsEmpty(t *testing.T) {
	e := newRel(t, nil)
	if got := e.r.SumsHash(fixture(t, "checksums.txt"), "s390x"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestReleaseLibFindRunUsesRunMatchingTag(t *testing.T) {
	e := newRel(t, map[string][]reply{tagAPI: one(reply{out: "sha1\n"}), runList: one(reply{out: "99\n98\n"})})
	if err := e.r.FindRun(); err != nil || e.r.RunID != "99" {
		t.Fatalf("run %q err %v", e.r.RunID, err)
	}
	if got := e.r.stateGet("run"); got != "99" {
		t.Fatalf("state run %q, want 99", got)
	}
}

func TestReleaseLibFindRunDropsStaleStateRun(t *testing.T) {
	e := newRel(t, map[string][]reply{
		tagAPI: one(reply{out: "sha1\n"}), "gh run view 50": one(reply{out: "other\n"}), runList: one(reply{out: "99\n"}),
	})
	e.r.stateSet("run", "50")
	if err := e.r.FindRun(); err != nil || e.r.RunID != "99" {
		t.Fatalf("run %q err %v", e.r.RunID, err)
	}
	if want := "  run 50 from an earlier attempt is not the build of v1.5.0; looking it up again\n"; e.out.String() != want {
		t.Fatalf("stdout %q, want %q", e.out.String(), want)
	}
}

func TestReleaseLibFindRunNoTagMeansNoRun(t *testing.T) {
	e := newRel(t, map[string][]reply{tagAPI: one(reply{code: 1, stderr: "gh: Not Found (HTTP 404)"})})
	if err := e.r.FindRun(); err != nil || e.r.RunID != "" {
		t.Fatalf("run %q err %v", e.r.RunID, err)
	}
	if e.run.count("gh run list") != 0 {
		t.Fatal("run list must not be called without a tag")
	}
}

func TestReleaseLibFindRunUnreadableTagIsFatal(t *testing.T) {
	e := newRel(t, map[string][]reply{tagAPI: one(reply{code: 1, stderr: "HTTP 500"})})
	err := e.r.FindRun()
	if err == nil || err.Error() != "release: cannot resolve v1.5.0 on cli: HTTP 500" {
		t.Fatalf("got %v", err)
	}
}

func TestReleaseLibEnsureChecksumsPrefersPublishedRelease(t *testing.T) {
	e := newRel(t, map[string][]reply{
		tagAPI: one(reply{out: "sha1\n"}), runList: one(reply{out: "99\n"}),
		relDown: one(reply{hook: writeSums("abc  f\n")}),
	})
	ok, err := e.r.EnsureChecksums()
	b, _ := os.ReadFile(e.r.ChecksumsFile)
	if !ok || err != nil || e.r.ChecksumsSrc != "the published release" || string(b) != "abc  f\n" {
		t.Fatalf("ok %v err %v src %q file %q", ok, err, e.r.ChecksumsSrc, b)
	}
	if e.run.count("gh run download") != 0 {
		t.Fatal("artifact must not be fetched when the release has the file")
	}
}

func TestReleaseLibEnsureChecksumsFallsBackToArtifact(t *testing.T) {
	e := newRel(t, map[string][]reply{
		tagAPI: one(reply{out: "sha1\n"}), runList: one(reply{out: "99\n"}),
		relDown: one(reply{code: 1, stderr: "release not found"}),
		runDown: one(reply{hook: writeSums("abc  f\n")}),
	})
	ok, _ := e.r.EnsureChecksums()
	if !ok || e.r.ChecksumsSrc != "the build artifact of run 99" {
		t.Fatalf("ok %v src %q", ok, e.r.ChecksumsSrc)
	}
}

func TestReleaseLibEnsureChecksumsNeitherSourceCleansUp(t *testing.T) {
	e := newRel(t, map[string][]reply{
		tagAPI: one(reply{code: 1, stderr: "404"}), relDown: one(reply{code: 1, stderr: "nope"}),
	})
	ok, err := e.r.EnsureChecksums()
	if ok || err != nil || e.r.ChecksumsFile != "" {
		t.Fatalf("ok %v err %v file %q", ok, err, e.r.ChecksumsFile)
	}
	if _, serr := os.Stat(filepath.Join(e.r.Work, "checksums")); serr == nil {
		t.Fatal("checksums dir must be removed")
	}
}
