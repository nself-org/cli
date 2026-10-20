package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/license"
)

// TestBundleInfoCacheLabels keeps cached claims distinct from an active licence
// decision: bundle info displays local bytes without checking their trust.
func TestBundleInfoCacheLabels(t *testing.T) {
	chat, ok := bundle.Get("chat")
	if !ok || len(chat.Plugins) == 0 {
		t.Fatal("chat bundle must declare a plugin")
	}
	for _, tc := range []struct {
		name  string
		slug  string
		entry *license.CacheEntry
		want  string
	}{
		{"free", "task", nil, "active (free)"},
		{"missing", "chat", nil, "not activated"},
		{"plus", "chat", &license.CacheEntry{Tier: "plus"}, "cached entitlement (unverified;"},
		{"enterprise", "chat", &license.CacheEntry{Tier: "enterprise"}, "cached entitlement (unverified;"},
		{"owner", "chat", &license.CacheEntry{Tier: "owner"}, "cached entitlement (unverified;"},
		{"included", "chat", &license.CacheEntry{Tier: "free", PluginsAllowed: []string{chat.Plugins[0]}}, "cached entitlement (unverified;"},
		{"excluded", "chat", &license.CacheEntry{Tier: "free"}, "not included in cached license (unverified;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "license.json"))
			if tc.entry != nil {
				if err := license.WriteCache(tc.entry); err != nil {
					t.Fatal(err)
				}
			}
			got := resolveBundleLicenseStatus(tc.slug)
			if !strings.HasPrefix(got, tc.want) {
				t.Errorf("status = %q, want prefix %q", got, tc.want)
			}
		})
	}
}
