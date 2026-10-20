package repoqa

// wiki_offering_test.go — the licensing and bundle wiki pages state the offering
// the VMI carries and nothing else (D-1289).
//
// Purpose: the offering is Free, one Task Bundle (free), five licensed Bundles
//          at one price, ɳSelf+ and ɳCloud. The retired Basic/Pro/Elite/Business
//          tier ladder, its prices and "planned v1.1.0" statuses must not return
//          to the pages that sell or explain the Bundles, and the pages must name
//          the same Bundles (Task and Sentry included).
// Constraints: reads files only; the page list is explicit so a new page joins
//          it on purpose. Prose that quotes the old ladder as history belongs in
//          a changelog, which this test does not read.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var offeringPages = []string{
	"Feature-Plugins.md", "Plugin-Licensing.md", "Plugin-Overview.md", "Licensing-Guide.md",
	"bundle-nchat.md", "bundle-nfamily.md", "bundle-nclaw.md", "bundle-clawde.md", "cmd-bundle.md",
}

var retiredOffering = regexp.MustCompile(`(?i)\bbasic\b|\belite\b|business\+|\bbusiness\s*\||pro[- ]tiers?\b|enterprise tier|\$(?:1\.99|4\.99|49\.99|99\.99|19\.99|499\.99|999\.99)|planned v1\.1\.0|\(planned\)|will be available|` + "`tier: (?:pro|max)`")

func TestWikiOfferingPagesCarryNoRetiredTiers(t *testing.T) {
	dir := filepath.Join(repoRoot(t), ".github", "wiki")
	for _, name := range offeringPages {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if m := retiredOffering.FindString(line); m != "" {
				t.Errorf("%s:%d names %q, which the VMI offering table does not carry: %s", name, i+1, m, strings.TrimSpace(line))
			}
		}
	}
}

func TestWikiOfferingPagesNameEveryBundle(t *testing.T) {
	dir := filepath.Join(repoRoot(t), ".github", "wiki")
	for _, name := range []string{"Licensing-Guide.md", "cmd-bundle.md"} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, want := range []string{"Task", "Chat", "Claw", "Family", "Sentry", "ClawDE"} {
			if !strings.Contains(string(raw), want) {
				t.Errorf("%s does not name the %s Bundle", name, want)
			}
		}
	}
}
