package license

import (
	"os"
	"regexp"
	"testing"
)

// TestWikiAcceptedPrefixesMatchCode keeps the "Accepted prefixes" list in
// Plugin-Licensing.md identical to validProductPrefixes, so the wiki cannot
// drift from what ValidateKeyFormat accepts (D-1289).
func TestWikiAcceptedPrefixesMatchCode(t *testing.T) {
	data, err := os.ReadFile("../../.github/wiki/Plugin-Licensing.md")
	if err != nil {
		t.Fatalf("reading wiki page: %v", err)
	}
	re := regexp.MustCompile("(?m)^- `(nself_[a-z]+_)`, (.+)$")
	got := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		got[m[1]] = m[2]
	}
	if len(got) != len(validProductPrefixes) {
		t.Errorf("wiki lists %d prefixes, code defines %d", len(got), len(validProductPrefixes))
	}
	for _, pp := range validProductPrefixes {
		name, ok := got[pp.Prefix]
		if !ok {
			t.Errorf("wiki is missing prefix %s", pp.Prefix)
			continue
		}
		if name != pp.DisplayName {
			t.Errorf("wiki names %s %q, code says %q", pp.Prefix, name, pp.DisplayName)
		}
	}
}
