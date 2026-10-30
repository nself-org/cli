//go:build e2eagent

package agent

// Live mode (run.sh --live; the session itself is P7-SHIP-13): a headless
// Claude Code session whose only tools are the sandboxed nself MCP server.
// The stream-json output is converted to the transcript format and verified
// like a replay. Needs AGENT_E2E_LIVE=1, AGENT_E2E_BIN, INTEGRATION=1, Docker
// and a `claude` login on the operator machine (not a production credential).

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// builtinTools are disallowed so the MCP server is the agent's only surface.
var builtinTools = []string{"Bash", "Edit", "MultiEdit", "Write", "Read", "Glob", "Grep", "LS",
	"WebFetch", "WebSearch", "NotebookEdit", "Task", "TodoWrite"}

func TestAgentLive(t *testing.T) {
	bin := os.Getenv("AGENT_E2E_BIN")
	if os.Getenv("AGENT_E2E_LIVE") != "1" || bin == "" {
		t.Skip("AGENT_E2E_LIVE=1 and AGENT_E2E_BIN are not set")
	}
	operatorHome := os.Getenv("HOME")
	claude, err := exec.LookPath("claude")
	if err != nil {
		t.Fatal("claude is not on PATH")
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
	if err := box.InstallKey(fleet.KeyPath()); err != nil {
		t.Fatal(err)
	}
	if err := box.WriteSSHConfig(); err != nil {
		t.Fatal(err)
	}
	if err := box.WriteInventory(hosts...); err != nil {
		t.Fatal(err)
	}
	tpath := os.Getenv("AGENT_E2E_TRANSCRIPT_OUT")
	if tpath == "" {
		tpath = filepath.Join(t.TempDir(), "live.jsonl")
	}
	w, err := NewWriter(tpath)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	ctx := context.Background()
	if _, err := run(ctx, box.Project, "git", "init", "-q"); err != nil {
		t.Fatal(err)
	}
	// The trust step needs the sandbox HOME; claude keeps the operator's.
	t.Setenv("HOME", box.Home)
	trustHosts(t, hosts)
	t.Setenv("HOME", operatorHome)
	t.Setenv("SSH_AUTH_SOCK", "")

	prog, args := box.ServerCommand(bin, operatorHome)
	cfg, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"nself": map[string]any{"command": prog, "args": args}}})
	cfgPath := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(cfgPath, cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	prompt, err := os.ReadFile(filepath.Join("testdata", "live-prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, claude, "-p", string(prompt), "--output-format", "stream-json", "--verbose",
		"--strict-mcp-config", "--mcp-config", cfgPath,
		"--disallowedTools", strings.Join(builtinTools, ","), "--allowedTools", "mcp__nself")
	cmd.Dir = t.TempDir()
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := convertStream(out, w); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("claude: %v", err)
	}
	if err := Verify(w.Records); err != nil {
		t.Fatalf("live transcript %s does not verify:\n%v", tpath, err)
	}
}

// streamLine is the part of a stream-json line the converter reads.
type streamLine struct {
	Type    string `json:"type"`
	Message struct {
		Content []struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Input     map[string]any  `json:"input"`
			ToolUseID string          `json:"tool_use_id"`
			Content   json.RawMessage `json:"content"`
			IsError   bool            `json:"is_error"`
		} `json:"content"`
	} `json:"message"`
}

// convertStream writes one agent record per tool_use (an MCP tool of the
// nself server keeps its bare name; anything else keeps its own name, which
// the verifier rejects) and one server record per tool_result.
func convertStream(r interface{ Read([]byte) (int, error) }, w *Writer) error {
	tools := map[string]string{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		var l streamLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		for _, c := range l.Message.Content {
			switch c.Type {
			case "tool_use":
				name := strings.TrimPrefix(c.Name, "mcp__nself__")
				tools[c.ID] = name
				if _, err := w.Write(Record{Role: RoleAgent, Kind: KindToolsCall, Tool: name, Arguments: c.Input}); err != nil {
					return err
				}
			case "tool_result":
				if _, err := w.Write(Record{Role: RoleServer, Kind: KindResult, Tool: tools[c.ToolUseID],
					Envelope: resultText(c.Content), IsError: c.IsError}); err != nil {
					return err
				}
			}
		}
	}
	return sc.Err()
}

// resultText returns the envelope JSON of a tool_result content value.
func resultText(raw json.RawMessage) json.RawMessage {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return compact(json.RawMessage(s))
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		for _, p := range parts {
			if p.Type == "text" {
				return compact(json.RawMessage(p.Text))
			}
		}
	}
	return raw
}
