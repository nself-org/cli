package deploy

// Purpose: golden argv test for every ssh/scp/rsync exec site in this package.
// Inputs:  fake ssh, scp and rsync executables placed first on PATH; they record
//          their argv and one inherited environment variable, then exit 0.
// Outputs: asserts the argv and environment each site produced. The expected
//          slices were written from origin/main's remote_exec.go, ssh.go and
//          secrets.go BEFORE the sites were delegated to sdk/go/remote, so the
//          test fails if delegation changes a single byte of argv.
// Constraints: never connects to a host: only the fake binaries run.

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

const goldenProbeEnv = "NSELF_GOLDEN_PROBE"

type recordedCall struct {
	tool  string
	args  []string
	probe string
}

// installFakeSSHTools puts recording ssh, scp and rsync stubs first on PATH and
// returns a function that reads back every recorded call in order.
func installFakeSSHTools(t *testing.T) func() []recordedCall {
	t.Helper()
	if runtime.GOOS == "windows" {
		// The stubs are POSIX shell scripts; Windows would resolve the real
		// ssh.exe instead. Never run these tests against a real client.
		t.Skip("argv golden test uses POSIX shell stubs for ssh, scp and rsync")
	}
	dir := t.TempDir()
	logFile := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" +
		"{ echo \"TOOL $(basename \"$0\")\"; echo \"PROBE ${" + goldenProbeEnv + "}\";" +
		" for a in \"$@\"; do printf 'ARG %s\\n' \"$a\"; done; echo END; } >> \"" + logFile + "\"\n" +
		"exit 0\n"
	// Create the log up front so a run that records nothing reads back as zero calls.
	if err := os.WriteFile(logFile, nil, 0o644); err != nil {
		t.Fatalf("create call log: %v", err)
	}
	for _, tool := range []string{"ssh", "scp", "rsync"} {
		if err := os.WriteFile(filepath.Join(dir, tool), []byte(script), 0o755); err != nil {
			t.Fatalf("write fake %s: %v", tool, err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(goldenProbeEnv, "inherited")
	return func() []recordedCall {
		raw, err := os.ReadFile(logFile)
		if err != nil {
			t.Fatalf("read call log: %v", err)
		}
		var calls []recordedCall
		var cur recordedCall
		for _, line := range strings.Split(string(raw), "\n") {
			switch {
			case strings.HasPrefix(line, "TOOL "):
				cur = recordedCall{tool: strings.TrimPrefix(line, "TOOL ")}
			case strings.HasPrefix(line, "PROBE "):
				cur.probe = strings.TrimPrefix(line, "PROBE ")
			case strings.HasPrefix(line, "ARG "):
				cur.args = append(cur.args, strings.TrimPrefix(line, "ARG "))
			case line == "END":
				calls = append(calls, cur)
			}
		}
		return calls
	}
}

func goldenBase(key string) []string {
	return []string{"-i", key, "-o", "StrictHostKeyChecking=accept-new", "-o", "ForwardAgent=no"}
}

func assertCalls(t *testing.T, got, want []recordedCall) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("recorded %d calls, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].tool != want[i].tool || !reflect.DeepEqual(got[i].args, want[i].args) {
			t.Errorf("call %d: got %s %q\nwant %s %q", i, got[i].tool, got[i].args, want[i].tool, want[i].args)
		}
		if got[i].probe != want[i].probe {
			t.Errorf("call %d: inherited env probe = %q, want %q", i, got[i].probe, want[i].probe)
		}
	}
}

func TestGoldenArgv_RunRemoteCommand(t *testing.T) {
	read := installFakeSSHTools(t)
	rt := RemoteTarget{SSHTarget: "deploy@203.0.113.7", KeyPath: "/k/id"}
	if _, err := RunRemoteCommand(context.Background(), rt, "docker exec 'c' ls"); err != nil {
		t.Fatalf("RunRemoteCommand: %v", err)
	}
	want := []recordedCall{{
		tool:  "ssh",
		args:  append(goldenBase("/k/id"), "--", "deploy@203.0.113.7", "docker exec 'c' ls"),
		probe: "inherited",
	}}
	assertCalls(t, read(), want)
}

func TestGoldenArgv_DeployViaSsh(t *testing.T) {
	read := installFakeSSHTools(t)
	cfg := SSHConfig{Host: "deploy@203.0.113.7:/opt/app", KeyPath: "/k/id", Follow: true}
	if err := DeployViaSsh(context.Background(), cfg, "/work/compose.yml"); err != nil {
		t.Fatalf("DeployViaSsh: %v", err)
	}
	base := goldenBase("/k/id")
	want := []recordedCall{
		{tool: "rsync", probe: "inherited", args: []string{
			"-az", "-e", "ssh " + strings.Join(base, " "),
			"--", "/work/compose.yml", "deploy@203.0.113.7:/opt/app/nself-compose.yml",
		}},
		{tool: "ssh", probe: "inherited", args: append(append([]string{}, base...),
			"--", "deploy@203.0.113.7", "docker compose -f '/opt/app/nself-compose.yml' pull")},
		{tool: "ssh", probe: "inherited", args: append(append([]string{}, base...),
			"--", "deploy@203.0.113.7", "docker compose -f '/opt/app/nself-compose.yml' up -d")},
		{tool: "ssh", probe: "inherited", args: append(append([]string{}, base...),
			"--", "deploy@203.0.113.7", "docker compose -f '/opt/app/nself-compose.yml' logs --follow")},
	}
	assertCalls(t, read(), want)
}

func TestGoldenArgv_DeployViaSshDefaultRemotePath(t *testing.T) {
	read := installFakeSSHTools(t)
	cfg := SSHConfig{Host: "deploy@203.0.113.7", KeyPath: "/k/id"}
	if err := DeployViaSsh(context.Background(), cfg, "/work/compose.yml"); err != nil {
		t.Fatalf("DeployViaSsh: %v", err)
	}
	calls := read()
	if len(calls) != 3 {
		t.Fatalf("recorded %d calls, want 3 (no follow)", len(calls))
	}
	if got := calls[0].args[len(calls[0].args)-1]; got != "deploy@203.0.113.7:/tmp/nself-compose.yml" {
		t.Errorf("rsync destination = %q", got)
	}
}

// TestGoldenArgv_PushSecrets changes ON PURPOSE (P7-DEPL-28, D-0248): secret
// shipping is strict in v1.4 too, so the argv now carries the pinned host-key
// policy, a HostKeyAlias for the IP host, and an absolute local operand.
func TestGoldenArgv_PushSecrets(t *testing.T) {
	requireKeygen(t)
	t.Setenv("HOME", pinHome(t))
	read := installFakeSSHTools(t)
	installFakeKeyscan(t)
	if err := (remote.PinnedHostKeys{Path: DeployKnownHostsPath()}).Add("nself-203.0.113.7-22", "ssh-ed25519 AQID"); err != nil {
		t.Fatalf("pin: %v", err)
	}
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env.prod")
	if err := os.WriteFile(envFile, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := SSHConfig{Host: "deploy@203.0.113.7:/ignored", KeyPath: "/k/id"}
	err := PushSecrets(context.Background(), cfg, PushSecretsOptions{
		Target: "prod", EnvFile: envFile, RemotePath: "/opt/app/.env",
	})
	if err != nil {
		t.Fatalf("PushSecrets: %v", err)
	}
	want := []recordedCall{{
		tool: "scp",
		args: []string{
			"-i", "/k/id",
			"-o", "StrictHostKeyChecking=yes",
			"-o", "GlobalKnownHostsFile=" + DeployKnownHostsPath(),
			"-o", "HostKeyAlias=nself-203.0.113.7-22",
			"-o", "ForwardAgent=no",
			"--", envFile, "deploy@203.0.113.7:/opt/app/.env",
		},
		probe: "inherited",
	}}
	assertCalls(t, read(), want)
}

// TestGoldenArgv_Transport pins the argv of the exported transport builder the
// v1.4 `nself deploy <target>` push uses (P7-DEPL-29): ssh, a multi-source
// rsync and a rsync with a long flag, all from one option list.
func TestGoldenArgv_Transport(t *testing.T) {
	spec, err := remote.ParseHostSpec("deploy@203.0.113.7")
	if err != nil {
		t.Fatal(err)
	}
	policy := []string{"-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=/h/deploy_known_hosts"}
	tr, err := NewTransport(spec, "/k/id", policy)
	if err != nil {
		t.Fatal(err)
	}
	opts := "-i /k/id -o StrictHostKeyChecking=yes -o UserKnownHostsFile=/h/deploy_known_hosts -o ForwardAgent=no"
	if got, want := tr.SSHArgv("docker compose pull"), strings.Fields(opts+" -- deploy@203.0.113.7"); !reflect.DeepEqual(got[:len(got)-1], want) || got[len(got)-1] != "docker compose pull" {
		t.Errorf("ssh argv = %q", got)
	}
	got, err := tr.RsyncArgv(nil, "/opt/nself/", "/w/docker-compose.yml", "/w/.nself-deploy-env-qa-1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-az", "-e", "ssh " + opts, "--", "/w/docker-compose.yml", "/w/.nself-deploy-env-qa-1", "deploy@203.0.113.7:/opt/nself/"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rsync argv = %q\nwant %q", got, want)
	}
	got, err = tr.RsyncArgv([]string{"--delete"}, "/opt/nself/hasura/", "/w/hasura/")
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"-az", "--delete", "-e", "ssh " + opts, "--", "/w/hasura/", "deploy@203.0.113.7:/opt/nself/hasura/"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rsync --delete argv = %q\nwant %q", got, want)
	}
	if _, err := tr.RsyncArgv([]string{"-e evil"}, "/opt/nself/", "/w/x"); err == nil {
		t.Error("a flag that is not a long option was accepted")
	}
}
