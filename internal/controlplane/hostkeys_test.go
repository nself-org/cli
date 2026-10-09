package controlplane

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

func TestHostKeyPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX command stubs")
	}
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	bin := t.TempDir()
	key := "host.example ssh-ed25519 AQID"
	keyscan := "#!/bin/sh\nprintf '%s\\n' '" + key + "'\n"
	keygen := "#!/bin/sh\nif [ -f \"$4\" ]; then /bin/cat \"$4\"; fi\n"
	for name, script := range map[string]string{"ssh-keyscan": keyscan, "ssh-keygen": keygen} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("NSELF_V15", "0")
	opts, err := HostKeyOptions(context.Background(), "qa", "web", TierLocalServers, "u@host.example", true)
	if err != nil || !strings.Contains(strings.Join(opts, " "), "accept-new") {
		t.Fatalf("v1.4: %v %q", err, opts)
	}
	t.Setenv("NSELF_V15", "1")
	_, err = HostKeyOptions(context.Background(), "qa", "web", TierLocalServers, "u@host.example", true)
	if err == nil || !strings.Contains(err.Error(), "E487") || !strings.Contains(err.Error(), "env target add qa web --trust-host-key SHA256:") {
		t.Fatalf("unknown: %v", err)
	}
	if _, statErr := os.Stat(DeployKnownHostsPath()); !os.IsNotExist(statErr) {
		t.Fatal("unknown key changed pin file")
	}
	fp, err := remote.Fingerprint(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := TrustHostKey(context.Background(), "u@host.example", "SHA256:wrong"); err == nil {
		t.Fatal("wrong fingerprint accepted")
	}
	if _, statErr := os.Stat(DeployKnownHostsPath()); !os.IsNotExist(statErr) {
		t.Fatal("wrong fingerprint changed pin file")
	}
	if err := TrustHostKey(context.Background(), "u@host.example", fp); err != nil {
		t.Fatal(err)
	}
	opts, err = HostKeyOptions(context.Background(), "qa", "web", TierLocalServers, "u@host.example", true)
	if err != nil || !strings.Contains(strings.Join(opts, " "), "StrictHostKeyChecking=yes") {
		t.Fatalf("trusted: %v %q", err, opts)
	}
	if !strings.HasPrefix(DeployKnownHostsPath(), filepath.Join(dir, ".config")) {
		t.Fatal("pin path outside HOME")
	}
}

func TestHostKeyIdentityFromProjectRoot(t *testing.T) {
	root := t.TempDir()
	host := "u@host.example:2222"
	inv := &Inventory{SchemaVersion: 2, Environments: map[string]Environment{
		"local": {Name: "local", Kind: "local", Tier: TierLocal},
		"live":  {Name: "live", Kind: "remote", Tier: TierProd, Servers: []Server{{Name: "web", Role: RoleApp, Host: host, RemotePath: "/opt/nself", Primary: true}}},
	}}
	if err := Write(root, inv); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "nested")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(child)
	env, server := hostKeyIdentity("prod", "primary", host)
	if env != "live" || server != "web" {
		t.Fatalf("inventory identity = %q/%q, want live/web", env, server)
	}
}

func TestReadOnlyProdTierRequiresKnownHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX stubs")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NSELF_V15", "1")
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "ssh-keyscan"), []byte("#!/bin/sh\nprintf 'prod.example ssh-ed25519 AQID\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	if _, err := HostKeyOptions(context.Background(), "live", "web", TierProd, "u@prod.example", false); err == nil || !strings.Contains(err.Error(), "E487") {
		t.Fatalf("read-only prod host accepted unknown key: %v", err)
	}
	if _, err := HostKeyOptions(context.Background(), "production", "web", TierLocalServers, "u@prod.example", false); err == nil || !strings.Contains(err.Error(), "E487") {
		t.Fatalf("prod-name host accepted unknown key: %v", err)
	}
}

func TestProbeTierLoadErrorFailsClosed(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".nself"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".nself", "control-plane.yaml"), []byte("{bad yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	if tier, _ := probeTier(root, Server{Name: "web", Host: "u@host.example"}); tier != TierProd {
		t.Fatalf("unreadable inventory tier = %s", tier)
	}
}

func TestReadOnlyFirstContactOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires shell stubs")
	}
	t.Setenv("NSELF_V15", "1")
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	for name, script := range map[string]string{
		"ssh-keygen":  "#!/bin/sh\nexit 1\n",
		"ssh-keyscan": "#!/bin/sh\nprintf 'once.example ssh-ed25519 AQID\\n'\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = writer
	t.Cleanup(func() { os.Stderr = old })
	for i := 0; i < 2; i++ {
		opts, err := HostKeyOptions(context.Background(), "qa", "web", TierLocalServers, "u@once.example", false)
		if err != nil || !strings.Contains(strings.Join(opts, " "), "accept-new") {
			t.Fatalf("read-only policy: %v %v", err, opts)
		}
	}
	_ = writer.Close()
	buf := make([]byte, 1024)
	n, _ := reader.Read(buf)
	if got := strings.Count(string(buf[:n]), "first contact once.example: host key SHA256:"); got != 1 {
		t.Fatalf("first-contact count=%d, output=%q", got, buf[:n])
	}
}

func TestDockerOKHostSpecPort(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires shell stub")
	}
	t.Setenv("NSELF_V15", "0")
	root := t.TempDir()
	bin := t.TempDir()
	argsFile := filepath.Join(bin, "ssh-args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + argsFile + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("NSELF_SSH_KEY_DOCKER_TEST", filepath.Join(root, "key"))
	server := Server{Name: "web", Host: "u@localhost:2222", SSHKeyRef: "NSELF_SSH_KEY_DOCKER_TEST"}
	ok, err := NewSSHProber(root, true).DockerOK(server)
	if err != nil || !ok {
		t.Fatalf("DockerOK: %v, %v", ok, err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "-p\n2222\n") || !strings.Contains(string(args), "--\nu@localhost\n") {
		t.Fatalf("non-default port argv: %s", args)
	}
	if _, err := NewSSHProber(root, true).DockerOK(Server{SSHKeyRef: "MISSING"}); err == nil {
		t.Fatal("missing key ref accepted")
	}
	t.Setenv("NSELF_SSH_KEY_DOCKER_TEST", filepath.Join(root, "key"))
	if _, err := NewSSHProber(root, true).DockerOK(Server{Host: "-bad", SSHKeyRef: "NSELF_SSH_KEY_DOCKER_TEST"}); err == nil {
		t.Fatal("bad host accepted")
	}
}

func TestHostSpecArgvSites(t *testing.T) {
	// Discover direct exec sites from git; these exceptions have separate owners.
	// db_remote_exec.go receives argv already built by db_remote.go.
	// internal/admin/connect.go is owned by P7-CANON-19 / DEPL-14.
	// db_remote_dryrun.go's raw probe destination is deferred to DEPL-14.
	root := filepath.Join("..", "..")
	grep := exec.Command("git", "grep", "-lE", `exec\.Command(Context)?\([^)]*"(ssh|scp|rsync)"`, "--", "cmd/", "internal/", "sdk/")
	grep.Dir = root
	out, err := grep.Output()
	if err != nil {
		t.Fatalf("discover exec sites: %v", err)
	}
	files := strings.Fields(string(out))
	if len(files) == 0 {
		t.Fatal("no ssh/scp/rsync exec sites discovered")
	}
	for _, file := range files {
		if file == "cmd/commands/db_remote_exec.go" || file == "internal/admin/connect.go" || strings.HasSuffix(file, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join("..", "..", file))
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if !strings.Contains(s, "ParseHostSpec") || !(strings.Contains(s, "SSHArgs()") || strings.Contains(s, "SSHOptions()")) {
			t.Errorf("%s does not parse hosts and build argv from HostSpec", file)
		}
	}
	// The dry-run probe is an indirect exec through db_remote_exec.go.
	b, err := os.ReadFile(filepath.Join(root, "cmd/commands/db_remote_dryrun.go"))
	if err != nil || !strings.Contains(string(b), "rt.SSHTarget") {
		t.Fatal("dry-run argv passthrough exception changed")
	}
}
