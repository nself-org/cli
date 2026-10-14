package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

func TestLicensedFragmentRejectsComposeOverrides(t *testing.T) {
	compattest.Set(t, true)
	for _, tag := range []string{"!reset", "!override", "!custom"} {
		for _, shape := range []struct {
			name, fragment string
		}{
			{"services", "services: " + tag + " {fixture: {image: fake}}\n"},
			{"service", "services:\n  fixture: " + tag + " {image: fake}\n"},
			{"environment-map", "services:\n  fixture:\n    environment: " + tag + " {}\n"},
			{"environment-list", "services:\n  fixture:\n    environment: " + tag + " []\n"},
			{"volumes", "services:\n  fixture:\n    volumes: " + tag + " []\n"},
		} {
			t.Run(tag+"/"+shape.name, func(t *testing.T) {
				dir, _ := entitlementFixture(t, "licensed")
				_, err := normalizeComposeEntitlement([]byte(shape.fragment), dir, "fixture")
				if detail := errs.Describe(err); detail == nil || detail.Code != "E125" || !strings.Contains(err.Error(), tag) {
					t.Fatalf("want named tag E125, got %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "fixture", pluginComposeFilename), []byte(shape.fragment), 0600); err != nil {
					t.Fatal(err)
				}
				// The full discovery pipeline must reject original tags before
				// earlier normalizers can drop the metadata.
				_, err = DiscoverPluginComposeFiles(t.TempDir(), dir)
				if detail := errs.Describe(err); detail == nil || detail.Code != "E125" || !strings.Contains(err.Error(), tag) {
					t.Fatalf("discovery: want named tag E125, got %v", err)
				}
			})
		}
	}
}

func TestLicensedFragmentRejectsIncludesAndExtraDocuments(t *testing.T) {
	compattest.Set(t, true)
	for _, fragment := range []string{
		"include: [other.yml]\nservices: {fixture: {image: fake}}\n",
		"services: {fixture: {image: fake}}\n---\nservices: {worker: {image: fake}}\n",
		"services: {fixture: {image: fake}}\n---\n",
	} {
		dir, _ := entitlementFixture(t, "licensed")
		_, err := normalizeComposeEntitlement([]byte(fragment), dir, "fixture")
		if detail := errs.Describe(err); detail == nil || detail.Code != "E125" {
			t.Fatalf("want E125, got %v", err)
		}
		path := filepath.Join(dir, "fixture", pluginComposeFilename)
		if err := os.WriteFile(path, []byte(fragment), 0600); err != nil {
			t.Fatal(err)
		}
		_, err = DiscoverPluginComposeFiles(t.TempDir(), dir)
		if detail := errs.Describe(err); detail == nil || detail.Code != "E125" {
			t.Fatalf("discovery silently discarded document: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != fragment {
			t.Fatalf("rejected fragment changed: %s %v", got, err)
		}
	}
}

func TestLicensedServiceCannotOverlapDiscoveredFragments(t *testing.T) {
	for _, tc := range []struct {
		name, first, second, fragment string
		disabled, v15, reject         bool
	}{
		{"licensed-first-free", "licensed", "free", "services: {shared: {environment: !reset {}}}\n", false, true, true},
		{"licensed-second-free", "free", "licensed", "services: {shared: {image: fake}}\n", false, true, true},
		{"both-licensed", "licensed", "licensed", "services: {shared: {image: fake}}\n", false, true, true},
		{"free-extra-document", "licensed", "free", "services: {other: {image: fake}}\n---\nservices: {shared: {environment: !reset {}}}\n", false, true, true},
		{"disabled", "licensed", "free", "services: {shared: {image: fake}}\n", true, true, false},
		{"both-free", "free", "free", "services: {shared: {image: fake}}\n", false, true, false},
		{"v14", "licensed", "free", "services: {shared: {image: fake}}\n", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, tc.v15)
			dir := t.TempDir()
			for i, name := range []string{"a", "z"} {
				axis, fragment := tc.first, "services: {shared: {image: fake}}\n"
				if i == 1 {
					axis, fragment = tc.second, tc.fragment
				}
				plugin := filepath.Join(dir, name)
				if err := os.Mkdir(plugin, 0700); err != nil {
					t.Fatal(err)
				}
				for file, content := range map[string]string{
					"plugin.json":         `{"name":"` + name + `","license":"` + axis + `"}`,
					pluginComposeFilename: fragment,
				} {
					if err := os.WriteFile(filepath.Join(plugin, file), []byte(content), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if i == 1 && tc.disabled {
					if err := os.WriteFile(filepath.Join(plugin, ".disabled"), nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			err = validateEntitlementFragments(dir, entries)
			if tc.reject {
				if detail := errs.Describe(err); detail == nil || detail.Code != "E125" || !strings.Contains(err.Error(), "shared") {
					t.Fatalf("want named collision E125: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
