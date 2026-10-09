package commands

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/access"
	"github.com/nself-org/cli/internal/admin"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/version"
)

func accessInventoryFixture(t *testing.T) string {
	t.Helper()
	root, cleanup := newTestRoot(t)
	t.Cleanup(cleanup)
	writeInventory(t, root, &controlplane.Inventory{SchemaVersion: 2, Project: "test", Environments: map[string]controlplane.Environment{
		"local": {Name: "local", Kind: "local", Tier: controlplane.TierLocal},
		"qa": {Name: "qa", Kind: "remote", Tier: controlplane.TierLocalServers, Servers: []controlplane.Server{
			{Name: "app", Role: controlplane.RoleApp, Host: "u@qa-app.test:2222", RemotePath: "/srv/app", Primary: true},
			{Name: "db", Role: controlplane.RoleDB, Host: "u@qa-db.test:2223", RemotePath: "/srv/db"},
		}},
		"live": {Name: "live", Kind: "remote", Tier: controlplane.TierProd, Servers: []controlplane.Server{
			{Name: "live-app", Role: controlplane.RoleApp, Host: "u@live.test:2200", RemotePath: "/srv/live", Primary: true},
		}},
	}})
	return root
}

func TestAccessTargetsSelectors(t *testing.T) {
	root := accessInventoryFixture(t)
	t.Setenv("NSELF_V15", "1")
	t.Setenv("HETZNER_NSELF_TOKEN", "")
	defer resetFlags(accessGrantCmd)
	accessGrantCmd.SetContext(context.Background())
	for _, tc := range []struct {
		flag, value string
		want        []string
	}{
		{"tier", "local-servers", []string{"qa/app", "qa/db"}},
		{"env", "qa", []string{"qa/app", "qa/db"}},
		{"server", "db", []string{"qa/db"}},
	} {
		resetFlags(accessGrantCmd)
		_ = accessGrantCmd.Flags().Set(tc.flag, tc.value)
		targets, selected, err := resolveAccessTargets(accessGrantCmd)
		if err != nil || !selected || len(targets) != len(tc.want) {
			t.Fatalf("--%s %s: targets=%v selected=%v err=%v", tc.flag, tc.value, targets, selected, err)
		}
		for i, target := range targets {
			if target.Env+"/"+target.Server != tc.want[i] {
				t.Fatalf("--%s %s: target %d=%s/%s", tc.flag, tc.value, i, target.Env, target.Server)
			}
		}
	}
	resetFlags(accessGrantCmd)
	_ = accessGrantCmd.Flags().Set("host", "u@one.test")
	_ = accessGrantCmd.Flags().Set("env", "qa")
	if _, _, err := resolveAccessTargets(accessGrantCmd); err == nil {
		t.Fatal("--host with selector accepted")
	}
	resetFlags(accessGrantCmd)
	_ = accessGrantCmd.Flags().Set("env", "missing")
	if _, _, err := resolveAccessTargets(accessGrantCmd); err == nil {
		t.Fatal("unknown inventory env accepted")
	}
	// The CLI grant path must visit exactly the selected hosts. A dry-run
	// visits the same set while leaving every authorized_keys file absent.
	paths := map[string]string{}
	oldFactory := newAccessTargetTransport
	newAccessTargetTransport = func(_ string, _ string, env, server string, _ controlplane.Tier) access.Transport {
		key := env + "/" + server
		path := filepath.Join(root, env+"-"+server+"-authorized_keys")
		paths[key] = path
		return access.NewLocalFileTransport(path)
	}
	t.Cleanup(func() { newAccessTargetTransport = oldFactory })
	undoAudit := access.SetAuditLogPathForTest(filepath.Join(root, "audit.log"))
	t.Cleanup(undoAudit)
	resetFlags(accessGrantCmd)
	_ = accessGrantCmd.Flags().Set("tier", "local-servers")
	_ = accessGrantCmd.Flags().Set("user", "t")
	_ = accessGrantCmd.Flags().Set("key", testKeyLine)
	_ = accessGrantCmd.Flags().Set("dry-run", "true")
	dryRows := captureAccessStdout(t, func() error { return runAccessGrant(accessGrantCmd, nil) })
	if strings.Count(dryRows, "status=dry-run") != 2 {
		t.Fatalf("dry-run rows: %q", dryRows)
	}
	if len(paths) != 2 {
		t.Fatalf("dry-run reached %d hosts, want 2", len(paths))
	}
	for key, path := range paths {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("dry-run changed %s: %v", key, err)
		}
	}
	_ = accessGrantCmd.Flags().Set("dry-run", "false")
	grantRows := captureAccessStdout(t, func() error { return runAccessGrant(accessGrantCmd, nil) })
	if strings.Count(grantRows, "status=granted") != 2 {
		t.Fatalf("grant rows: %q", grantRows)
	}
	for _, key := range []string{"qa/app", "qa/db"} {
		b, err := os.ReadFile(paths[key])
		if err != nil || !strings.Contains(string(b), "t") {
			t.Fatalf("grant missed %s: %v", key, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "live-app-authorized_keys")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("grant touched prod host")
	}
	defer resetFlags(accessListCmd)
	accessListCmd.SetContext(context.Background())
	_ = accessListCmd.Flags().Set("tier", "local-servers")
	_ = accessListCmd.Flags().Set("json", "true")
	listed := captureAccessStdout(t, func() error { return runAccessList(accessListCmd, nil) })
	if strings.Count(listed, `"server":`) != 2 || !strings.Contains(listed, `"server": "app"`) || !strings.Contains(listed, `"server": "db"`) {
		t.Fatalf("selector list must have one row per host: %s", listed)
	}
	defer resetFlags(accessRevokeCmd)
	accessRevokeCmd.SetContext(context.Background())
	_ = accessRevokeCmd.Flags().Set("tier", "local-servers")
	_ = accessRevokeCmd.Flags().Set("user", "t")
	if err := runAccessRevoke(accessRevokeCmd, nil); err == nil || !strings.Contains(err.Error(), "last key") {
		t.Fatalf("last-key guard lost: %v", err)
	}
	for _, key := range []string{"qa/app", "qa/db"} {
		b, err := os.ReadFile(paths[key])
		if err != nil || !strings.Contains(string(b), "user=t") {
			t.Fatalf("revoke changed %s: %v", key, err)
		}
	}
}

func TestAccessProdConfirm(t *testing.T) {
	root := accessInventoryFixture(t)
	t.Setenv("NSELF_V15", "1")
	t.Setenv("HETZNER_NSELF_TOKEN", "")
	undoAudit := access.SetAuditLogPathForTest(filepath.Join(root, "audit.log"))
	t.Cleanup(undoAudit)
	defer resetFlags(accessGrantCmd)
	accessGrantCmd.SetContext(context.Background())
	_ = accessGrantCmd.Flags().Set("env", "live")
	_ = accessGrantCmd.Flags().Set("user", "t")
	_ = accessGrantCmd.Flags().Set("key", testKeyLine)
	path := filepath.Join(root, "should-not-exist")
	old := newAccessTargetTransport
	newAccessTargetTransport = func(_, _, _, _ string, _ controlplane.Tier) access.Transport {
		return access.NewLocalFileTransport(path)
	}
	t.Cleanup(func() { newAccessTargetTransport = old })
	err := runAccessGrant(accessGrantCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "E403") || errs.ExitCodeFor(err) != 4 {
		t.Fatalf("prod gate: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prod gate wrote authorized_keys: %v", err)
	}
	_ = accessGrantCmd.Flags().Set("dry-run", "true")
	if err := runAccessGrant(accessGrantCmd, nil); err != nil {
		t.Fatalf("prod dry-run refused: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run wrote authorized_keys: %v", err)
	}
	_ = accessGrantCmd.Flags().Set("dry-run", "false")
	_ = accessGrantCmd.Flags().Set("yes", "true")
	if err := runAccessGrant(accessGrantCmd, nil); err != nil {
		t.Fatalf("--yes grant refused: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("--yes did not grant: %v", err)
	}
}

func TestAccessListHostGolden(t *testing.T) {
	withFixtureTransport(t)
	defer resetFlags(accessListCmd)
	accessListCmd.SetContext(context.Background())
	_ = accessListCmd.Flags().Set("host", "u@golden.test")
	_ = accessListCmd.Flags().Set("json", "true")
	out := captureAccessStdout(t, func() error { return runAccessList(accessListCmd, nil) })
	if out != "[]\n" {
		t.Fatalf("legacy --host JSON changed: %q", out)
	}
}

func captureAccessStdout(t *testing.T, run func() error) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	runErr := run()
	_ = w.Close()
	os.Stdout = old
	b, _ := io.ReadAll(r)
	_ = r.Close()
	if runErr != nil {
		t.Fatal(runErr)
	}
	return string(b)
}

func TestAccessSecurityArgvSites(t *testing.T) {
	// No network call: all SSH executions are captured by a PATH stub.
	t.Setenv("NSELF_V15", "0")
	bin := t.TempDir()
	log := filepath.Join(bin, "ssh-argv")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + log + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	if err := admin.VerifySSHKey(context.Background(), "u", "host.test", 2222); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "-p\n2222\n--\nu@host.test\ntrue\n") {
		t.Fatalf("admin argv: %q", b)
	}
	_ = os.Remove(log)
	if err := admin.VerifySSHKey(context.Background(), "u", "-oProxyCommand=touch", 2222); err == nil {
		t.Fatal("option-like admin host accepted")
	}
	if _, err := os.Stat(log); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("hostile host reached ssh")
	}

	old := runSSHCaptured
	called := false
	runSSHCaptured = func(_ context.Context, args []string) (string, error) {
		called = true
		joined := strings.Join(args, "|")
		if !strings.Contains(joined, "-p|2223|--|u@db.test|nself version --json") {
			t.Fatalf("db probe argv: %q", args)
		}
		return `{"capabilities":["` + version.CapDBDryRunSafe + `"]}`, nil
	}
	defer func() { runSSHCaptured = old }()
	if err := checkRemoteDryRunSupport(context.Background(), dbRemoteTarget{SSHTarget: "u@db.test:2223", EnvName: "qa"}, []string{"-o", "BatchMode=yes"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("db dry-run probe did not run")
	}
	called = false
	if err := checkRemoteDryRunSupport(context.Background(), dbRemoteTarget{SSHTarget: "-oProxyCommand=touch", EnvName: "qa"}, nil); err == nil {
		t.Fatal("hostile db host accepted")
	}
	if called {
		t.Fatal("hostile db host reached ssh")
	}
}
