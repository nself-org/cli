package controlplane

// Tests for P7-DEPL-29: connections made with a shipping-run context are strict
// in v1.4 too (load-balancer helper calls and the preflight architecture read).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// shippingToolsFixture puts a fake ssh that logs its argv (and answers uname -m
// with aarch64), a fake docker that lists an amd64-only image, and a pinned host
// key on PATH. It returns the ssh log path.
func shippingToolsFixture(t *testing.T) (dir, log string) {
	t.Helper()
	skipOnWindows(t)
	compattest.Set(t, false)
	dir = t.TempDir()
	log = filepath.Join(dir, "ssh.log")
	index := `{"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"digest":"sha256:abc","platform":{"os":"linux","architecture":"amd64"}}]}`
	for name, body := range map[string]string{
		"ssh":    "#!/bin/sh\necho \"$@\" >> '" + log + "'\nprintf 'aarch64\\n'\nexit 0\n",
		"docker": "#!/bin/sh\nprintf '%s\\n' '" + index + "'\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	key := filepath.Join(dir, "id_test")
	if err := os.WriteFile(key, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_SHIP_KEY", key)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	stubHostKeyPin(t, dir)
	return dir, log
}

// TestLBHelperStrictInShippingRunV14: Drain called with a shipping-run context
// carries StrictHostKeyChecking=yes in v1.4; without the marker it is accept-new.
func TestLBHelperStrictInShippingRunV14(t *testing.T) {
	_, log := shippingToolsFixture(t)
	srv := Server{Name: "lb-ship", Host: "ubuntu@lb-ship.example.com", SSHKeyRef: "TEST_SHIP_KEY", RemotePath: "/opt/nself"}
	for _, c := range []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"plain run", context.Background(), "StrictHostKeyChecking=accept-new"},
		{"shipping run", WithShippingRun(context.Background()), "StrictHostKeyChecking=yes"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_ = os.Remove(log)
			if _, err := Drain(c.ctx, srv, "webapp"); err != nil {
				t.Fatalf("Drain: %v", err)
			}
			b, _ := os.ReadFile(log)
			if !strings.Contains(string(b), c.want) {
				t.Errorf("argv lacks %s: %s", c.want, b)
			}
			if c.want == "StrictHostKeyChecking=yes" && strings.Contains(string(b), "accept-new") {
				t.Errorf("shipping run used accept-new: %s", b)
			}
		})
	}
}

// TestPreflightShippingStrictV14: the architecture read of a shipping preflight
// is strict in v1.4, while the plain Preflight keeps accept-new.
func TestPreflightShippingStrictV14(t *testing.T) {
	_, log := shippingToolsFixture(t)
	inv := &Inventory{Environments: map[string]Environment{"qa": {Name: "qa", Kind: "remote", Servers: []Server{
		{Name: "arm-host", Host: "u@arm.example.test", Role: RoleApp, SSHKeyRef: "TEST_SHIP_KEY"}}}}}
	for _, c := range []struct {
		name string
		run  func(context.Context, *Inventory, string, []string) error
		want string
	}{
		{"Preflight", Preflight, "StrictHostKeyChecking=accept-new"},
		{"PreflightShipping", PreflightShipping, "StrictHostKeyChecking=yes"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_ = os.Remove(log)
			// One project dir per run: the prober caches the architecture under it.
			compose := filepath.Join(t.TempDir(), "docker-compose.yml")
			if err := os.WriteFile(compose, []byte("services:\n  api:\n    image: example.test/amd64-only:1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			err := c.run(context.Background(), inv, "qa", []string{compose})
			if err == nil || !strings.Contains(err.Error(), "E491") {
				t.Fatalf("want the E491 architecture refusal (proves the read ran), got %v", err)
			}
			b, _ := os.ReadFile(log)
			if !strings.Contains(string(b), c.want) {
				t.Errorf("argv lacks %s: %s", c.want, b)
			}
			if c.want == "StrictHostKeyChecking=yes" && strings.Contains(string(b), "accept-new") {
				t.Errorf("shipping preflight used accept-new: %s", b)
			}
		})
	}
}
