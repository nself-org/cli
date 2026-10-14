package build

// Purpose: mount ping's entitlement file in licensed plugin services.
// Inputs: installed manifest and compose bytes. Outputs: rewritten fragment.
// Constraints: v1.4 bytes are preserved; release builds never honor key overrides.
// SPORT: contract:plugin.entitlement-token.

import (
	"bytes"
	"path"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license/entitlement"
	"gopkg.in/yaml.v3"
)

const entitlementMount = "./.nself/state/entitlement.jwt:/run/nself/entitlement.jwt:ro"
const entitlementContainerFile = "/run/nself/entitlement.jwt"

// skipEntitlementMount is replaced only by the development build-tag file.
var skipEntitlementMount = func() bool { return false }

// licensedPlugin reads only the manifest licence axis, never a plugin name.
// Inputs: installed directory/name. Outputs: licence decision; malformed data is refused.
func licensedPlugin(pluginDir, name string) (bool, error) {
	return entitlement.IsLicensedPlugin(filepath.Join(pluginDir, name))
}

// normalizeComposeEntitlement enforces the fixed env and ro mount on every service.
// Inputs: fragment and licence classification. Outputs: fragment or fail-closed error.
// Constraints: a malformed licensed fragment must never silently omit its guard.
func normalizeComposeEntitlement(content []byte, pluginDir, name string) ([]byte, error) {
	// compat.V15(P7-PLUG-24): plugin compose has no entitlement mount -> licensed services get the signed JWT file
	if !compat.V15() {
		return content, nil
	}
	licensed, err := licensedPlugin(pluginDir, name)
	if err != nil {
		return nil, err
	}
	if !licensed || skipEntitlementMount() {
		return content, nil
	}
	doc, err := licensedComposeDocument(content, filepath.Join(pluginDir, name))
	if err != nil {
		return nil, err
	}
	services := yamlGet(doc.Content[0], "services")
	if services == nil || services.Kind != yaml.MappingNode {
		return nil, errs.New("E125", "licensed compose services must be a mapping")
	}
	if err := injectEntitlementServices(services); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, errs.Wrap("E125", "cannot encode licensed compose fragment", err)
	}
	if err := enc.Close(); err != nil {
		return nil, errs.Wrap("E125", "cannot finish licensed compose fragment", err)
	}
	return out.Bytes(), nil
}

// injectEntitlementServices enforces entitlement on each service in a licensed fragment.
// Inputs: services mapping. Outputs: guarded services or an error.
// Constraints: duplicate keys must not bypass fixed environment or mounts.
// SPORT: contract:plugin.entitlement-token.
func injectEntitlementServices(services *yaml.Node) error {
	if err := rejectEntitlementTag(services, "services"); err != nil {
		return err
	}
	for i := 1; i < len(services.Content); i += 2 {
		svc := services.Content[i]
		if err := rejectEntitlementTag(svc, "service"); err != nil {
			return err
		}
		if svc.Kind != yaml.MappingNode {
			return errs.New("E125", "licensed service must be a mapping")
		}
		seen := map[string]bool{}
		for j := 0; j < len(svc.Content); j += 2 {
			key := svc.Content[j].Value
			if key == "<<" || key == "extends" {
				return errs.New("E125", "licensed service cannot use "+key+"; declare environment and volumes directly")
			}
			if seen[key] {
				return errs.New("E125", "duplicate licensed service key")
			}
			seen[key] = true
			if key == "environment" || key == "volumes" {
				if err := rejectEntitlementTag(svc.Content[j+1], key); err != nil {
					return err
				}
			}
		}
		if err := injectEntitlementEnv(svc); err != nil {
			return err
		}
		if err := injectEntitlementVolume(svc); err != nil {
			return err
		}
	}
	return nil
}

// rejectEntitlementTag prevents Compose reset/override tags from dropping the guard.
// Inputs: guarded YAML node. Outputs: E125 for any non-core tag.
func rejectEntitlementTag(node *yaml.Node, field string) error {
	if !strings.HasPrefix(node.Tag, "!!") {
		return errs.Newf("E125", "licensed %s cannot use YAML tag %q", field, node.Tag)
	}
	return nil
}

// injectEntitlementVolume replaces conflicting target mounts, keeping unrelated volumes.
// Inputs: service mapping. Outputs: fixed ro binding. Constraints: aliases fail closed.
func injectEntitlementVolume(svc *yaml.Node) error {
	volumes := yamlGet(svc, "volumes")
	if volumes == nil {
		volumes = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		svc.Content = append(svc.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "volumes"}, volumes)
	}
	if volumes.Kind != yaml.SequenceNode {
		return errs.New("E125", "licensed volumes must be a list")
	}
	kept := volumes.Content[:0]
	for _, v := range volumes.Content {
		if v.Kind == yaml.ScalarNode {
			parts := bytes.Split([]byte(v.Value), []byte(":"))
			// Read the target from the right so source interpolation may contain colons.
			target := parts[len(parts)-1]
			if len(parts) >= 2 && !bytes.HasPrefix(target, []byte("/")) {
				target = parts[len(parts)-2]
			}
			if path.Clean(string(target)) == entitlementContainerFile {
				continue
			}
		} else if v.Kind == yaml.MappingNode {
			if target := yamlGet(v, "target"); target != nil {
				if target.Kind != yaml.ScalarNode {
					return errs.New("E125", "licensed volume target must be a literal path")
				}
				if path.Clean(target.Value) == entitlementContainerFile {
					continue
				}
			}
		} else {
			return errs.New("E125", "unsupported licensed volume shape")
		}
		kept = append(kept, v)
	}
	volumes.Content = append(kept, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entitlementMount})
	return nil
}
