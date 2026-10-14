package main

// Tests for Agent-Guide.md: it is the AGENTS.md renderer's output apart from
// the first line.

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/agentsetup"
)

func TestAgentGuideIsCanonicalRender(t *testing.T) {
	verbs := agentVerbs(hermeticRegistry(t))
	if len(verbs) == 0 {
		t.Fatal("no canon verbs")
	}
	page := renderAgentGuide(verbs)
	_, canonical, _ := strings.Cut(agentsetup.Render(verbs), "\n")
	_, got, _ := strings.Cut(page, "\n")
	if got != canonical {
		t.Fatal("Agent-Guide.md differs from the AGENTS.md render after the first line")
	}
	if !strings.HasPrefix(page, generatedNotice+"\n# AGENTS.md\n") {
		t.Error("page lacks the generated header")
	}
}

func TestAgentGuideFixtureDrift(t *testing.T) {
	a := []agentsetup.Verb{{Name: "init", Summary: "Scaffold"}}
	b := []agentsetup.Verb{{Name: "init", Summary: "Scaffold it"}}
	if renderAgentGuide(a) == renderAgentGuide(b) {
		t.Error("a changed verb summary did not change the page")
	}
	path := t.TempDir() + "/Agent-Guide.md"
	if changed, err := writePage(path, renderAgentGuide(a), false); err != nil || !changed {
		t.Fatalf("first write: %v %v", changed, err)
	}
	if changed, _ := writePage(path, renderAgentGuide(a), true); changed {
		t.Error("an unchanged page reported stale")
	}
	if changed, _ := writePage(path, renderAgentGuide(b), true); !changed {
		t.Error("-check did not report a changed template input")
	}
}
