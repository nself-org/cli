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
		args:  append(goldenBase("/k/id"), "deploy@203.0.113.7", "docker exec 'c' ls"),
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
			"/work/compose.yml", "deploy@203.0.113.7:/opt/app/nself-compose.yml",
		}},
		{tool: "ssh", probe: "inherited", args: append(append([]string{}, base...),
			"deploy@203.0.113.7", "docker compose -f /opt/app/nself-compose.yml pull")},
		{tool: "ssh", probe: "inherited", args: append(append([]string{}, base...),
			"deploy@203.0.113.7", "docker compose -f /opt/app/nself-compose.yml up -d")},
		{tool: "ssh", probe: "inherited", args: append(append([]string{}, base...),
			"deploy@203.0.113.7", "docker compose -f /opt/app/nself-compose.yml logs --follow")},
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

func TestGoldenArgv_PushSecrets(t *testing.T) {
	read := installFakeSSHTools(t)
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
		tool:  "scp",
		args:  append(goldenBase("/k/id"), envFile, "deploy@203.0.113.7:/opt/app/.env"),
		probe: "inherited",
	}}
	assertCalls(t, read(), want)
}
