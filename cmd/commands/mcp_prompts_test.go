package commands

// Purpose: the authored prompts must only name commands and tools that exist.
// Every `nself …` spelling in the three prompts resolves in the v1.5 registry,
// and every tool or resource name resolves on the v1.5 server and, for tools,
// on the v1.4 server too.
// Constraints: fresh process (the real tree is shared); no network.

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

var (
	promptSpelling = regexp.MustCompile(`\bnself ((?:[a-z][a-z0-9-]*)(?: [a-z][a-z0-9-]*)*)`)
	promptTool     = regexp.MustCompile(`\b(nself_[a-z0-9_]+)\b`)
	promptResource = regexp.MustCompile(`nself://[a-z/]+`)
)

// promptTexts renders every prompt with sample arguments.
func promptTexts(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, h := range map[string]struct {
		args map[string]string
		get  func(context.Context, mcp.GetPromptRequest) (*mcp.GetPromptResult, error)
	}{
		"diagnose-failure": {map[string]string{"service": "db"}, mcpDiagnoseFailurePrompt()},
		"add-service":      {map[string]string{"name": "mlflow"}, mcpAddServicePrompt()},
		"prepare-deploy":   {map[string]string{"env": "prod"}, mcpPrepareDeployPrompt()},
	} {
		req := mcp.GetPromptRequest{}
		req.Params.Arguments = h.args
		res, err := h.get(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range res.Messages {
			out[name] += m.Content.(mcp.TextContent).Text + "\n"
		}
	}
	return out
}

func TestMCPPromptSpellings(t *testing.T) {
	if !inFreshProcess(t, "TestMCPPromptSpellings") {
		return
	}
	stdioFixture(t, true)
	reg, err := machineRegistry()
	if err != nil {
		t.Fatal(err)
	}
	texts := promptTexts(t)
	if len(texts) != 3 {
		t.Fatalf("%d prompts", len(texts))
	}
	s, err := newMCPServer("stdio", true)
	if err != nil {
		t.Fatal(err)
	}
	sess := startStdio(t, s) // one stdio session per server
	v15Tools := toolsOf(t, sess)
	hand := map[string]bool{}
	for _, n := range legacyHandNames(t) {
		hand[n] = true
	}
	var rl struct{ Resources []struct{ URI string } }
	_ = json.Unmarshal(sess.call("resources/list", `{}`), &rl)
	resources := map[string]bool{}
	for _, r := range rl.Resources {
		resources[r.URI] = true
	}
	checked := 0
	for name, text := range texts {
		for _, m := range promptSpelling.FindAllStringSubmatch(text, -1) {
			path := strings.TrimSpace(m[1])
			if _, ok := reg.Lookup(path); !ok {
				t.Errorf("prompt %s names `nself %s`, which is not in the v1.5 registry", name, path)
			}
			checked++
		}
		for _, m := range promptTool.FindAllStringSubmatch(text, -1) {
			if _, ok := v15Tools[m[1]]; !ok {
				t.Errorf("prompt %s names the tool %s, which the v1.5 server does not register", name, m[1])
			}
			if !hand[m[1]] {
				t.Errorf("prompt %s names the tool %s, which the v1.4 server does not register", name, m[1])
			}
			checked++
		}
		for _, u := range promptResource.FindAllString(text, -1) {
			if !resources[u] {
				t.Errorf("prompt %s names the resource %s, which the v1.5 server does not list", name, u)
			}
			checked++
		}
	}
	if checked < 10 {
		t.Fatalf("only %d spellings were checked", checked)
	}
	t.Log("CHECKED-TestMCPPromptSpellings")
}
