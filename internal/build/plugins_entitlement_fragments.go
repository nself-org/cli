package build

// Purpose: reject Compose shapes that can remove a licensed service's guard.
// Inputs: installed fragments. Outputs: coded validation errors before rewriting.
// Constraints: v1.4 fragments remain untouched; free fragments may share names.
// SPORT: contract:plugin.entitlement-token.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"gopkg.in/yaml.v3"
)

// licensedComposeDocument validates the original bytes before normalizers can
// discard extra documents or Compose tags. It returns one guarded document.
func licensedComposeDocument(content []byte, fragment string) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var doc, extra yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return nil, errs.Wrap("E125", "invalid licensed compose fragment in "+fragment, err)
	}
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errs.New("E125", "licensed compose fragment must contain one YAML document: "+fragment)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errs.New("E125", "invalid licensed compose fragment in "+fragment)
	}
	root := doc.Content[0]
	if yamlGet(root, "include") != nil {
		return nil, errs.New("E125", "licensed compose fragment cannot use top-level include: "+fragment)
	}
	services := yamlGet(root, "services")
	if services == nil || services.Kind != yaml.MappingNode {
		return nil, errs.New("E125", "licensed compose services must be a mapping")
	}
	if err := injectEntitlementServices(services); err != nil {
		return nil, err
	}
	return &doc, nil
}

type entitlementServiceOwner struct {
	fragment string
	licensed bool
}

// validateEntitlementFragments checks all enabled fragments before any rewrite.
// A free fragment cannot redefine a licensed service and remove its guard.
func validateEntitlementFragments(pluginDir string, entries []os.DirEntry) error {
	// compat.V15(P7-PLUG-24): overlapping fragments merge freely -> licensed service collisions fail closed
	if !compat.V15() || skipEntitlementMount() {
		return nil
	}
	owners := map[string]entitlementServiceOwner{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(pluginDir, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, ".disabled")); err == nil {
			continue
		}
		fragment := filepath.Join(dir, pluginComposeFilename)
		content, err := os.ReadFile(fragment)
		if err != nil {
			continue // Discovery owns missing-fragment diagnostics.
		}
		licensed, err := licensedPlugin(pluginDir, entry.Name())
		if err != nil {
			return err
		}
		if licensed {
			if _, err := licensedComposeDocument(content, fragment); err != nil {
				return err
			}
		}
		if err := registerEntitlementServices(content, entitlementServiceOwner{fragment, licensed}, owners); err != nil {
			return err
		}
	}
	return nil
}

// registerEntitlementServices compares names across every YAML document in free
// fragments as well as the single allowed document in licensed fragments.
func registerEntitlementServices(content []byte, current entitlementServiceOwner, owners map[string]entitlementServiceOwner) error {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	for {
		var doc yaml.Node
		if decoder.Decode(&doc) != nil {
			return nil // Free-fragment syntax remains owned by Compose.
		}
		if len(doc.Content) == 0 {
			continue
		}
		services := yamlGet(doc.Content[0], "services")
		if services == nil || services.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i < len(services.Content); i += 2 {
			name := services.Content[i].Value
			previous, exists := owners[name]
			if exists && previous.fragment != current.fragment && (previous.licensed || current.licensed) {
				return errs.Newf("E125", "licensed service %q appears in multiple fragments: %s and %s", name, previous.fragment, current.fragment)
			}
			owners[name] = current
		}
	}
}
