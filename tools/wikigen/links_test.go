package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCommandPagesDoNotLinkPrivateWorkspace(t *testing.T) {
	pages, err := filepath.Glob("../../.github/wiki/cmd-*.md")
	if err != nil || len(pages) == 0 {
		t.Fatalf("find command pages: %v", err)
	}
	links := regexp.MustCompile(`https?://[^\s<>\x60)]+`)
	for _, page := range pages {
		body, err := os.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		for _, link := range links.FindAllString(string(body), -1) {
			if strings.Contains(link, "/.claude") {
				t.Errorf("%s links to private workspace: %s", filepath.Base(page), link)
			}
		}
	}
}

func TestInvalidPublicLinks(t *testing.T) {
	nav := map[string]bool{"": true, "plugins/pro/google": true}
	for _, tc := range []struct {
		body string
		bad  bool
	}{
		{"[private](https://github.com/nself-org/cli/blob/main/.claude)", true},
		{"[private](../.claude/plan.md)", true},
		{"[private](https://github.com/nself-org/cli/blob/main/%2eclaude/plan)", true},
		{"[private][plan]\n[plan]: .claude/plan.md", true},
		{"<https://nself.org/docs/templates/publish-template>", true},
		{"[revenue](https://nself.org/docs/developers/revenue-share)", true},
		{"[metrics](https://nself.org/docs/operations/observability)", true},
		{"[google](https://nself.org/docs/plugins/google)", true},
		{"[sdk](https://nself.org/docs/sdks/)", true},
		{"See https://nself.org/docs/new-missing-page.", true},
		{"[missing](/docs/missing)", true},
		{"[missing](//nself.org/docs/missing)", true},
		{"[google](https://nself.org/docs/plugins/pro/google/?mode=oauth#setup)", false},
		{"[google](/docs/plugins/pro/google)", false},
		{"[google][ref]\n[ref]: https://nself.org/docs/plugins/pro/google", false},
		{"See https://nself.org/docs.", false},
		{"[docs](https://nself.org/docs/)", false},
		{"[other](https://example.com/docs/missing)", false},
		{"Example endpoint: http://127.0.0.1:MINIO_PORT", false},
		{"[bad](https://nself.org/docs/%zz)", true},
		{"[other](https://nself.org/docs-other)", false},
		{"Add `.claude/settings.json` to your project.", false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			got := invalidPublicLinks(tc.body, nav)
			if (len(got) > 0) != tc.bad {
				t.Fatalf("invalid links = %v, want rejection %v", got, tc.bad)
			}
		})
	}
}

func TestRepositoryCommandLinks(t *testing.T) {
	if err := checkWikiLinks("../../.github/wiki"); err != nil {
		t.Fatal(err)
	}
}

func TestWikiCheckRejectsInvalidLinks(t *testing.T) {
	if dir := os.Getenv("WIKIGEN_LINK_CHECK_DIR"); dir != "" {
		os.Args = []string{"wikigen", "-check", "-dir", dir}
		main()
		return
	}
	dir := t.TempDir()
	page := filepath.Join(dir, "cmd-retired.md")
	body := "# Legacy command\n\n[missing](https://nself.org/docs/missing)\n"
	if err := os.WriteFile(page, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestWikiCheckRejectsInvalidLinks$")
	cmd.Env = append(os.Environ(), "WIKIGEN_LINK_CHECK_DIR="+dir)
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "cmd-retired.md: docs page missing from navigation") {
		t.Fatalf("wikigen -check: err=%v output=%s", err, out)
	}
	after, err := os.ReadFile(page)
	if err != nil || string(after) != body {
		t.Fatalf("check modified page: %v", err)
	}
}
