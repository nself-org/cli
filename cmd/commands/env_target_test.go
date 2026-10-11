package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/nself-org/cli/internal/controlplane"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// newTestRoot creates a temp directory that looks like a project root and
// changes the process working directory to it. The returned cleanup function
// restores the original cwd.
func newTestRoot(t *testing.T) (string, func()) {
	t.Helper()
	root := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir to temp root: %v", err)
	}
	return root, func() { _ = os.Chdir(orig) }
}

// writeInventory writes a control-plane.yaml into root for test setup.
func writeInventory(t *testing.T, root string, inv *controlplane.Inventory) {
	t.Helper()
	if _, ok := inv.Environments["local"]; !ok {
		inv.Environments["local"] = controlplane.Environment{Name: "local", Kind: "local", Servers: []controlplane.Server{{Name: "local-app", Role: controlplane.RoleApp, Primary: true}}}
	}
	if err := controlplane.Write(root, inv); err != nil {
		t.Fatalf("writeInventory: %v", err)
	}
}

// readInventory loads inventory from root.
func readInventory(t *testing.T, root string) *controlplane.Inventory {
	t.Helper()
	inv, err := controlplane.Load(root)
	if err != nil {
		t.Fatalf("readInventory: %v", err)
	}
	return inv
}

// freshCmd returns a new command with src's RunE and a copy of every flag src
// declares itself (name, type, default), then applies set. Tests run against
// the copy, so they never reset or redeclare flags on the shared command tree
// that the registry golden and the shipped binary read (D-0237). Flags merged
// in from a parent's persistent set (the same *pflag.Flag) are left out; a
// local flag that shadows one, like `list --json`, is copied.
func freshCmd(t *testing.T, src *cobra.Command, set map[string]string) *cobra.Command {
	t.Helper()
	inherited := func(f *pflag.Flag) bool {
		for p := src.Parent(); p != nil; p = p.Parent() {
			if p.PersistentFlags().Lookup(f.Name) == f {
				return true
			}
		}
		return false
	}
	c := &cobra.Command{Use: src.Use, RunE: src.RunE}
	src.Flags().VisitAll(func(f *pflag.Flag) {
		if inherited(f) {
			return
		}
		switch f.Value.Type() {
		case "string":
			c.Flags().String(f.Name, f.DefValue, f.Usage)
		case "bool":
			def, err := strconv.ParseBool(f.DefValue)
			if err != nil {
				t.Fatalf("flag --%s default %q: %v", f.Name, f.DefValue, err)
			}
			c.Flags().Bool(f.Name, def, f.Usage)
		case "stringSlice":
			var def []string
			if v := strings.Trim(f.DefValue, "[]"); v != "" {
				def = strings.Split(v, ",")
			}
			c.Flags().StringSlice(f.Name, def, f.Usage)
		default:
			t.Fatalf("freshCmd: flag --%s has unsupported type %s", f.Name, f.Value.Type())
		}
	})
	for name, v := range set {
		if err := c.Flags().Set(name, v); err != nil {
			t.Fatalf("set --%s=%q: %v", name, v, err)
		}
	}
	return c
}

// TestFreshCmdLeavesSharedTreeIntact guards D-0237: a copy carries the real
// defaults, and setting flags on it leaves the shared command's flag set,
// values and Changed state as init() registered them.
func TestFreshCmdLeavesSharedTreeIntact(t *testing.T) {
	snapshot := func() string {
		var b strings.Builder
		envTargetAddCmd.Flags().VisitAll(func(f *pflag.Flag) {
			b.WriteString(f.Name + "=" + f.Value.Type() + ":" + f.Value.String() + ":" + strconv.FormatBool(f.Changed) + ";")
		})
		return b.String()
	}
	for _, name := range []string{"host", "role", "tier", "arch", "trust-host-key", "upstreams"} {
		if envTargetAddCmd.Flags().Lookup(name) == nil {
			t.Fatalf("shared `env target add` lost --%s before this test ran", name)
		}
	}
	before := snapshot()
	c := freshCmd(t, envTargetAddCmd, map[string]string{"host": "ubuntu@copy.example.com", "primary": "true"})
	if role, _ := c.Flags().GetString("role"); role != "app" {
		t.Errorf("copy --role default = %q, want the real default app", role)
	}
	if rp, _ := c.Flags().GetString("remote-path"); rp != "/opt/nself" {
		t.Errorf("copy --remote-path default = %q, want /opt/nself", rp)
	}
	if after := snapshot(); after != before {
		t.Fatalf("shared command flags changed:\nbefore %s\nafter  %s", before, after)
	}
}

// ── T01: list ────────────────────────────────────────────────────────────────

func TestEnvTargetList_EmptyInventory(t *testing.T) {
	root, cleanup := newTestRoot(t)
	defer cleanup()

	// Write a minimal inventory with only a local env.
	writeInventory(t, root, &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]controlplane.Environment{
			"local": {
				Name: "local",
				Kind: "local",
				Servers: []controlplane.Server{
					{Name: "local-app", Role: controlplane.RoleApp, Primary: true},
				},
			},
		},
	})

	cmd := freshCmd(t, envTargetListCmd, nil)

	err := runEnvTargetList(cmd, nil)
	if err != nil {
		t.Fatalf("runEnvTargetList: %v", err)
	}
}

func TestEnvTargetList_JSON(t *testing.T) {
	root, cleanup := newTestRoot(t)
	defer cleanup()

	writeInventory(t, root, &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]controlplane.Environment{
			"staging": {
				Name: "staging",
				Kind: "remote",
				Servers: []controlplane.Server{
					{
						Name:      "staging-app",
						Role:      controlplane.RoleApp,
						Host:      "ubuntu@staging.example.com",
						SSHKeyRef: "NSELF_SSH_KEY_STAGING",
						Primary:   true,
					},
				},
			},
		},
	})

	// Capture stdout via a pipe.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := freshCmd(t, envTargetListCmd, map[string]string{"json": "true"})

	err := runEnvTargetList(cmd, []string{"staging"})
	_ = w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("runEnvTargetList: %v", err)
	}

	var buf strings.Builder
	tmp := make([]byte, 4096)
	for {
		n, _ := r.Read(tmp)
		if n == 0 {
			break
		}
		buf.Write(tmp[:n])
	}

	var rows []envTargetListRow
	if err := json.Unmarshal([]byte(buf.String()), &rows); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %s", err, buf.String())
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Env != "staging" {
		t.Errorf("env: got %q, want %q", rows[0].Env, "staging")
	}
	if rows[0].Server != "staging-app" {
		t.Errorf("server: got %q, want %q", rows[0].Server, "staging-app")
	}
	// SSHKeyRef must be the env-var NAME, never the key value.
	if rows[0].SSHKeyRef != "NSELF_SSH_KEY_STAGING" {
		t.Errorf("ssh_key_ref: got %q, want %q", rows[0].SSHKeyRef, "NSELF_SSH_KEY_STAGING")
	}
}

// ── T01: add / remove CRUD round-trip ────────────────────────────────────────

func TestEnvTargetAdd_RoundTrip(t *testing.T) {
	root, cleanup := newTestRoot(t)
	defer cleanup()

	// Seed with empty inventory (local only).
	writeInventory(t, root, &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]controlplane.Environment{
			"local": {
				Name:    "local",
				Kind:    "local",
				Servers: []controlplane.Server{{Name: "local-app", Role: controlplane.RoleApp, Primary: true}},
			},
		},
	})

	// Add staging server.
	cmd := freshCmd(t, envTargetAddCmd, map[string]string{
		"host": "ubuntu@staging.example.com", "role": "app", "key-ref": "NSELF_SSH_KEY_STAGING", "primary": "true",
	})

	if err := runEnvTargetAdd(cmd, []string{"staging", "web1"}); err != nil {
		t.Fatalf("add: %v", err)
	}

	inv := readInventory(t, root)
	stagingEnv, ok := inv.Environments["staging"]
	if !ok {
		t.Fatal("staging environment not found after add")
	}
	if len(stagingEnv.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(stagingEnv.Servers))
	}
	srv := stagingEnv.Servers[0]
	if srv.Name != "web1" {
		t.Errorf("server name: got %q, want %q", srv.Name, "web1")
	}
	if srv.Host != "ubuntu@staging.example.com" {
		t.Errorf("host: got %q", srv.Host)
	}
	if srv.SSHKeyRef != "NSELF_SSH_KEY_STAGING" {
		t.Errorf("key ref: got %q", srv.SSHKeyRef)
	}
	if !srv.Primary {
		t.Error("expected Primary=true")
	}

	// Remove staging server.
	if err := runEnvTargetRemove(freshCmd(t, envTargetRemoveCmd, nil), []string{"staging", "web1"}); err != nil {
		t.Fatalf("remove: %v", err)
	}

	inv = readInventory(t, root)
	stagingEnv = inv.Environments["staging"]
	if len(stagingEnv.Servers) != 0 {
		t.Fatalf("expected 0 servers after remove, got %d", len(stagingEnv.Servers))
	}
}

func TestEnvTargetAdd_DuplicateServerRejected(t *testing.T) {
	root, cleanup := newTestRoot(t)
	defer cleanup()

	writeInventory(t, root, &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]controlplane.Environment{
			"staging": {
				Name: "staging",
				Kind: "remote",
				Servers: []controlplane.Server{
					{Name: "web1", Role: controlplane.RoleApp, Host: "ubuntu@staging.example.com", SSHKeyRef: "NSELF_SSH_KEY_STAGING"},
				},
			},
		},
	})

	cmd := freshCmd(t, envTargetAddCmd, map[string]string{"host": "ubuntu@staging.example.com", "key-ref": "NSELF_SSH_KEY_STAGING"})

	err := runEnvTargetAdd(cmd, []string{"staging", "web1"})
	if err == nil {
		t.Fatal("expected error for duplicate server name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEnvTargetAdd_InlineKeyRefRejected(t *testing.T) {
	root, cleanup := newTestRoot(t)
	defer cleanup()

	writeInventory(t, root, &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments:  map[string]controlplane.Environment{},
	})

	// Passing a file path instead of an env-var name must be rejected.
	cmd := freshCmd(t, envTargetAddCmd, map[string]string{"host": "ubuntu@staging.example.com", "key-ref": "/home/user/.ssh/id_ed25519"})

	err := runEnvTargetAdd(cmd, []string{"staging", "web1"})
	if err == nil {
		t.Fatal("expected error for inline key path in --key-ref")
	}
	if !strings.Contains(err.Error(), "environment variable NAME") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestEnvTargetAdd_RemotePathInjectionRejected (T31) verifies that
// `env target add --remote-path` rejects values containing shell
// metacharacters before they are ever persisted to
// .nself/control-plane.yaml, since RemotePath is later interpolated into a
// shell command string run on the remote host over SSH.
func TestEnvTargetAdd_RemotePathInjectionRejected(t *testing.T) {
	payloads := []string{
		"/opt/x; id",
		"/opt/x$(id)",
		"/opt/x`id`",
		"/opt/x | id",
		"/opt/x && id",
	}

	for _, payload := range payloads {
		payload := payload
		t.Run(payload, func(t *testing.T) {
			root, cleanup := newTestRoot(t)
			defer cleanup()

			writeInventory(t, root, &controlplane.Inventory{
				SchemaVersion: 1,
				Project:       "test",
				Environments:  map[string]controlplane.Environment{},
			})

			cmd := freshCmd(t, envTargetAddCmd, map[string]string{
				"host": "ubuntu@staging.example.com", "key-ref": "NSELF_SSH_KEY_STAGING", "remote-path": payload,
			})

			err := runEnvTargetAdd(cmd, []string{"staging", "web1"})
			if err == nil {
				t.Fatalf("expected error for malicious --remote-path %q, got nil", payload)
			}
			if !strings.Contains(err.Error(), "unsafe characters") {
				t.Errorf("unexpected error for %q: %v", payload, err)
			}

			// The malicious value must never reach the inventory file.
			inv := readInventory(t, root)
			if stg, ok := inv.Environments["staging"]; ok {
				for _, s := range stg.Servers {
					if s.RemotePath == payload {
						t.Errorf("malicious RemotePath %q was written to control-plane.yaml", payload)
					}
				}
			}
		})
	}
}

// TestEnvTargetAdd_RemotePathNormalPathAccepted (T31) proves the new guard
// causes no regression for legitimate remote paths.
func TestEnvTargetAdd_RemotePathNormalPathAccepted(t *testing.T) {
	root, cleanup := newTestRoot(t)
	defer cleanup()

	writeInventory(t, root, &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments:  map[string]controlplane.Environment{},
	})

	cmd := freshCmd(t, envTargetAddCmd, map[string]string{"host": "ubuntu@staging.example.com", "key-ref": "NSELF_SSH_KEY_STAGING"})

	if err := runEnvTargetAdd(cmd, []string{"staging", "web1"}); err != nil {
		t.Fatalf("add with normal --remote-path: %v", err)
	}

	inv := readInventory(t, root)
	stg, ok := inv.Environments["staging"]
	if !ok || len(stg.Servers) != 1 {
		t.Fatal("expected staging environment with 1 server")
	}
	if stg.Servers[0].RemotePath != "/opt/nself" {
		t.Errorf("RemotePath: got %q, want /opt/nself", stg.Servers[0].RemotePath)
	}
}

func TestEnvTargetRemove_NotFound(t *testing.T) {
	root, cleanup := newTestRoot(t)
	defer cleanup()

	writeInventory(t, root, &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]controlplane.Environment{
			"staging": {Name: "staging", Kind: "remote", Servers: nil},
		},
	})

	err := runEnvTargetRemove(freshCmd(t, envTargetRemoveCmd, nil), []string{"staging", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for non-existent server")
	}
}

// ── T02: probe ───────────────────────────────────────────────────────────────

func TestEnvTargetProbe_LocalEnv(t *testing.T) {
	// Local env always resolves to manage without any SSH probe.
	root, cleanup := newTestRoot(t)
	defer cleanup()

	writeInventory(t, root, &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]controlplane.Environment{
			"local": {
				Name:    "local",
				Kind:    "local",
				Servers: []controlplane.Server{{Name: "local-app", Role: controlplane.RoleApp, Primary: true}},
			},
		},
	})

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := freshCmd(t, envTargetProbeCmd, map[string]string{"json": "true"})

	err := runEnvTargetProbe(cmd, []string{"local"})
	_ = w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("probe: %v", err)
	}

	var buf strings.Builder
	tmp := make([]byte, 4096)
	for {
		n, _ := r.Read(tmp)
		if n == 0 {
			break
		}
		buf.Write(tmp[:n])
	}

	var rows []probeRow
	if err := json.Unmarshal([]byte(buf.String()), &rows); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %s", err, buf.String())
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Capability != string(controlplane.CapManage) {
		t.Errorf("capability: got %q, want %q", rows[0].Capability, controlplane.CapManage)
	}
}

func TestEnvTargetProbe_UnknownEnvError(t *testing.T) {
	root, cleanup := newTestRoot(t)
	defer cleanup()

	writeInventory(t, root, &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]controlplane.Environment{
			"local": {Name: "local", Kind: "local", Servers: nil},
		},
	})

	cmd := freshCmd(t, envTargetProbeCmd, nil)

	err := runEnvTargetProbe(cmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown environment")
	}
}

// ── T03: migrate ─────────────────────────────────────────────────────────────

func TestEnvTargetMigrate_FromEnvVars(t *testing.T) {
	root, cleanup := newTestRoot(t)
	defer cleanup()

	// Simulate legacy env vars.
	t.Setenv("NSELF_DEPLOY_HOST_STAGING", "ubuntu@staging.example.com")
	t.Setenv("NSELF_SSH_KEY_STAGING", "/nonexistent/key") // value irrelevant for this test

	if err := runEnvTargetMigrate(freshCmd(t, envTargetMigrateCmd, nil), nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// The file must exist and be 0600 (perm check is Unix only).
	path := filepath.Join(root, ".nself", "control-plane.yaml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("control-plane.yaml not found: %v", err)
	}
	if runtime.GOOS != "windows" {
		if info.Mode().Perm() != 0o600 {
			t.Errorf("mode: got %o, want 0600", info.Mode().Perm())
		}
	}

	inv := readInventory(t, root)
	if _, ok := inv.Environments["staging"]; !ok {
		t.Error("staging environment not synthesized from env vars")
	}
}

func TestEnvTargetMigrate_Idempotent(t *testing.T) {
	root, cleanup := newTestRoot(t)
	defer cleanup()

	// Seed with an existing inventory.
	writeInventory(t, root, &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "test",
		Environments: map[string]controlplane.Environment{
			"local": {
				Name:    "local",
				Kind:    "local",
				Servers: []controlplane.Server{{Name: "local-app", Role: controlplane.RoleApp, Primary: true}},
			},
		},
	})

	// Run migrate twice; the second run must not corrupt the file.
	if err := runEnvTargetMigrate(freshCmd(t, envTargetMigrateCmd, nil), nil); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := runEnvTargetMigrate(freshCmd(t, envTargetMigrateCmd, nil), nil); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	inv := readInventory(t, root)
	if len(inv.Environments) < 1 {
		t.Error("inventory should not be empty after idempotent migrate")
	}
}

// ── helpers: rejectInlineSecret ──────────────────────────────────────────────

func TestRejectInlineSecret(t *testing.T) {
	cases := []struct {
		flagName string
		value    string
		wantErr  bool
	}{
		{"--key-ref", "NSELF_SSH_KEY_STAGING", false},              // valid env-var name
		{"--key-ref", "/home/ubuntu/.ssh/id_ed25519", true},        // file path rejected
		{"--key-ref", "ssh-ed25519 AAAA...", true},                 // inline key rejected
		{"--key-ref", "-----BEGIN OPENSSH PRIVATE KEY-----", true}, // PEM rejected
		{"--host", "ubuntu@staging.example.com", false},            // valid host
		{"--host", "ubuntu@staging.example.com\ninjected", true},   // multi-line rejected
	}

	for _, c := range cases {
		err := rejectInlineSecret(c.flagName, c.value)
		if c.wantErr && err == nil {
			t.Errorf("rejectInlineSecret(%q, %q): expected error, got nil", c.flagName, c.value)
		}
		if !c.wantErr && err != nil {
			t.Errorf("rejectInlineSecret(%q, %q): unexpected error: %v", c.flagName, c.value, err)
		}
	}
}

// ── helpers: sortedEnvNames ──────────────────────────────────────────────────

func TestSortedEnvNames_LocalFirst(t *testing.T) {
	inv := &controlplane.Inventory{
		Environments: map[string]controlplane.Environment{
			"prod":    {Name: "prod"},
			"local":   {Name: "local"},
			"staging": {Name: "staging"},
		},
	}
	names := sortedEnvNames(inv)
	if names[0] != "local" {
		t.Errorf("expected 'local' first, got %q", names[0])
	}
}
