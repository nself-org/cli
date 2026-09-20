package build

// Purpose: regression coverage for normalizeComposeDockerfile (plugins.go),
// the legacy Rust->Go Dockerfile-name migration cleanup, and for the
// basename-preserving dockerfile resolution normalizeComposeBuildContext
// (plugins_image_to_build.go) now uses via resolveDockerfileName.
// Inputs: a real shipped fixture (testdata/plugin-compose-fixtures/claw.yml,
// copied read-only from plugins-pro/paid/claw) plus small synthetic
// fragments for the legacy-rewrite and escaping-context-with-.golang shapes.
// Outputs: assertions that a bare "Dockerfile.golang"/"Dockerfile.go" value
// is preserved byte-for-byte when the referenced file actually exists, and
// is rewritten (never substring-edited) only when it does not.
// Constraints: this is the regression guard for E2E golden path step 13,
// PR #439 follow-up — normalizeComposeDockerfile used to do
// bytes.ReplaceAll([]byte("dockerfile: Dockerfile.go"), ...) against the raw
// fragment. "Dockerfile.go" is a literal prefix of "Dockerfile.golang", so
// that ReplaceAll also matched inside "dockerfile: Dockerfile.golang" and
// corrupted it to "dockerfile: Dockerfilelang" — docker then failed with
// "open Dockerfilelang: no such file or directory" building the claw plugin.
// mux, google, podcast and post ship the identical bare-".golang" shape and
// were silently corrupted by the same bug.

import (
	"os"
	"path/filepath"
	"testing"
)

// writeDockerfileNamed creates <pluginDir>/<pluginName>/<filename> so
// canonicalDockerfile / resolveDockerfileName / os.Stat checks in the code
// under test see it as present.
func writeDockerfileNamed(t *testing.T, pluginDir, pluginName, filename string) {
	t.Helper()
	dir := filepath.Join(pluginDir, pluginName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
}

// TestNormalizeComposeDockerfile_ClawShape_GolangPreservedByteForByte is the
// direct regression test for the reported defect: claw's real compose
// fragment declares `dockerfile: Dockerfile.golang` inside a non-escaping
// `context: ${NSELF_PLUGIN_DIR}/claw` block, and the plugin ships BOTH
// Dockerfile and Dockerfile.golang. The fragment must come out byte-for-byte
// identical — no substring edit, no clobbering "Dockerfile.golang" into
// "Dockerfilelang".
func TestNormalizeComposeDockerfile_ClawShape_GolangPreservedByteForByte(t *testing.T) {
	pluginDir := t.TempDir()
	writeDockerfileNamed(t, pluginDir, "claw", "Dockerfile")
	writeDockerfileNamed(t, pluginDir, "claw", "Dockerfile.golang")

	in := readFixture(t, "claw.yml")
	out := string(normalizeComposeDockerfile([]byte(in), pluginDir, "claw"))

	if out != in {
		t.Fatalf("dockerfile: Dockerfile.golang must be preserved byte-for-byte when the file exists:\ngot:\n%s\nwant (unchanged):\n%s", out, in)
	}
	if got := countSubstring(out, "Dockerfilelang"); got != 0 {
		t.Fatalf("fragment corrupted: found %d occurrence(s) of the truncated value 'Dockerfilelang':\n%s", got, out)
	}

	// The full pipeline order matters: normalizeComposeImageToBuild and
	// normalizeComposeBuildContext run after normalizeComposeDockerfile in
	// DiscoverPluginComposeFiles (plugins.go) — assert the whole chain is
	// also a no-op on this already-correct fragment.
	chained := normalizeComposeImageToBuild([]byte(out), pluginDir, "claw")
	chained = normalizeComposeBuildContext(chained, pluginDir, "claw")
	if string(chained) != in {
		t.Fatalf("full normalization pipeline must leave the claw fragment untouched:\ngot:\n%s\nwant (unchanged):\n%s", chained, in)
	}
}

// TestNormalizeComposeDockerfile_LegacyValueRewrittenOnlyWhenFileMissing
// covers both legacy names named in the bug report: a bare "Dockerfile.go"
// and a bare "Dockerfile.golang" value are rewritten to the canonical
// "Dockerfile" only when the legacy file does NOT actually exist (mirrors
// plugins/free/podcast's real shape: it references "Dockerfile.golang" but
// only ships a plain "Dockerfile") — and left untouched when it does.
func TestNormalizeComposeDockerfile_LegacyValueRewrittenOnlyWhenFileMissing(t *testing.T) {
	cases := []struct {
		name           string
		legacyValue    string
		legacyExists   bool
		wantDockerfile string
	}{
		{name: "dot-go missing file gets rewritten", legacyValue: "Dockerfile.go", legacyExists: false, wantDockerfile: "Dockerfile"},
		{name: "dot-go existing file preserved", legacyValue: "Dockerfile.go", legacyExists: true, wantDockerfile: "Dockerfile.go"},
		{name: "golang missing file gets rewritten (podcast shape)", legacyValue: "Dockerfile.golang", legacyExists: false, wantDockerfile: "Dockerfile"},
		{name: "golang existing file preserved (claw/mux/google/post shape)", legacyValue: "Dockerfile.golang", legacyExists: true, wantDockerfile: "Dockerfile.golang"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pluginDir := t.TempDir()
			writeDockerfileNamed(t, pluginDir, "widget", "Dockerfile")
			if tc.legacyExists {
				writeDockerfileNamed(t, pluginDir, "widget", tc.legacyValue)
			}

			in := "services:\n  widget:\n    build:\n      context: ${NSELF_PLUGIN_DIR}/widget\n      dockerfile: " + tc.legacyValue + "\n"
			want := "services:\n  widget:\n    build:\n      context: ${NSELF_PLUGIN_DIR}/widget\n      dockerfile: " + tc.wantDockerfile + "\n"

			out := string(normalizeComposeDockerfile([]byte(in), pluginDir, "widget"))
			if out != want {
				t.Fatalf("unexpected rewrite:\ngot:\n%s\nwant:\n%s", out, want)
			}

			// Idempotent.
			twice := string(normalizeComposeDockerfile([]byte(out), pluginDir, "widget"))
			if twice != out {
				t.Fatalf("rewrite is not idempotent:\nonce:\n%s\n---\ntwice:\n%s", out, twice)
			}
		})
	}
}

// TestResolveDockerfileName_BasenamePreservedOverGenericDockerfile is the
// unit-level guard for the CORRECT RULE governing an escaping-context
// rewrite: when the context escapes the plugin dir, the rewritten dockerfile
// value must become the BASENAME of the originally-authored value (when that
// file exists at the plugin root), not unconditionally "Dockerfile".
func TestResolveDockerfileName_BasenamePreservedOverGenericDockerfile(t *testing.T) {
	t.Run("golang basename exists -> preserved", func(t *testing.T) {
		pluginDir := t.TempDir()
		writeDockerfileNamed(t, pluginDir, "nself-alert-router", "Dockerfile.golang")
		got := resolveDockerfileName(pluginDir, "nself-alert-router", "paid/nself-alert-router/Dockerfile.golang")
		if got != "Dockerfile.golang" {
			t.Fatalf("got %q, want %q", got, "Dockerfile.golang")
		}
	})

	t.Run("dot-go basename exists -> preserved", func(t *testing.T) {
		pluginDir := t.TempDir()
		writeDockerfileNamed(t, pluginDir, "widget", "Dockerfile.go")
		got := resolveDockerfileName(pluginDir, "widget", "free/widget/Dockerfile.go")
		if got != "Dockerfile.go" {
			t.Fatalf("got %q, want %q", got, "Dockerfile.go")
		}
	})

	t.Run("basename missing, canonical Dockerfile exists -> falls back", func(t *testing.T) {
		pluginDir := t.TempDir()
		writeDockerfileNamed(t, pluginDir, "cron", "Dockerfile")
		got := resolveDockerfileName(pluginDir, "cron", "free/cron/Dockerfile")
		if got != "Dockerfile" {
			t.Fatalf("got %q, want %q", got, "Dockerfile")
		}
	})

	t.Run("neither exists -> empty, caller must warn and leave untouched", func(t *testing.T) {
		pluginDir := t.TempDir() // no Dockerfile written at all
		got := resolveDockerfileName(pluginDir, "ghost", "free/ghost/Dockerfile.golang")
		if got != "" {
			t.Fatalf("got %q, want empty (no safe target)", got)
		}
	})
}

// TestNormalizeComposeBuildContext_EscapingContextPreservesGolangBasename
// exercises resolveDockerfileName through the full
// normalizeComposeBuildContext rewrite path: an escaping source-repo-layout
// context with a "Dockerfile.golang" dockerfile value must be rewritten to
// the installed-layout context with dockerfile: Dockerfile.golang — the
// BASENAME — not the generic "Dockerfile", when Dockerfile.golang exists at
// the plugin root.
func TestNormalizeComposeBuildContext_EscapingContextPreservesGolangBasename(t *testing.T) {
	pluginDir := t.TempDir()
	writeDockerfileNamed(t, pluginDir, "widget", "Dockerfile.golang")

	in := `services:
  widget:
    build:
      context: ${NSELF_PLUGIN_DIR}/../..
      dockerfile: free/widget/Dockerfile.golang
`
	out := string(normalizeComposeBuildContext([]byte(in), pluginDir, "widget"))
	want := `services:
  widget:
    build:
      context: ${NSELF_PLUGIN_DIR}/widget
      dockerfile: Dockerfile.golang
`
	if out != want {
		t.Fatalf("unexpected rewrite:\ngot:\n%s\nwant:\n%s", out, want)
	}

	// Idempotent: the rewritten shape has no dir component and a
	// non-escaping context, so a second pass must be a byte-for-byte no-op.
	twice := normalizeComposeBuildContext([]byte(out), pluginDir, "widget")
	if string(twice) != out {
		t.Fatalf("rewrite is not idempotent:\nonce:\n%s\n---\ntwice:\n%s", out, twice)
	}
}

// TestNormalizeComposeBuildContext_EscapingContextDotGoBasename is the same
// check for the "Dockerfile.go" legacy name (as opposed to "Dockerfile.golang").
func TestNormalizeComposeBuildContext_EscapingContextDotGoBasename(t *testing.T) {
	pluginDir := t.TempDir()
	writeDockerfileNamed(t, pluginDir, "widget", "Dockerfile.go")

	in := `services:
  widget:
    build:
      context: ../..
      dockerfile: free/widget/Dockerfile.go
`
	out := string(normalizeComposeBuildContext([]byte(in), pluginDir, "widget"))
	want := `services:
  widget:
    build:
      context: ${NSELF_PLUGIN_DIR}/widget
      dockerfile: Dockerfile.go
`
	if out != want {
		t.Fatalf("unexpected rewrite:\ngot:\n%s\nwant:\n%s", out, want)
	}
}

// countSubstring returns how many times sub occurs in s.
func countSubstring(s, sub string) int {
	count := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			count++
		}
	}
	return count
}
