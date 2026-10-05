package nginx

// acme_challenge_test.go: the HTTP-01 challenge location (P7-LIVE-24).
// TestAcmeChallengeLocation renders the default server and every route block
// (core, optional, custom-service, internal/plugin) and asserts that each
// server block listening on port 80 carries the shared location exactly once,
// ahead of its catch-all, that SSL route blocks are unchanged, and that the
// text is the one internal/ssl/acme.NginxChallengeLocation hands the
// custom-domain confs. TestAcmeChallengeNginx runs `nginx -t` and the odd
// request cases (traversal, subdirectory, odd token, non-GET) in a container.
// Goldens: testdata/acme/ (UPDATE_GOLDEN=1 rewrites them).

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/ssl/acme"
)

const challengeHead = "location ^~ /.well-known/acme-challenge/ {"

// acmeFiles renders the whole nginx tree for mode ("local" has certificates,
// "letsencrypt" with no certificate on disk has none) with one of each route kind.
func acmeFiles(t *testing.T, mode string) map[string]string {
	t.Helper()
	cfg, err := config.ApplyDefaults(&config.Config{BaseDomain: "example.com", SSLMode: mode})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Admin.Enabled, cfg.Search.Enabled, cfg.Mailpit.Enabled, cfg.Functions.Enabled = true, true, true, true
	cfg.CustomServices = []config.CustomService{{Index: 1, Name: "myapi", Port: 8001, Route: "myapi"}}
	cfg.InternalRoutes = []config.InternalRoute{{Index: 1, Name: "task-auth", Subdomain: "auth.task", Target: "http://ntask_auth:4001"}}
	files, err := NewGenerator(cfg, t.TempDir()).Generate()
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func serverBlocks(conf string) (blocks []string) {
	for _, part := range strings.Split(conf, "\nserver {")[1:] {
		blocks = append(blocks, "server {"+part)
	}
	return blocks
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", "acme", strings.ReplaceAll(name, "/", "__")+".golden")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil { //nolint:gosec // test golden
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p) //nolint:gosec // test golden
	if err != nil {
		t.Fatalf("golden %s: %v (UPDATE_GOLDEN=1 writes it)", p, err)
	}
	if string(want) != got {
		t.Errorf("%s differs from golden %s (UPDATE_GOLDEN=1 rewrites it)\n--- got ---\n%s", name, p, got)
	}
}

func TestAcmeChallengeLocation(t *testing.T) {
	plain := acmeFiles(t, "letsencrypt")
	var names []string
	for n := range plain {
		if strings.HasSuffix(n, ".conf") && (strings.HasPrefix(n, "nginx/sites/") || n == "nginx/conf.d/default.conf") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if len(names) < 8 {
		t.Fatalf("expected the default conf and at least 7 route confs, got %v", names)
	}
	t.Run("every port-80 block carries the location once, before its catch-all", func(t *testing.T) {
		for _, n := range names {
			p80 := 0
			for _, b := range serverBlocks("\n" + plain[n]) {
				if !regexp.MustCompile(`(?m)^\s*listen (\[::\]:)?80\b`).MatchString(b) {
					continue
				}
				p80++
				if c := strings.Count(b, challengeHead); c != 1 {
					t.Errorf("%s: %d challenge locations in a port-80 block, want 1\n%s", n, c, b)
				}
				if i, j := strings.Index(b, challengeHead), strings.Index(b, "location / {"); i < 0 || j < i {
					t.Errorf("%s: challenge location must precede `location / {`", n)
				}
				if !strings.Contains(b, acme.NginxChallengeLocation) {
					t.Errorf("%s: block does not contain acme.NginxChallengeLocation verbatim", n)
				}
			}
			if p80 != 1 {
				t.Errorf("%s: %d port-80 blocks, want 1", n, p80)
			}
			golden(t, n, plain[n])
		}
	})
	t.Run("ssl route blocks are unchanged and the default server still serves the location", func(t *testing.T) {
		ssl := acmeFiles(t, "local")
		for _, n := range []string{"nginx/sites/hasura.conf", "nginx/sites/cs-myapi.conf"} {
			if !strings.Contains(ssl[n], "listen 443 ssl;") || strings.Contains(ssl[n], "acme-challenge") {
				t.Errorf("%s: an SSL route block must not carry the challenge location\n%s", n, ssl[n])
			}
			golden(t, "ssl-"+n, ssl[n])
		}
		def := ssl["nginx/conf.d/default.conf"]
		if strings.Count(def, challengeHead) != 1 || !strings.Contains(def, "return 301 https://$host$request_uri;") {
			t.Errorf("default server with SSL: want one challenge location and the redirect\n%s", def)
		}
		golden(t, "ssl-nginx/conf.d/default.conf", def)
	})
}

func TestAcmeChallengeNginx(t *testing.T) {
	// Only a Linux CI runner is required to have docker; macOS and Windows runners have no Linux containers.
	home, err := os.UserHomeDir() // Colima shares only $HOME
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(home, ".nself-acme-nginx-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	for n, c := range acmeFiles(t, "letsencrypt") {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(n)), 0o755); err != nil { //nolint:gosec // container reads it
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, n), []byte(c), 0o644); err != nil { //nolint:gosec // container reads it
			t.Fatal(err)
		}
	}
	tokens := filepath.Join(root, "ssl", ".acme-webroot", ".well-known", "acme-challenge")
	if err := os.MkdirAll(filepath.Join(tokens, "sub"), 0o755); err != nil { //nolint:gosec // container reads it
		t.Fatal(err)
	}
	for f, body := range map[string]string{"tok-OK_1": "proof", "sub/deep": "deep", "a.b": "dotted"} {
		if err := os.WriteFile(filepath.Join(tokens, f), []byte(body), 0o644); err != nil { //nolint:gosec // container reads it
			t.Fatal(err)
		}
	}
	script, err := os.ReadFile(filepath.Join("testdata", "acme", "probe.sh"))
	if err != nil {
		t.Fatal(err)
	}
	mounts := []docker.Mount{{Source: root + "/nginx/nginx.conf", Destination: "/etc/nginx/nginx.conf", ReadOnly: true}}
	for _, d := range []string{"conf.d", "sites", "includes"} {
		mounts = append(mounts, docker.Mount{Source: root + "/nginx/" + d, Destination: "/etc/nginx/" + d, ReadOnly: true})
	}
	mounts = append(mounts, docker.Mount{Source: root + "/ssl", Destination: "/etc/nginx/ssl", ReadOnly: true})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, stderr, err := docker.RunOneShot(ctx, docker.RunSpec{Image: nginxTestImage, Mounts: mounts, Args: []string{"sh", "-c", string(script)}})
	if err != nil {
		missing := strings.Contains(err.Error(), "executable file not found") || strings.Contains(err.Error(), "Cannot connect to the Docker daemon")
		if ci := os.Getenv("CI") != ""; (!ci && missing) || (ci && runtime.GOOS != "linux") {
			t.Skip("docker not available")
		}
		t.Fatalf("container run failed: %v\n%s\n%s", err, out, stderr)
	}
	if !strings.Contains(out, "PROBE-PASS") {
		t.Fatalf("probe script did not pass:\n%s\n%s", out, stderr)
	}
}

// nginxTestImage is the digest-pinned nginx also used by scripts/ci/acme-pebble.sh.
const nginxTestImage = "nginx@sha256:5616878291a2eed594aee8db4dade5878cf7edcb475e59193904b198d9b830de"
