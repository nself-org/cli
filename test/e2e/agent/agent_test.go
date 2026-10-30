//go:build e2eagent

package agent

// Agent E2E harness tests (P7-SHIP-02, SHIP Epic D7). Run through
// scripts/e2e/agent/run.sh; each test reads its inputs from AGENT_E2E_* env.
//
//   TestVerifyFixtures, TestReplayFile   unit tests, always run
//   TestVerifyTranscriptFile             AGENT_E2E_TRANSCRIPT=<file>
//   TestToolsList                        AGENT_E2E_BIN, AGENT_E2E_TOOLS_OUT[, AGENT_E2E_ENV]
//   TestAgentReplay                      AGENT_E2E_BIN, INTEGRATION=1, Docker[, AGENT_E2E_TRANSCRIPT_OUT]

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// replayTimeout bounds one tool call in the replay and live runs: a cold
// `start` pulls every image, which outlasts the server's 5m default.
const replayTimeout = "30m"

func TestVerifyFixtures(t *testing.T) {
	good, err := ReadTranscript(filepath.Join("testdata", "good.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(good); err != nil {
		t.Fatalf("good fixture: %v", err)
	}
	for name, want := range map[string]string{
		"non-mcp":              "non-MCP tool use",
		"missing-step":         `missing step "db migrate up"`,
		"promote-order":        "promote apply before its dry-run",
		"bad-envelope":         "fails the v1 schema",
		"evidence-before-wait": "ci evidence before ci runs wait",
	} {
		recs, err := ReadTranscript(filepath.Join("testdata", "bad", name+".jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if err := Verify(recs); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("bad/%s: want an error containing %q, got %v", name, want, err)
		}
	}
}

func TestReplayFile(t *testing.T) {
	steps, err := LoadReplay(filepath.Join("testdata", "replay.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, s := range steps {
		if seen[s.ID] {
			t.Errorf("duplicate step id %q", s.ID)
		}
		seen[s.ID] = true
		if s.Role == RoleAgent && s.Kind != KindToolsList && !strings.HasPrefix(s.Tool, "nself_") {
			t.Errorf("step %s: agent tool %q is not an nself MCP tool", s.ID, s.Tool)
		}
		if s.Role == RoleHarness && s.Action == "" {
			t.Errorf("step %s: harness step without an action", s.ID)
		}
	}
}

func TestVerifyTranscriptFile(t *testing.T) {
	path := os.Getenv("AGENT_E2E_TRANSCRIPT")
	if path == "" {
		t.Skip("AGENT_E2E_TRANSCRIPT is not set")
	}
	recs, err := ReadTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(recs); err != nil {
		t.Fatalf("%s:\n%v", path, err)
	}
}

// startClient starts and initialises an MCP client over the sandboxed server.
func startClient(t *testing.T, box *Sandbox, bin, operatorHome string, env ...string) *client.Client {
	t.Helper()
	t.Setenv("SSH_AUTH_SOCK", "")
	prog, args := box.ServerCommand(bin, operatorHome, env...)
	c, err := client.NewStdioMCPClient(prog, nil, args...)
	if err != nil {
		t.Fatalf("start nself mcp in the sandbox: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	req := mcp.InitializeRequest{}
	req.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	req.Params.ClientInfo = mcp.Implementation{Name: "nself-agent-e2e", Version: "1"}
	if _, err := c.Initialize(context.Background(), req); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return c
}

func TestToolsList(t *testing.T) {
	bin, out := os.Getenv("AGENT_E2E_BIN"), os.Getenv("AGENT_E2E_TOOLS_OUT")
	if bin == "" || out == "" {
		t.Skip("AGENT_E2E_BIN and AGENT_E2E_TOOLS_OUT are not set")
	}
	box, err := NewSandbox(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Outside a project the server lists only init and the read-only tools;
	// a project marker makes it list the whole surface (the parity input).
	if err := os.WriteFile(filepath.Join(box.Project, ".env"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var env []string
	if e := os.Getenv("AGENT_E2E_ENV"); e != "" {
		env = strings.Split(e, ",")
	}
	c := startClient(t, box, bin, os.Getenv("HOME"), env...)
	res, err := c.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		Name        string          `json:"name"`
		InputSchema json.RawMessage `json:"inputSchema"`
	}
	rows := make([]row, 0, len(res.Tools))
	for _, tool := range res.Tools {
		b, err := json.Marshal(tool)
		if err != nil {
			t.Fatal(err)
		}
		var r row
		if err := json.Unmarshal(b, &r); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAgentReplay(t *testing.T) {
	bin := os.Getenv("AGENT_E2E_BIN")
	if bin == "" {
		t.Skip("AGENT_E2E_BIN is not set")
	}
	operatorHome := os.Getenv("HOME")
	if out, err := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output(); err == nil {
		t.Setenv("DOCKER_HOST", strings.TrimSpace(string(out)))
	}
	fleet, hosts := StartSim(t)
	box, err := NewSandbox(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	box.Timeout = replayTimeout
	if err := box.WriteDockerConfig(); err != nil {
		t.Fatal(err)
	}
	for _, step := range []func() error{
		func() error { return box.InstallKey(fleet.KeyPath()) },
		box.WriteSSHConfig,
		func() error { return box.WriteInventory(hosts...) },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", box.Home)
	trustHosts(t, hosts)
	tpath := os.Getenv("AGENT_E2E_TRANSCRIPT_OUT")
	if tpath == "" {
		tpath = filepath.Join(t.TempDir(), "replay.jsonl")
	}
	w, err := NewWriter(tpath)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	h := &SimHarness{T: t, Fleet: fleet, Box: box, Hosts: hosts}
	t.Cleanup(h.Teardown)
	ctx := context.Background()
	if _, err := run(ctx, box.Project, "git", "init", "-q"); err != nil {
		t.Fatal(err)
	}
	steps, err := LoadReplay(filepath.Join("testdata", "replay.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	c := startClient(t, box, bin, operatorHome)
	r := &Runner{C: c, H: h, W: w}
	if err := r.Run(ctx, steps); err != nil {
		t.Fatalf("replay stopped (transcript %s): %v", tpath, err)
	}
	if err := Verify(w.Records); err != nil {
		t.Fatalf("transcript %s does not verify:\n%v", tpath, err)
	}
}

// trustHosts records each sim host's ed25519 key in the sandbox HOME (the
// HOME must already be the sandbox's), as the deploy-sim tests do.
func trustHosts(t *testing.T, hosts []SimHost) {
	t.Helper()
	for _, h := range hosts {
		scan, err := exec.Command("ssh-keyscan", "-T", "5", "-t", "ed25519", "-p", strconv.Itoa(h.Port), "--", LoopbackHost).Output()
		if err != nil {
			t.Fatalf("ssh-keyscan %s: %v", h.Server, err)
		}
		fp := ""
		for _, line := range strings.Split(string(scan), "\n") {
			if got, err := remote.Fingerprint(line); err == nil {
				fp = got
				break
			}
		}
		if fp == "" {
			t.Fatalf("no ed25519 key from %s", h.Server)
		}
		if err := controlplane.TrustHostKey(context.Background(), h.Host(), fp); err != nil {
			t.Fatalf("trust %s: %v", h.Server, err)
		}
	}
}
