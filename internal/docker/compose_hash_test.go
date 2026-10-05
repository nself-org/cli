package docker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfigHashes(t *testing.T) {
	got, err := parseConfigHashes("hasura abc123\nnginx def456\n\n")
	if err != nil || len(got) != 2 || got["hasura"] != "abc123" || got["nginx"] != "def456" {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, bad := range []string{"hasura", "hasura a b"} {
		if _, err := parseConfigHashes(bad); err == nil {
			t.Errorf("%q must be an error, a dropped service reads as no impact", bad)
		}
	}
}

func TestRunningPsArgsAndParse(t *testing.T) {
	joined := strings.Join(runningPsArgs("myproj"), " ")
	for _, want := range []string{"ps -a", "label=com.docker.compose.project=myproj", "com.docker.compose.config-hash"} {
		if !strings.Contains(joined, want) {
			t.Errorf("ps args lack %q: %s", want, joined)
		}
	}
	rs := parseRunningPs("fx_hasura\trunning\thasura\tabc\nfx_old\texited\told\t\n\n")
	if len(rs) != 2 || rs[0] != (RunningService{Name: "fx_hasura", State: "running", Service: "hasura", ConfigHash: "abc"}) || rs[1].ConfigHash != "" {
		t.Fatalf("parsed %+v", rs)
	}
}

func TestStatefulServices(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yml")
	body := `services:
  db:
    volumes:
      - pgdata:/var/lib/postgresql/data
  web:
    volumes:
      - ./site:/usr/share/nginx/html:ro
      - /etc/ssl:/ssl
      - /anonymous
  cache:
    volumes:
      - type: volume
        source: cachedata
        target: /data
  tmp:
    volumes:
      - type: tmpfs
        target: /tmp
      - type: bind
        source: ./x
        target: /x
  none: {}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := StatefulServices([]string{path, filepath.Join(dir, "missing.yml")})
	if err != nil {
		t.Fatal(err)
	}
	if !got["db"] || !got["cache"] || got["web"] || got["tmp"] || got["none"] || len(got) != 2 {
		t.Fatalf("stateful = %v", got)
	}
	if err := os.WriteFile(path, []byte("services: [unterminated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := StatefulServices([]string{path}); err == nil {
		t.Fatal("invalid YAML must be an error")
	}
}

// TestComposeConfigHashesReal runs the real `docker compose config --hash`
// (client side, no daemon) when it is available: the hash follows an env-file
// value that feeds the service, and not one that does not.
func TestComposeConfigHashesReal(t *testing.T) {
	if out, err := exec.Command("docker", "compose", "config", "--help").CombinedOutput(); err != nil || !strings.Contains(string(out), "--hash") {
		t.Skip("docker compose with config --hash is not available")
	}
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	compose := write("docker-compose.yml", "services:\n  app:\n    image: busybox\n    environment:\n      TOKEN: ${TOKEN}\n  other:\n    image: busybox\n")
	hashes := func(env string) map[string]string {
		h, err := ComposeConfigHashes(context.Background(), []string{compose}, []string{write("e.env", env)}, dir)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	a, b := hashes("TOKEN=one\nUNUSED=1\n"), hashes("TOKEN=two\nUNUSED=2\n")
	if a["app"] == "" || a["other"] == "" || a["app"] == b["app"] || a["other"] != b["other"] {
		t.Fatalf("hashes did not follow the env value: %v vs %v", a, b)
	}
	if _, err := ComposeConfigHashes(context.Background(), []string{filepath.Join(dir, "nope.yml")}, nil, dir); err == nil {
		t.Fatal("a missing compose file must be an error")
	}
}
