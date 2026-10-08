package controlplane

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// TestLoadAbsentFileUsesEnvVarSynthesis verifies that when control-plane.yaml
// does not exist, Load synthesizes the inventory from NSELF_DEPLOY_HOST_*
// environment variables.
func TestLoadAbsentFileUsesEnvVarSynthesis(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("NSELF_DEPLOY_HOST_STAGING", "ubuntu@staging.example.com")
	t.Setenv("NSELF_SSH_KEY_STAGING", "NSELF_SSH_KEY_STAGING")

	inv, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Must include synthesized "local" + "staging" environments.
	if _, ok := inv.Environments["local"]; !ok {
		t.Error("missing 'local' environment in synthesized inventory")
	}
	stg, ok := inv.Environments["staging"]
	if !ok {
		t.Fatal("missing synthesized 'staging' environment")
	}
	if len(stg.Servers) != 1 {
		t.Fatalf("staging servers: got %d, want 1", len(stg.Servers))
	}
	s := stg.Servers[0]
	if s.Host != "ubuntu@staging.example.com" {
		t.Errorf("server Host: got %q, want ubuntu@staging.example.com", s.Host)
	}
	if s.SSHKeyRef == "" {
		t.Error("server SSHKeyRef must not be empty")
	}
	// SSHKeyRef must be an env-var name, not a literal path.
	if len(s.SSHKeyRef) > 0 && s.SSHKeyRef[0] == '/' {
		t.Errorf("SSHKeyRef looks like a path: %q", s.SSHKeyRef)
	}
	if s.Role != RoleApp {
		t.Errorf("server Role: got %q, want app", s.Role)
	}
	if !s.Primary {
		t.Error("synthesized server should be Primary")
	}
	if s.RemotePath == "" {
		t.Error("RemotePath must be set (default /opt/nself)")
	}
}

// TestLoadNoEnvVarsSynthesizesLocalOnly verifies that with no env vars, Load
// synthesizes a minimal local-only inventory.
func TestLoadNoEnvVarsSynthesizesLocalOnly(t *testing.T) {
	dir := t.TempDir()
	// Ensure no NSELF_DEPLOY_HOST_* vars are set.
	for _, e := range []string{"NSELF_DEPLOY_HOST_STAGING", "NSELF_DEPLOY_HOST_PROD"} {
		t.Setenv(e, "")
	}

	inv, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if inv.SchemaVersion != currentSchemaVersion {
		t.Errorf("SchemaVersion: got %d, want %d", inv.SchemaVersion, currentSchemaVersion)
	}
	if _, ok := inv.Environments["local"]; !ok {
		t.Error("missing 'local' environment")
	}
}

// TestLoadPresentFile verifies that a valid control-plane.yaml is loaded
// correctly, ignoring env vars that might synthesize extras.
func TestLoadPresentFile(t *testing.T) {
	dir := t.TempDir()
	nself := filepath.Join(dir, ".nself")
	if err := os.MkdirAll(nself, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	content := `schema_version: 1
project: myproject
environments:
  local:
    name: local
    kind: local
    servers:
      - name: local-app
        role: app
        primary: true
`
	p := filepath.Join(nself, "control-plane.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	inv, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if inv.Project != "myproject" {
		t.Errorf("Project: got %q, want myproject", inv.Project)
	}
	if _, ok := inv.Environments["local"]; !ok {
		t.Error("missing 'local' environment from file")
	}
}

// TestWriteCreatesFileWith0600Perms verifies that Write produces a file
// readable only by the owner (mode 0600).
func TestWriteCreatesFileWith0600Perms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped on Windows: chmod 0600 is not enforced")
	}
	dir := t.TempDir()
	inv := &Inventory{
		SchemaVersion: currentSchemaVersion,
		Project:       "testperm",
		Environments: map[string]Environment{
			"local": {Name: "local", Kind: "local", Servers: []Server{
				{Name: "local-app", Role: RoleApp, Primary: true},
			}},
		},
	}

	if err := Write(dir, inv); err != nil {
		t.Fatalf("Write: %v", err)
	}

	path := filepath.Join(dir, inventoryFileName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0o600 {
		t.Errorf("file mode: got %04o, want 0600", mode)
	}
}

// TestWriteRoundTrip verifies that writing and then loading an inventory
// produces an equivalent structure.
func TestWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	inv := &Inventory{
		SchemaVersion: currentSchemaVersion,
		Project:       "roundtrip",
		Environments: map[string]Environment{
			"local": {
				Name: "local",
				Kind: "local",
				Servers: []Server{
					{Name: "local-app", Role: RoleApp, Primary: true},
				},
			},
		},
	}

	if err := Write(dir, inv); err != nil {
		t.Fatalf("Write: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Project != inv.Project {
		t.Errorf("Project: got %q, want %q", loaded.Project, inv.Project)
	}
	if loaded.SchemaVersion != inv.SchemaVersion {
		t.Errorf("SchemaVersion: got %d, want %d", loaded.SchemaVersion, inv.SchemaVersion)
	}
}

// TestMigrateIdempotent verifies that calling Migrate twice on a current-version
// inventory does not change it.
func TestMigrateIdempotent(t *testing.T) {
	inv := &Inventory{
		SchemaVersion: currentSchemaVersion,
		Project:       "idempotent",
		Environments:  map[string]Environment{},
	}

	if err := Migrate(inv); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := Migrate(inv); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if inv.SchemaVersion != currentSchemaVersion {
		t.Errorf("SchemaVersion changed after double migrate: got %d", inv.SchemaVersion)
	}
}

// TestMigrateVersion0AddsLocal verifies that a version-0 (unversioned) inventory
// gets the local environment synthesized.
func TestMigrateVersion0AddsLocal(t *testing.T) {
	inv := &Inventory{
		SchemaVersion: 0,
		Project:       "legacy",
	}

	if err := Migrate(inv); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if inv.SchemaVersion != currentSchemaVersion {
		t.Errorf("SchemaVersion: got %d, want %d", inv.SchemaVersion, currentSchemaVersion)
	}
	if _, ok := inv.Environments["local"]; !ok {
		t.Error("expected 'local' environment after version-0 migration")
	}
}

// TestMigrateUnsupportedVersionErrors verifies that an unsupported schema
// version returns an error.
func TestMigrateUnsupportedVersionErrors(t *testing.T) {
	inv := &Inventory{
		SchemaVersion: 999,
	}
	if err := Migrate(inv); err == nil {
		t.Error("expected error for unsupported schema_version 999, got nil")
	}
}

// TestValidateServerName verifies that ValidateServerName accepts safe names
// and rejects names containing shell metacharacters or other dangerous chars.
// These names flow into SSH argv (lb.go) — any character outside [a-zA-Z0-9_-]
// is a potential injection vector.
func TestValidateServerName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid names.
		{name: "simple", input: "app01", wantErr: false},
		{name: "hyphen", input: "lb-prod", wantErr: false},
		{name: "underscore", input: "stg_app", wantErr: false},
		{name: "mixed", input: "App-Server_01", wantErr: false},
		{name: "single-char", input: "a", wantErr: false},

		// Invalid names — shell metacharacters and injection payloads.
		{name: "empty", input: "", wantErr: true},
		{name: "semicolon", input: "x; touch /tmp/pwned", wantErr: true},
		{name: "backtick", input: "x`id`", wantErr: true},
		{name: "dollar", input: "x$(id)", wantErr: true},
		{name: "pipe", input: "x|cat /etc/passwd", wantErr: true},
		{name: "ampersand", input: "x&", wantErr: true},
		{name: "space", input: "my app", wantErr: true},
		{name: "newline", input: "app\n", wantErr: true},
		{name: "slash", input: "app/subdir", wantErr: true},
		{name: "dot-dot", input: "../escape", wantErr: true},
		{name: "single-quote", input: "app'name", wantErr: true},
		{name: "double-quote", input: `app"name`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateServerName(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateServerName(%q): expected error, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateServerName(%q): unexpected error: %v", tc.input, err)
			}
		})
	}
}

// TestLoadRejectsInvalidServerNames verifies that Load returns an error when
// the inventory YAML contains a server name with shell metacharacters. This
// exercises the defence-in-depth validation gate in Load().
func TestLoadRejectsInvalidServerNames(t *testing.T) {
	dir := t.TempDir()
	nself := filepath.Join(dir, ".nself")
	if err := os.MkdirAll(nself, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// A server whose name contains a shell injection payload.
	content := `schema_version: 1
project: evilproject
environments:
  prod:
    name: prod
    kind: remote
    servers:
      - name: "x; touch /tmp/pwned"
        role: app
        host: ubuntu@prod.example.com
        ssh_key_ref: NSELF_SSH_KEY_PROD
        remote_path: /opt/nself
        primary: true
`
	p := filepath.Join(nself, "control-plane.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Error("Load: expected error for invalid server name, got nil")
	}
}

// TestLoadRejectsInvalidRemotePath (T31) verifies that a hand-edited or
// imported .nself/control-plane.yaml carrying a RemotePath with shell
// metacharacters is rejected by Load, not just the `env target add` CLI
// flag path. RemotePath is later interpolated into a shell command string
// executed on the remote host over SSH (internal/deploy/ssh.go).
func TestLoadRejectsInvalidRemotePath(t *testing.T) {
	payloads := []string{
		"/opt/x; id",
		"/opt/x$(id)",
		"/opt/x`id`",
	}

	for _, payload := range payloads {
		dir := t.TempDir()
		nself := filepath.Join(dir, ".nself")
		if err := os.MkdirAll(nself, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		content := "schema_version: 1\n" +
			"project: evilproject\n" +
			"environments:\n" +
			"  prod:\n" +
			"    name: prod\n" +
			"    kind: remote\n" +
			"    servers:\n" +
			"      - name: web1\n" +
			"        role: app\n" +
			"        host: ubuntu@prod.example.com\n" +
			"        ssh_key_ref: NSELF_SSH_KEY_PROD\n" +
			"        remote_path: \"" + payload + "\"\n" +
			"        primary: true\n"

		p := filepath.Join(nself, "control-plane.yaml")
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}

		if _, err := Load(dir); err == nil {
			t.Errorf("Load: expected error for RemotePath %q, got nil", payload)
		}
	}
}

// TestLoadSynthesizeRejectsInvalidRemotePathEnvVar (T31) verifies the
// env-var synthesis path (Load when .nself/control-plane.yaml is absent)
// applies the same RemotePath guard, since NSELF_REMOTE_PATH_<TARGET> is
// just as much external input as the YAML file.
func TestLoadSynthesizeRejectsInvalidRemotePathEnvVar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NSELF_DEPLOY_HOST_STAGING", "ubuntu@staging.example.com")
	t.Setenv("NSELF_REMOTE_PATH_STAGING", "/opt/x; id")

	if _, err := Load(dir); err == nil {
		t.Error("Load: expected error for malicious NSELF_REMOTE_PATH_STAGING, got nil")
	}
}

// TestInventoryRefusesEnvCaseCollision: two env keys that differ only by case
// are refused by Load (from yaml) with E483 naming both keys; distinct names
// load.
func TestInventoryRefusesEnvCaseCollision(t *testing.T) {
	dir := t.TempDir()
	mk := func(keys ...string) *Inventory {
		inv := &Inventory{SchemaVersion: 1, Project: "t", Environments: map[string]Environment{}}
		for _, k := range keys {
			inv.Environments[k] = Environment{Name: k, Kind: "remote", Servers: []Server{{Name: "s-" + strings.ToLower(k), Role: RoleApp, Host: "u@h.example.test", RemotePath: "/opt/nself", Primary: true}}}
		}
		inv.Environments["local"] = Environment{Name: "local", Kind: "local", Servers: []Server{{Name: "local-app", Role: RoleApp, Primary: true}}}
		return inv
	}
	if err := Write(dir, mk("qa", "QA", "prod")); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E483" {
		t.Fatalf("Load = %v, want E483", err)
	}
	if !strings.Contains(err.Error(), `"QA"`) || !strings.Contains(err.Error(), `"qa"`) {
		t.Errorf("error must name both keys: %v", err)
	}
	if err := Write(dir, mk("qa", "prod")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err != nil {
		t.Errorf("distinct env names must load: %v", err)
	}
}

func TestInventoryValidate(t *testing.T) {
	base := func() *Inventory {
		return &Inventory{SchemaVersion: 2, Project: "test", Environments: map[string]Environment{
			"local": {Name: "local", Kind: "local", Tier: TierLocal, Servers: []Server{{Name: "local-app", Role: RoleApp, Primary: true}}},
			"prod":  {Name: "prod", Kind: "remote", Tier: TierProd, Servers: []Server{{Name: "prod-app", Role: RoleApp, Host: "u@host.example", Primary: true}}},
		}}
	}
	for _, tc := range []struct {
		name, field string
		mutate      func(*Inventory)
	}{
		{"duplicate server", "servers.name", func(i *Inventory) {
			e := i.Environments["prod"]
			e.Servers[0].Name = "local-app"
			i.Environments["prod"] = e
		}},
		{"prod primary", "servers", func(i *Inventory) {
			e := i.Environments["prod"]
			e.Servers[0].Primary = false
			i.Environments["prod"] = e
		}},
		{"two local", "local", func(i *Inventory) {
			i.Environments["second"] = Environment{Name: "second", Kind: "local", Tier: TierLocal}
		}},
		{"invalid env", "name", func(i *Inventory) {
			i.Environments["Bad Name"] = Environment{Name: "Bad Name", Kind: "remote", Tier: TierLocalServers}
		}},
		{"host injection", ".host", func(i *Inventory) {
			e := i.Environments["prod"]
			e.Servers[0].Host = "-oProxyCommand=touch /tmp/pwn"
			i.Environments["prod"] = e
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "exec-ran")
			if tc.name == "host injection" {
				bin := t.TempDir()
				for _, tool := range []string{"ssh", "scp", "rsync"} {
					if err := os.WriteFile(filepath.Join(bin, tool), []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o700); err != nil {
						t.Fatal(err)
					}
				}
				t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			}
			i := base()
			tc.mutate(i)
			dir := t.TempDir()
			if err := Write(dir, i); err != nil {
				t.Fatal(err)
			}
			_, err := Load(dir)
			if err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("Load = %v, want field %s", err, tc.field)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("invalid host started a process")
			}
		})
	}
}

func TestLegacyHost(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NSELF_V15", "0")
	t.Setenv("NSELF_DEPLOY_HOST_QA", "u@host.example:/opt/nself")
	legacy, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_DEPLOY_HOST_QA", "u@host.example")
	t.Setenv("NSELF_REMOTE_PATH_QA", "/opt/nself")
	canonical, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, b := legacy.Environments["qa"].Servers[0], canonical.Environments["qa"].Servers[0]
	if a.Host != b.Host || a.RemotePath != b.RemotePath {
		t.Fatalf("legacy %+v != canonical %+v", a, b)
	}
	legacyWarningOnce = sync.Once{}
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_DEPLOY_HOST_QA", "u@host.example:/opt/nself")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()
	for i := 0; i < 2; i++ {
		if _, err := Load(dir); err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Close()
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	if got := strings.Count(string(buf[:n]), "v1.6.0"); got != 1 {
		t.Fatalf("warning count = %d: %q", got, buf[:n])
	}
}

func TestInventoryV2ByteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	inv := &Inventory{SchemaVersion: 2, Project: "test", Environments: map[string]Environment{
		"local": {Name: "local", Kind: "local", Tier: TierLocal, Servers: []Server{{Name: "local-app", Role: RoleApp}}},
		"qa":    {Name: "qa", Kind: "remote", Tier: TierLocalServers, Servers: []Server{{Name: "qa-app", Role: RoleApp, Host: "u@host.example:2222", RemotePath: "/opt/nself", Arch: "amd64", Primary: true}}},
	}}
	if err := Write(dir, inv); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".nself", "control-plane.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, loaded); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("v2 round trip changed bytes\nbefore:\n%s\nafter:\n%s", before, after)
	}
}
