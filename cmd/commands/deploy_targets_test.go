package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/spf13/cobra"
)

func TestDeployTargetsJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX command stubs")
	}
	dir := t.TempDir()
	violation := filepath.Join(dir, "connected")
	ssh := "#!/bin/sh\nif [ \"$1\" != -G ]; then touch '" + violation + "'; exit 1; fi\nprintf 'hostname localhost\\nport 2222\\nuser deploy\\n'\n"
	keygen := "#!/bin/sh\nif [ \"$1\" != -F ]; then touch '" + violation + "'; exit 1; fi\nprintf 'localhost ssh-ed25519 AQID\\n'\n"
	for name, script := range map[string]string{"ssh": ssh, "ssh-keygen": keygen} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	inv := &controlplane.Inventory{Environments: map[string]controlplane.Environment{
		"qa": {Name: "qa", Kind: "remote", Tier: controlplane.TierLocalServers, Servers: []controlplane.Server{{Name: "z", Role: controlplane.RoleApp, Host: "deploy@localhost:2222"}, {Name: "a", Role: controlplane.RoleLB, Host: "deploy@localhost:2222"}}},
	}}
	data, err := collectDeployTargets(context.Background(), inv, "inventory")
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Targets) != 2 || data.Targets[0].Server != "a" || data.Targets[0].SSHHostname != "localhost" || data.Targets[0].Port != 2222 {
		t.Fatalf("rows: %+v", data.Targets)
	}
	if len(data.Targets[0].HostKeyFingerprints) != 1 || !strings.HasPrefix(data.Targets[0].HostKeyFingerprints[0], "SHA256:") {
		t.Fatalf("fingerprints: %+v", data.Targets[0])
	}
	if _, err := os.Stat(violation); !os.IsNotExist(err) {
		t.Fatal("a connection command was attempted")
	}
	var out bytes.Buffer
	if err := output.EmitData(output.Writer{Out: &out, Err: &bytes.Buffer{}}, "deploy targets", data); err != nil {
		t.Fatal(err)
	}
	if err := contractValidate(contractResolve(t, "deploy-targets.v1.schema.json"), mustData(t, out.Bytes())); err != nil {
		t.Fatalf("schema: %v", err)
	}
	for _, v15 := range []bool{false, true} {
		reg, err := BuildRegistry(v15)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, command := range reg.Commands {
			if command.Path == "nself deploy targets" {
				found = true
				if command.SideEffect != "read" || command.Output != "document" || command.JSON != "envelope" {
					t.Fatalf("registry v15=%v: %+v", v15, command)
				}
			}
		}
		if !found {
			t.Fatal("deploy targets missing from registry")
		}
	}
}

func TestDeployTargetsIntegration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX command stubs")
	}
	root, cleanup := newTestRoot(t)
	defer cleanup()
	writeInventory(t, root, &controlplane.Inventory{SchemaVersion: 1, Project: "test", Environments: map[string]controlplane.Environment{
		"local": {Name: "local", Kind: "local", Servers: []controlplane.Server{{Name: "local-app", Role: controlplane.RoleApp}}},
		"qa":    {Name: "qa", Kind: "remote", Servers: []controlplane.Server{{Name: "web", Role: controlplane.RoleApp, Host: "u@localhost:2222", Primary: true}}},
	}})
	bin := t.TempDir()
	violation := filepath.Join(bin, "connected")
	ssh := "#!/bin/sh\nif [ \"$1\" != -G ]; then touch '" + violation + "'; exit 1; fi\nprintf 'hostname localhost\\nport 2222\\nuser u\\n'\n"
	keygen := "#!/bin/sh\nif [ \"$1\" != -F ]; then touch '" + violation + "'; exit 1; fi\nexit 1\n"
	for name, script := range map[string]string{"ssh": ssh, "ssh-keygen": keygen} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("json", false, "")
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runDeployTargets(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "qa\tweb\tlocal-servers\tlocalhost") {
		t.Fatalf("targets output: %q", out.String())
	}
	if _, err := os.Stat(violation); !os.IsNotExist(err) {
		t.Fatal("deploy targets opened a connection")
	}
}

func mustData(t *testing.T, doc []byte) []byte {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(doc, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data) == 0 {
		t.Fatal("no data field")
	}
	return envelope.Data
}

func TestEnvTargetTrustHostKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX command stubs")
	}
	root, cleanup := newTestRoot(t)
	defer cleanup()
	t.Setenv("HOME", t.TempDir())
	writeInventory(t, root, &controlplane.Inventory{SchemaVersion: 1, Project: "test", Environments: map[string]controlplane.Environment{
		"local": {Name: "local", Kind: "local", Servers: []controlplane.Server{{Name: "local-app", Role: controlplane.RoleApp}}},
		"qa":    {Name: "qa", Kind: "remote", Servers: []controlplane.Server{{Name: "web", Role: controlplane.RoleApp, Host: "u@host.example", Primary: true}}},
	}})
	before, err := os.ReadFile(filepath.Join(root, ".nself", "control-plane.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	key := "host.example ssh-ed25519 AQID"
	if err := os.WriteFile(filepath.Join(bin, "ssh-keyscan"), []byte("#!/bin/sh\nprintf '%s\\n' '"+key+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().String("host", "", "")
	cmd.Flags().String("role", "app", "")
	cmd.Flags().String("key-ref", "", "")
	cmd.Flags().String("remote-path", "/opt/nself", "")
	cmd.Flags().Bool("primary", false, "")
	cmd.Flags().StringSlice("upstreams", nil, "")
	cmd.Flags().String("tier", "", "")
	cmd.Flags().String("arch", "", "")
	cmd.Flags().String("trust-host-key", "SHA256:wrong", "")
	if err := runEnvTargetAdd(cmd, []string{"qa", "web"}); err == nil {
		t.Fatal("wrong fingerprint accepted")
	}
	after, _ := os.ReadFile(filepath.Join(root, ".nself", "control-plane.yaml"))
	if !bytes.Equal(before, after) {
		t.Fatal("failed trust mutated inventory")
	}
	fp, err := remote.Fingerprint(key)
	if err != nil {
		t.Fatal(err)
	}
	_ = cmd.Flags().Set("trust-host-key", fp)
	if err := runEnvTargetAdd(cmd, []string{"qa", "web"}); err != nil {
		t.Fatal(err)
	}
	after, _ = os.ReadFile(filepath.Join(root, ".nself", "control-plane.yaml"))
	if !bytes.Equal(before, after) {
		t.Fatal("trust mutated inventory")
	}
	if _, err := os.Stat(controlplane.DeployKnownHostsPath()); err != nil {
		t.Fatal("pin missing:", err)
	}
}

func TestE487HintEnrollThenDeploy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX command stubs")
	}
	root, cleanup := newTestRoot(t)
	defer cleanup()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NSELF_V15", "1")
	host := "u@host.example"
	writeInventory(t, root, &controlplane.Inventory{SchemaVersion: 2, Environments: map[string]controlplane.Environment{
		"qa": {Name: "qa", Kind: "remote", Tier: controlplane.TierLocalServers, Servers: []controlplane.Server{{Name: "web", Role: controlplane.RoleApp, Host: host, RemotePath: "/opt/nself", Primary: true}}},
	}})
	bin := t.TempDir()
	marker := filepath.Join(bin, "deployed")
	for name, body := range map[string]string{
		"ssh-keyscan": "#!/bin/sh\nprintf 'host.example ssh-ed25519 AQID\\n'\n",
		"ssh-keygen":  "#!/bin/sh\nif [ -f \"$4\" ]; then /bin/cat \"$4\"; fi\n",
		"ssh":         "#!/bin/sh\nexit 0\n",
		"rsync":       "#!/bin/sh\ntouch '" + marker + "'\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	_, err := controlplane.HostKeyOptions(context.Background(), "qa", "primary", controlplane.TierLocalServers, host, true)
	if err == nil || !strings.Contains(err.Error(), "E487") {
		t.Fatalf("expected E487 hint: %v", err)
	}
	_, hint, ok := strings.Cut(err.Error(), "nself env target add ")
	if !ok {
		t.Fatalf("missing enrolment command: %v", err)
	}
	argv := strings.Fields(strings.SplitN(hint, "\n", 2)[0])
	if len(argv) != 4 || argv[0] != "qa" || argv[1] != "web" || argv[2] != "--trust-host-key" {
		t.Fatalf("unusable hint: %q", hint)
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().String("host", "", "")
	cmd.Flags().String("role", "app", "")
	cmd.Flags().String("key-ref", "", "")
	cmd.Flags().String("remote-path", "/opt/nself", "")
	cmd.Flags().Bool("primary", false, "")
	cmd.Flags().StringSlice("upstreams", nil, "")
	cmd.Flags().String("tier", "", "")
	cmd.Flags().String("arch", "", "")
	cmd.Flags().String("trust-host-key", "", "")
	if err := cmd.Flags().Set("trust-host-key", argv[3]); err != nil {
		t.Fatal(err)
	}
	if err := runEnvTargetAdd(cmd, argv[:2]); err != nil {
		t.Fatalf("printed command failed: %v", err)
	}
	compose := filepath.Join(root, "compose.yml")
	if err := os.WriteFile(compose, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := deploy.DeployViaSsh(context.Background(), deploy.SSHConfig{Host: host, RemotePath: "/opt/nself", KeyPath: "/tmp/key", EnvName: "qa", ServerName: "web"}, compose); err != nil {
		t.Fatalf("deploy after enrolment: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("fake transport did not deploy: %v", err)
	}
}
