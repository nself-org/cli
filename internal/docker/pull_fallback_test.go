package docker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compose"
	"gopkg.in/yaml.v3"
)

func TestPullFallbackDiagnose(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"manifest unknown: 404", "repository or tag missing"},
		{"unauthorized: 401", "authentication required or repository gone"},
		{"toomanyrequests: 429", "rate limited"},
		{"dial tcp: network is unreachable", "network unavailable"},
	} {
		if got := Diagnose(tc.input); got != tc.want {
			t.Errorf("Diagnose(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
	if !strings.Contains(Diagnose("unknown failure"), "unknown") {
		t.Fatal("unknown failure must not be presented as missing")
	}
}

func TestPullFallbackDiagnoseHTTP(t *testing.T) {
	for _, tc := range []struct {
		code int
		want string
	}{
		{http.StatusUnauthorized, "authentication required or repository gone"},
		{http.StatusTooManyRequests, "rate limited"},
	} {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
				if tc.code == http.StatusUnauthorized {
					w.Header().Set("WWW-Authenticate", `Basic realm="fixture"`)
				}
				http.Error(w, http.StatusText(tc.code), tc.code)
			}))
			defer server.Close()
			response, err := server.Client().Get(server.URL + "/v2/fixture/manifests/sha256:" + strings.Repeat("a", 64))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			got := Diagnose(fmt.Sprintf("registry returned HTTP %d %s", response.StatusCode, response.Status))
			if got != tc.want {
				t.Fatalf("HTTP fixture %d diagnosed %q, want %q", tc.code, got, tc.want)
			}
		})
	}
}

func TestPullFallbackIntegration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("registry:2 has no Windows container manifest")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	dockerCmd := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, "docker", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("docker %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is unavailable on this runner")
	}
	if err := exec.CommandContext(ctx, "docker", "image", "inspect", "registry:2").Run(); err != nil {
		dockerCmd("pull", "registry:2")
	}
	registry := func() (string, string) {
		id := dockerCmd("run", "--rm", "-d", "-P", "-e", "REGISTRY_STORAGE_DELETE_ENABLED=true", "registry:2")
		t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", id).Run() })
		port := dockerCmd("port", id, "5000/tcp")
		port = port[strings.LastIndex(port, ":")+1:]
		return "localhost:" + port, "http://localhost:" + port
	}
	upHost, upURL := registry()
	mirrorHost, mirrorURL := registry()
	upstream, mirror := upHost+"/l18fixture", mirrorHost+"/l18fixture"
	for _, repo := range []string{upstream, mirror} {
		dockerCmd("tag", "registry:2", repo+":test")
		t.Cleanup(func() { _ = exec.Command("docker", "image", "rm", "-f", repo+":test").Run() })
	}
	push := dockerCmd("push", upstream+":test")
	_ = dockerCmd("push", mirror+":test")
	digest := regexp.MustCompile(`digest: (sha256:[0-9a-f]{64})`).FindStringSubmatch(push)
	if len(digest) != 2 {
		t.Fatalf("push did not report digest: %s", push)
	}
	ref := LockedImage{Name: "l18fixture", Repository: upstream, Version: "test", IndexDigest: digest[1], Mirror: &mirror}
	removeLocal := func(repo string) {
		_ = exec.Command("docker", "image", "rm", repo+"@"+digest[1]).Run()
		_ = exec.Command("docker", "image", "rm", repo+":test").Run()
	}
	removeLocal(upstream)
	removeLocal(mirror)
	deleteManifest := func(base string) {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, base+"/v2/l18fixture/manifests/"+digest[1], nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("DELETE manifest: %s", resp.Status)
		}
	}
	deleteManifest(upURL)
	source, err := EnsureImage(ctx, ref)
	if err != nil || source != "mirror" {
		t.Fatalf("upstream deleted: source=%q err=%v", source, err)
	}
	dockerCmd("image", "inspect", mirror+"@"+digest[1])
	project := t.TempDir()
	composeFile := filepath.Join(project, "docker-compose.yml")
	if err := os.WriteFile(composeFile, []byte("services:\n  fixture:\n    image: "+upstream+":test@"+digest[1]+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	composeBin := "docker"
	if err := exec.CommandContext(ctx, "docker", "compose", "version").Run(); err != nil {
		if _, lookupErr := exec.LookPath("docker-compose"); lookupErr != nil {
			t.Fatalf("docker compose and docker-compose unavailable: %v", lookupErr)
		}
		composeBin = filepath.Join(project, "docker-wrapper")
		if err := os.WriteFile(composeBin, []byte("#!/bin/sh\nif [ \"$1\" = compose ]; then shift; exec docker-compose \"$@\"; fi\nexec docker \"$@\"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	c := &Compose{DockerPath: composeBin, ComposeFiles: []string{composeFile}}
	if changed, err := EnsureComposeImages(ctx, project, c, []LockedImage{ref}); err != nil || !changed {
		t.Fatalf("compose mirror override: changed=%v err=%v", changed, err)
	}
	c.ComposeFiles = append(c.ComposeFiles, filepath.Join(project, ImageOverrideFile))
	if err := c.ComposeUp(ctx, project); err != nil {
		t.Fatalf("stack did not start with mirror override: %v", err)
	}
	defer func() { _ = c.ComposeDown(context.Background(), project, DownOptions{}) }()
	services, err := c.ComposePs(ctx, project)
	if err != nil || len(services) != 1 || !strings.Contains(services[0].Image, mirror) {
		t.Fatalf("stack image not served by mirror: services=%+v err=%v", services, err)
	}
	if err := c.ComposeDown(ctx, project, DownOptions{}); err != nil {
		t.Fatalf("stop fixture stack before cache-removal proof: %v", err)
	}
	removeLocal(mirror)
	if _, err := runCapture(ctx, "image", "inspect", mirror+"@"+digest[1]); err == nil {
		t.Fatal("mirror image still cached; both-missing proof would be vacuous")
	}
	deleteManifest(mirrorURL)
	if _, err := EnsureImage(ctx, ref); err == nil || !strings.Contains(err.Error(), "repository or tag missing") {
		t.Fatalf("both registries deleted: %v", err)
	}
	t.Logf("same digest %s served from mirror; both-missing diagnosis verified", fmt.Sprintf("%.19s", digest[1]))
}

func TestEnsureImageOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell fixture")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "docker")
	script := `#!/bin/sh
if [ "$1" = compose ]; then
  printf '{"services":{"postgres":{"image":"%s"}}}\n' "$TEST_IMAGE"
elif [ "$1" = image ]; then
  echo 'No such image' >&2; exit 1
elif [ "$1" = pull ]; then
  if [ "$2" = "$TEST_UPSTREAM" ] && [ "$TEST_SOURCE" = mirror ]; then
    echo 'manifest unknown: 404' >&2; exit 1
  fi
else
  echo 'unexpected docker command' >&2; exit 1
fi
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ref, ok := compose.LockedRef("postgres")
	if !ok || ref.Mirror == nil {
		t.Fatal("postgres lock and mirror required")
	}
	upstream := ref.Repository + "@" + ref.IndexDigest
	mirror := *ref.Mirror + "@" + ref.IndexDigest
	t.Setenv("TEST_UPSTREAM", upstream)
	t.Setenv("TEST_IMAGE", ref.String())
	t.Setenv("TEST_SOURCE", "mirror")
	// The project override is valid Compose YAML, uses the same digest, and
	// carries a generated marker. No docker tag is invoked by the fake.
	locked := LockedImage{Name: ref.Name, Repository: ref.Repository, Version: ref.Version, IndexDigest: ref.IndexDigest, Mirror: ref.Mirror}
	changed, err := EnsureComposeImages(context.Background(), dir, &Compose{DockerPath: bin}, []LockedImage{locked})
	if err != nil || !changed {
		t.Fatalf("EnsureComposeImages mirror = %v, %v", changed, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ImageOverrideFile))
	if err != nil || !strings.Contains(string(data), "GENERATED BY") {
		t.Fatalf("override = %q, %v", data, err)
	}
	var override struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &override); err != nil || override.Services["postgres"].Image != mirror {
		t.Fatalf("override image = %q, want %q (parse error: %v)", override.Services["postgres"].Image, mirror, err)
	}
	t.Setenv("TEST_IMAGE", mirror)
	t.Setenv("TEST_SOURCE", "upstream")
	if changed, err := EnsureComposeImages(context.Background(), dir, &Compose{DockerPath: bin}, []LockedImage{locked}); err != nil || !changed {
		t.Fatalf("stale override not removed: %v, %v", changed, err)
	}
}
