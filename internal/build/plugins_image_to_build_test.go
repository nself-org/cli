package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDockerfile creates <pluginDir>/<pluginName>/Dockerfile so
// canonicalDockerfile (plugins.go) reports a build source is available.
func writeDockerfile(t *testing.T, pluginDir, pluginName string) {
	t.Helper()
	dir := filepath.Join(pluginDir, pluginName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatalf("write Dockerfile: %v", err)
	}
}

// TestNormalizeComposeImageToBuild_NotifyShape mirrors the real
// plugins/free/notify/docker-compose.plugin.yml fragment: a single service
// declaring only `image: nself/nself-notify:latest` with no build: at all.
// nself/nself-notify is never published (404 on Docker Hub), so this must
// become a build: block pointed at the plugin's own Dockerfile.
func TestNormalizeComposeImageToBuild_NotifyShape(t *testing.T) {
	pluginDir := t.TempDir()
	writeDockerfile(t, pluginDir, "notify")

	in := `services:
  notify:
    image: nself/nself-notify:latest
    container_name: ${COMPOSE_PROJECT_NAME}_notify
    restart: unless-stopped
    networks:
      - ${DOCKER_NETWORK}
`
	out := string(normalizeComposeImageToBuild([]byte(in), pluginDir, "notify"))

	want := `services:
  notify:
    build:
      context: ${NSELF_PLUGIN_DIR}/notify
      dockerfile: Dockerfile
    container_name: ${COMPOSE_PROJECT_NAME}_notify
    restart: unless-stopped
    networks:
      - ${DOCKER_NETWORK}
`
	if out != want {
		t.Fatalf("unexpected rewrite:\ngot:\n%s\nwant:\n%s", out, want)
	}
	if err := assertValidComposeYAML(out); err != nil {
		t.Fatalf("rewritten fragment is not valid YAML: %v\n%s", err, out)
	}
}

// TestNormalizeComposeImageToBuild_GoogleShape mirrors
// plugins-pro/paid/google/docker-compose.plugin.yml: the service already
// declares both build: and image: (image: is just the resulting local tag).
// This must be left completely untouched — google already builds from
// source.
func TestNormalizeComposeImageToBuild_GoogleShape(t *testing.T) {
	pluginDir := t.TempDir()
	writeDockerfile(t, pluginDir, "google")

	in := `services:
  nself-google:
    build:
      context: .
      dockerfile: Dockerfile.golang
    image: nself/nself-google:latest
    container_name: nself-google
    restart: unless-stopped
`
	out := string(normalizeComposeImageToBuild([]byte(in), pluginDir, "google"))
	if out != in {
		t.Fatalf("service already declaring build: must be left untouched:\ngot:\n%s\nwant (unchanged):\n%s", out, in)
	}
}

// TestNormalizeComposeImageToBuild_AiShape verifies a fragment that already
// uses build: with no image: line at all (plugins-pro/paid/ai's shape) is a
// no-op.
func TestNormalizeComposeImageToBuild_AiShape(t *testing.T) {
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
	out := string(normalizeComposeImageToBuild([]byte(in), pluginDir, "ai"))
	if out != in {
		t.Fatalf("build:-only fragment must be left untouched:\ngot:\n%s\nwant (unchanged):\n%s", out, in)
	}
}

// TestNormalizeComposeImageToBuild_NoDockerfile verifies that a service
// declaring an nself/* image is left untouched when the plugin directory has
// no Dockerfile to build from — rewriting to build: would just produce a
// different, equally-broken failure.
func TestNormalizeComposeImageToBuild_NoDockerfile(t *testing.T) {
	pluginDir := t.TempDir() // no Dockerfile written

	in := `services:
  notify:
    image: nself/nself-notify:latest
    container_name: ${COMPOSE_PROJECT_NAME}_notify
`
	out := string(normalizeComposeImageToBuild([]byte(in), pluginDir, "notify"))
	if out != in {
		t.Fatalf("fragment without a Dockerfile must be left untouched:\ngot:\n%s\nwant (unchanged):\n%s", out, in)
	}
}

// TestNormalizeComposeImageToBuild_NonNselfImageUntouched verifies a
// third-party image (postgres, redis, ...) is never rewritten, even when it
// sits in the same service block as an nself/* image that legitimately
// changes.
func TestNormalizeComposeImageToBuild_NonNselfImageUntouched(t *testing.T) {
	pluginDir := t.TempDir()
	writeDockerfile(t, pluginDir, "search")

	in := `services:
  search:
    image: nself/nself-search:latest
    container_name: ${COMPOSE_PROJECT_NAME}_search
  meilisearch:
    image: getmeili/meilisearch:v1.6
    container_name: ${COMPOSE_PROJECT_NAME}_meilisearch
`
	out := string(normalizeComposeImageToBuild([]byte(in), pluginDir, "search"))
	if !strings.Contains(out, "image: getmeili/meilisearch:v1.6") {
		t.Fatalf("non-nself image must be untouched, got:\n%s", out)
	}
	if strings.Contains(out, "image: nself/nself-search:latest") {
		t.Fatalf("nself/* image should have been rewritten, got:\n%s", out)
	}
	if !strings.Contains(out, "build:\n      context: ${NSELF_PLUGIN_DIR}/search\n      dockerfile: Dockerfile") {
		t.Fatalf("expected search service to gain a build: block, got:\n%s", out)
	}
	if err := assertValidComposeYAML(out); err != nil {
		t.Fatalf("rewritten fragment is not valid YAML: %v\n%s", err, out)
	}
}

// TestNormalizeComposeImageToBuild_Idempotent verifies re-running the
// rewrite on already-normalized bytes is a byte-for-byte no-op.
func TestNormalizeComposeImageToBuild_Idempotent(t *testing.T) {
	pluginDir := t.TempDir()
	writeDockerfile(t, pluginDir, "backup")

	in := `services:
  backup:
    image: nself/nself-backup:latest
    container_name: ${COMPOSE_PROJECT_NAME}_backup
`
	once := normalizeComposeImageToBuild([]byte(in), pluginDir, "backup")
	twice := normalizeComposeImageToBuild(once, pluginDir, "backup")
	if string(once) != string(twice) {
		t.Fatalf("rewrite is not idempotent:\nonce:\n%s\n---\ntwice:\n%s", once, twice)
	}
	if strings.Count(string(twice), "build:") != 1 {
		t.Fatalf("build: must appear exactly once, got:\n%s", twice)
	}
}

// TestNormalizeComposeDropObsoleteVersion_Quoted verifies the
// `version: "3.8"` + following blank line shape (access-controls, ddns,
// google) is stripped cleanly with no leftover leading blank line.
func TestNormalizeComposeDropObsoleteVersion_Quoted(t *testing.T) {
	in := "version: \"3.8\"\n\nservices:\n  ddns:\n    image: nself/nself-ddns:latest\n"
	out := string(normalizeComposeDropObsoleteVersion([]byte(in)))
	want := "services:\n  ddns:\n    image: nself/nself-ddns:latest\n"
	if out != want {
		t.Fatalf("unexpected strip:\ngot:  %q\nwant: %q", out, want)
	}
}

// TestNormalizeComposeDropObsoleteVersion_SingleQuotedNoBlank verifies the
// `version: '3.8'` shape with no following blank line (browser) is stripped.
func TestNormalizeComposeDropObsoleteVersion_SingleQuotedNoBlank(t *testing.T) {
	in := "version: '3.8'\nservices:\n  browser:\n    build: .\n"
	out := string(normalizeComposeDropObsoleteVersion([]byte(in)))
	want := "services:\n  browser:\n    build: .\n"
	if out != want {
		t.Fatalf("unexpected strip:\ngot:  %q\nwant: %q", out, want)
	}
}

// TestNormalizeComposeDropObsoleteVersion_Absent verifies a fragment with no
// version: key is returned unchanged.
func TestNormalizeComposeDropObsoleteVersion_Absent(t *testing.T) {
	in := "services:\n  notify:\n    image: nself/nself-notify:latest\n"
	out := string(normalizeComposeDropObsoleteVersion([]byte(in)))
	if out != in {
		t.Fatalf("fragment without version: must be unchanged:\ngot:  %q\nwant: %q", out, in)
	}
}

// TestNormalizeComposeDropObsoleteVersion_NotTopLevel verifies a
// coincidental "version:" that is NOT the very first line of the fragment
// (e.g. inside an environment block, hypothetically) is left alone.
func TestNormalizeComposeDropObsoleteVersion_NotTopLevel(t *testing.T) {
	in := "services:\n  notify:\n    environment:\n      APP_VERSION: 1\nversion: \"3.8\"\n"
	out := string(normalizeComposeDropObsoleteVersion([]byte(in)))
	if out != in {
		t.Fatalf("non-leading version: must be left untouched:\ngot:  %q\nwant: %q", out, in)
	}
}

// TestNormalizeComposeDropObsoleteVersion_Idempotent verifies re-running the
// strip on already-stripped bytes is a no-op.
func TestNormalizeComposeDropObsoleteVersion_Idempotent(t *testing.T) {
	in := "version: \"3.8\"\n\nservices:\n  ddns:\n    image: nself/nself-ddns:latest\n"
	once := normalizeComposeDropObsoleteVersion([]byte(in))
	twice := normalizeComposeDropObsoleteVersion(once)
	if string(once) != string(twice) {
		t.Fatalf("strip is not idempotent:\nonce:\n%s\n---\ntwice:\n%s", once, twice)
	}
}
