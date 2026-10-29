package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWiki(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func audit(t *testing.T, files map[string]string) string {
	t.Helper()
	bad, err := auditProseLinks(writeWiki(t, files))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(bad, "\n")
}

// A wikilink to a page that does not exist is reported on cmd-* pages and on
// every other page (D-0717).
func TestAuditProseLinksBrokenWikilink(t *testing.T) {
	got := audit(t, map[string]string{
		"cmd-start.md":       "See [[Guide-Migration]]\n",
		"Licensing-Guide.md": "- [[bundle-ntask]] and [[Home]]\n",
		"Home.md":            "home\n",
	})
	for _, want := range []string{
		"cmd-start.md: broken wikilink [[Guide-Migration]]",
		"Licensing-Guide.md: broken wikilink [[bundle-ntask]]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "[[Home]]") {
		t.Errorf("existing page reported broken:\n%s", got)
	}
}

// Targets resolve by basename (subdirectories are flattened), with anchors,
// pipes and a trailing .md ignored; fenced and inline code are not links.
func TestAuditProseLinksResolution(t *testing.T) {
	got := audit(t, map[string]string{
		"Home.md":          "[[releasing]] [[sdk/releasing#top]] [[releasing.md]] [[Home|the home page]]\n`[[nope]]`\n```\n[[also-nope]]\n```\n~~~\n[[tilde-nope]]\n~~~\n",
		"sdk/releasing.md": "x\n",
	})
	if got != "" {
		t.Errorf("expected no findings, got:\n%s", got)
	}
}

// The docs-URL allowlist is exactly the four published nav pages.
func TestAuditProseLinksDocsAllowlist(t *testing.T) {
	allowed := []string{"commands", "plugins", "guides", "reference"}
	var body strings.Builder
	for _, a := range allowed {
		body.WriteString("[x](https://nself.org/docs/" + a + ")\n")
	}
	if got := audit(t, map[string]string{"cmd-a.md": body.String()}); got != "" {
		t.Errorf("allowlisted URLs flagged:\n%s", got)
	}
	got := audit(t, map[string]string{"cmd-a.md": "[x](https://nself.org/docs/migrate/from-v0.9)\nhttps://nself.org/docs/commands/db.\n"})
	for _, want := range []string{"broken docs link https://nself.org/docs/migrate/from-v0.9", "broken docs link https://nself.org/docs/commands/db."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// Docs URLs in fenced CLI output are verbatim text, not links, and docs URLs
// on non-cmd pages are outside this audit; a .claude link on a cmd page fails.
func TestAuditProseLinksScope(t *testing.T) {
	got := audit(t, map[string]string{
		"cmd-start.md": "```\nSee https://nself.org/docs/migrate/from-v0.9\n```\n[c](https://github.com/nself-org/cli/blob/main/.claude/x.md)\n",
		"other.md":     "https://nself.org/docs/licensing/offline\n[c](https://github.com/nself-org/cli/blob/main/.claude/x.md)\n",
	})
	if got != "cmd-start.md: contains .claude/ link" {
		t.Errorf("unexpected findings:\n%s", got)
	}
}

// Retired tier rows and the Basic-grandfathering line fail on the pricing
// pages only; the bundle model passes (D-1289).
func TestAuditProseLinksStaleTiers(t *testing.T) {
	stale := "| **Elite** | $4.99/mo | Pro |\n| Business+ | $49.99 | x |\n| Enterprise | $99.99 | x |\nExisting $9.99/yr keys are grandfathered to the Basic tier.\n"
	got := audit(t, map[string]string{
		"Plugin-Overview.md":  stale,
		"Plugin-Licensing.md": stale,
		"Support.md":          stale,
	})
	for _, page := range []string{"Plugin-Overview.md", "Plugin-Licensing.md"} {
		if n := strings.Count(got, page+": stale tier pricing line"); n != 4 {
			t.Errorf("%s: want 4 findings, got %d in:\n%s", page, n, got)
		}
	}
	if strings.Contains(got, "Support.md") {
		t.Errorf("non-pricing page flagged:\n%s", got)
	}
	ok := "| **Bundle** | $0.99/mo | One product |\n| ɳSelf+ | $3.99 | All |\nThere is no Basic tier.\n"
	if got := audit(t, map[string]string{"Plugin-Overview.md": ok}); got != "" {
		t.Errorf("bundle model flagged:\n%s", got)
	}
}

// Bundle pages must not sell a retired Basic/Pro plan or the retired $1.99
// Pro price; a negation ("no Basic tier") and non-bundle pages pass (D-1289).
func TestAuditProseLinksBundlePagesStalePlans(t *testing.T) {
	stale := "Pro tier is $1.99/mo or $19.99/yr.\n- Basic tier license key or higher\n# Pro tier required for tier: max plugins\n"
	got := audit(t, map[string]string{
		"bundle-nclaw.md": stale,
		"Support.md":      stale,
	})
	if n := strings.Count(got, "bundle-nclaw.md: stale tier pricing line"); n != 3 {
		t.Errorf("bundle-nclaw.md: want 3 findings, got %d in:\n%s", n, got)
	}
	if strings.Contains(got, "Support.md") {
		t.Errorf("non-pricing page flagged:\n%s", got)
	}
	ok := "The ɳClaw bundle is $0.99/mo; there is no separate Basic or Pro plan.\nThere is no Basic tier.\n"
	if got := audit(t, map[string]string{"bundle-nclaw.md": ok}); got != "" {
		t.Errorf("bundle model flagged:\n%s", got)
	}
}
