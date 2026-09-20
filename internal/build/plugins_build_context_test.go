package build

// Purpose: table-driven coverage for normalizeComposeBuildContext
// (plugins_build_context.go), the rewrite that converges every relative
// build.context — mapping form and single-line string form alike — onto
// the canonical ${NSELF_PLUGIN_DIR}/<name>[/<subpath>] shape, the only one
// that resolves correctly once merged as a non-first `docker compose -f`
// file.
// Inputs: real shipped plugin compose fragments (testdata/plugin-compose-
// fixtures/*.yml, copied read-only from plugins/free and plugins-pro/paid)
// plus small synthetic fragments for the untouched/idempotent/rewritten
// shapes.
// Outputs: assertions on the rewritten bytes and on slog.Warn behavior when
// no Dockerfile exists to rewrite against.
// Constraints: the fixtures are copies, not symlinks — this package must
// never depend on the sibling plugins/plugins-pro repos at build or test
// time (they are separate git checkouts, not vendored).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readFixture loads a copied real plugin compose fragment from testdata.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "plugin-compose-fixtures", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// TestNormalizeComposeBuildContext_RealPluginShapes is the table-driven
// regression guard for defect #10 (E2E golden path step 13): each case is a
// real shipped plugin whose docker-compose.plugin.yml declared a
// source-repo-relative build.context that breaks once installed.
func TestNormalizeComposeBuildContext_RealPluginShapes(t *testing.T) {
	cases := []struct {
		name       string
		fixture    string
		pluginName string
		wantCtx    string // expected "context: ..." line after rewrite
		wantDF     string // expected "dockerfile: ..." line after rewrite
	}{
		{
			name:       "cron",
			fixture:    "cron.yml",
			pluginName: "cron",
			wantCtx:    "context: ${NSELF_PLUGIN_DIR}/cron",
			wantDF:     "dockerfile: Dockerfile",
		},
		{
			name:       "push",
			fixture:    "push.yml",
			pluginName: "push",
			wantCtx:    "context: ${NSELF_PLUGIN_DIR}/push",
			wantDF:     "dockerfile: Dockerfile",
		},
		{
			name:       "nself-alert-router",
			fixture:    "nself-alert-router.yml",
			pluginName: "nself-alert-router",
			wantCtx:    "context: ${NSELF_PLUGIN_DIR}/nself-alert-router",
			wantDF:     "dockerfile: Dockerfile",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pluginDir := t.TempDir()
			writeDockerfile(t, pluginDir, tc.pluginName)

			in := readFixture(t, tc.fixture)
			out := string(normalizeComposeBuildContext([]byte(in), pluginDir, tc.pluginName))

			if !strings.Contains(out, tc.wantCtx) {
				t.Errorf("rewritten fragment missing %q:\n%s", tc.wantCtx, out)
			}
			if !strings.Contains(out, tc.wantDF) {
				t.Errorf("rewritten fragment missing %q:\n%s", tc.wantDF, out)
			}
			if strings.Contains(out, "..") {
				t.Errorf("rewritten fragment must not still contain a source-repo-relative path component:\n%s", out)
			}
			if err := assertValidComposeYAML(out); err != nil {
				t.Fatalf("rewritten fragment is not valid YAML: %v\n%s", err, out)
			}

			// Idempotent: re-running on the rewritten bytes must be a no-op.
			twice := normalizeComposeBuildContext([]byte(out), pluginDir, tc.pluginName)
			if string(twice) != out {
				t.Fatalf("rewrite is not idempotent:\nonce:\n%s\n---\ntwice:\n%s", out, twice)
			}
		})
	}
}

// TestNormalizeComposeBuildContext_RelativeShapesRewritten is the
// table-driven regression guard for the defect-#10 follow-up (E2E golden
// path step 13, v1.4.2): docker compose resolves EVERY relative
// build.context against the directory of the FIRST `-f` file, never the
// fragment's own directory, so a bare `.`, `./sub`, or `sub` context is just
// as broken once installed as the already-fixed `..`-escaping shapes — it
// was never "already correct" the way the prior version of this rewrite
// assumed. browser and google (and ~29 other licensed plugins) ship the
// bare `.` shape.
func TestNormalizeComposeBuildContext_RelativeShapesRewritten(t *testing.T) {
	cases := []struct {
		name    string
		context string
		wantCtx string
	}{
		{name: "bare dot", context: ".", wantCtx: "context: ${NSELF_PLUGIN_DIR}/browser"},
		{name: "dot-slash subpath", context: "./web", wantCtx: "context: ${NSELF_PLUGIN_DIR}/browser/web"},
		{name: "bare subpath, no dot-slash", context: "web", wantCtx: "context: ${NSELF_PLUGIN_DIR}/browser/web"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pluginDir := t.TempDir()
			writeDockerfile(t, pluginDir, "browser")

			in := "services:\n  browser:\n    build:\n      context: " + tc.context + "\n      dockerfile: Dockerfile\n    image: nself/nself-browser:latest\n"
			out := string(normalizeComposeBuildContext([]byte(in), pluginDir, "browser"))

			if !strings.Contains(out, tc.wantCtx) {
				t.Errorf("rewritten fragment missing %q:\n%s", tc.wantCtx, out)
			}
			if err := assertValidComposeYAML(out); err != nil {
				t.Fatalf("rewritten fragment is not valid YAML: %v\n%s", err, out)
			}

			// Idempotent: re-running on the rewritten bytes must be a no-op.
			twice := normalizeComposeBuildContext([]byte(out), pluginDir, "browser")
			if string(twice) != out {
				t.Fatalf("rewrite is not idempotent:\nonce:\n%s\n---\ntwice:\n%s", out, twice)
			}
		})
	}
}

// TestNormalizeComposeBuildContext_AbsoluteContextUnchanged verifies an
// absolute filesystem build.context is left completely untouched — it is
// not project-relative at all, so the multi -f-file merge issue this
// rewrite targets never applies to it.
func TestNormalizeComposeBuildContext_AbsoluteContextUnchanged(t *testing.T) {
	pluginDir := t.TempDir()
	writeDockerfile(t, pluginDir, "widget")

	in := `services:
  widget:
    build:
      context: /opt/custom-build-context
      dockerfile: Dockerfile
`
	out := string(normalizeComposeBuildContext([]byte(in), pluginDir, "widget"))
	if out != in {
		t.Fatalf("absolute context fragment must be left untouched:\ngot:\n%s\nwant (unchanged):\n%s", out, in)
	}
}

// TestNormalizeComposeBuildContext_PluginDirSubpathUnchanged verifies the
// canonical shape extended with a subpath —
// `${NSELF_PLUGIN_DIR}/<name>/<subpath>` — is byte-for-byte untouched and
// idempotent, same as the bare `${NSELF_PLUGIN_DIR}/<name>` shape.
func TestNormalizeComposeBuildContext_PluginDirSubpathUnchanged(t *testing.T) {
	pluginDir := t.TempDir()
	writeDockerfile(t, pluginDir, "widget")

	in := `services:
  widget:
    build:
      context: ${NSELF_PLUGIN_DIR}/widget/web
      dockerfile: Dockerfile
`
	out := string(normalizeComposeBuildContext([]byte(in), pluginDir, "widget"))
	if out != in {
		t.Fatalf("${NSELF_PLUGIN_DIR}/<name>/<subpath> fragment must be left untouched:\ngot:\n%s\nwant (unchanged):\n%s", out, in)
	}
}

// TestNormalizeComposeBuildContext_StringFormBuild covers the single-line
// `build: <context>` shorthand (context only, dockerfile implicitly
// "Dockerfile" at that context's root) — the sibling shape to the
// context:/dockerfile: mapping, handled by the same normalizeBuildContext-
// Value logic.
func TestNormalizeComposeBuildContext_StringFormBuild(t *testing.T) {
	cases := []struct {
		name    string
		build   string
		want    string
		changed bool
	}{
		{name: "bare dot rewritten", build: ".", want: "build: ${NSELF_PLUGIN_DIR}/widget", changed: true},
		{name: "subpath rewritten", build: "./web", want: "build: ${NSELF_PLUGIN_DIR}/widget/web", changed: true},
		{name: "already canonical unchanged", build: "${NSELF_PLUGIN_DIR}/widget", want: "build: ${NSELF_PLUGIN_DIR}/widget", changed: false},
		{name: "absolute unchanged", build: "/opt/custom-build-context", want: "build: /opt/custom-build-context", changed: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pluginDir := t.TempDir()
			writeDockerfile(t, pluginDir, "widget")

			in := "services:\n  widget:\n    build: " + tc.build + "\n    image: nself/nself-widget:latest\n"
			out := string(normalizeComposeBuildContext([]byte(in), pluginDir, "widget"))
			want := "services:\n  widget:\n    " + tc.want + "\n    image: nself/nself-widget:latest\n"

			if out != want {
				t.Fatalf("unexpected rewrite:\ngot:\n%s\nwant:\n%s", out, want)
			}
			if err := assertValidComposeYAML(out); err != nil {
				t.Fatalf("rewritten fragment is not valid YAML: %v\n%s", err, out)
			}

			twice := normalizeComposeBuildContext([]byte(out), pluginDir, "widget")
			if string(twice) != out {
				t.Fatalf("rewrite is not idempotent:\nonce:\n%s\n---\ntwice:\n%s", out, twice)
			}
		})
	}
}

// TestNormalizeComposeBuildContext_PluginDirShapeUnchanged verifies the
// shape used by ai/claw/mux/voice — `context: ${NSELF_PLUGIN_DIR}/<name>` +
// `dockerfile: Dockerfile` — is byte-for-byte untouched. This is also the
// output shape of the rewrite itself, so this doubles as an idempotency
// check on a hand-authored (not rewrite-produced) fragment.
func TestNormalizeComposeBuildContext_PluginDirShapeUnchanged(t *testing.T) {
	pluginDir := t.TempDir()
	writeDockerfile(t, pluginDir, "ai")

	in := `services:
  ai:
    build:
      context: ${NSELF_PLUGIN_DIR}/ai
      dockerfile: Dockerfile
    container_name: ${COMPOSE_PROJECT_NAME}_ai
    restart: unless-stopped
`
	out := string(normalizeComposeBuildContext([]byte(in), pluginDir, "ai"))
	if out != in {
		t.Fatalf("${NSELF_PLUGIN_DIR}/<name> fragment must be left untouched:\ngot:\n%s\nwant (unchanged):\n%s", out, in)
	}
}

// TestNormalizeComposeBuildContext_NoDockerfileLeavesUntouchedAndWarns
// verifies that when the plugin directory has no Dockerfile to rewrite
// against, an escaping context is left completely untouched rather than
// guessed at — a Dockerfile that "exists" at the wrong rewritten path would
// trade one opaque build failure for another. This does not assert on the
// slog.Warn call itself (no test hook is wired for it); it asserts the
// documented fallback behavior the warning accompanies.
func TestNormalizeComposeBuildContext_NoDockerfileLeavesUntouchedAndWarns(t *testing.T) {
	pluginDir := t.TempDir() // no Dockerfile written

	in := readFixture(t, "cron.yml")
	out := string(normalizeComposeBuildContext([]byte(in), pluginDir, "cron"))
	if out != in {
		t.Fatalf("fragment must be left untouched when no Dockerfile exists to rewrite against:\ngot:\n%s\nwant (unchanged):\n%s", out, in)
	}
}

// TestNormalizeComposeBuildContext_NonEscapingDockerfileWithDirLeftAlone
// covers the "dockerfile has a directory component" detection signal in
// isolation from the context-escape signal: a context that stays inside the
// plugin directory but a dockerfile value that still names a subdirectory
// must also trigger the rewrite (Docker resolves dockerfile relative to
// context, so a correctly-scoped context never needs a directory prefix).
func TestNormalizeComposeBuildContext_DockerfileDirComponentTriggersRewrite(t *testing.T) {
	pluginDir := t.TempDir()
	writeDockerfile(t, pluginDir, "widget")

	in := `services:
  widget:
    build:
      context: ${NSELF_PLUGIN_DIR}/widget
      dockerfile: sub/Dockerfile
`
	out := string(normalizeComposeBuildContext([]byte(in), pluginDir, "widget"))
	want := `services:
  widget:
    build:
      context: ${NSELF_PLUGIN_DIR}/widget
      dockerfile: Dockerfile
`
	if out != want {
		t.Fatalf("unexpected rewrite:\ngot:\n%s\nwant:\n%s", out, want)
	}
}

// TestNormalizeComposeBuildContext_NoBuildBlockUnchanged verifies fragments
// with no build: at all (image:-only services, e.g. notify/github/stripe)
// pass through untouched.
func TestNormalizeComposeBuildContext_NoBuildBlockUnchanged(t *testing.T) {
	pluginDir := t.TempDir()
	in := `services:
  notify:
    image: nself/nself-notify:latest
    container_name: ${COMPOSE_PROJECT_NAME}_notify
`
	out := string(normalizeComposeBuildContext([]byte(in), pluginDir, "notify"))
	if out != in {
		t.Fatalf("fragment with no build: must be left untouched:\ngot:\n%s\nwant (unchanged):\n%s", out, in)
	}
}
