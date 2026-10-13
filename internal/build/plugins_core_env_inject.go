package build

// Purpose: the block-shape scanner and per-shape appenders
// normalizeComposePluginCoreEnv (plugins_core_env.go) dispatches to. Split
// out to keep plugins_core_env.go under the 300-line file cap once list-form
// support was added alongside the pre-existing map-form path.
// Inputs: a compose fragment's bytes plus a pluginEnvBlockRE match location.
// Outputs: the block's shape/existing-key set, or the fragment bytes with
// missing pluginCoreEnvKeys appended/inserted.
// Constraints: every appender is byte-splice-only (no YAML re-encoding) so a
// fragment's existing formatting, comments and quoting survive untouched —
// same constraint as the pre-existing map-form-only implementation.

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"gopkg.in/yaml.v3"
)

// envBlockShape classifies the child lines of an environment: block that
// scanEnvBlock walks.
type envBlockShape int

const (
	envBlockMap envBlockShape = iota
	envBlockList
	envBlockUnrecognised
)

// scanEnvBlock walks the child lines of an environment: block (loc is the
// pluginEnvBlockRE match) and classifies its shape, collects the keys
// already declared, and returns the byte offset immediately after the last
// child line (blockEnd) so appendMapEntries/appendListEntries know where to
// splice new entries in. Blank lines and "# ..." comments are tolerated in
// either shape (the real installed plugins/free/cron fragment interleaves a
// four-line comment among its list entries). A YAML merge key ("<<: *x") as
// a direct child makes the shape envBlockUnrecognised: its expanded keys
// aren't visible to this text-level scan, so appending after it risks a
// silent duplicate/shadow rather than the "fragment wins" guarantee.
func scanEnvBlock(content []byte, loc []int) (shape envBlockShape, blockEnd int, entryIndent string, existing map[string]bool) {
	envIndent := string(content[loc[2]:loc[3]])
	entryIndent = envIndent + "  "
	entryIndentB := []byte(entryIndent)

	existing = map[string]bool{}
	pos := loc[1]
	blockEnd = pos
	shape = envBlockMap // default until a list item proves otherwise

	for _, line := range bytes.SplitAfter(content[pos:], []byte("\n")) {
		if len(line) == 0 || !bytes.HasPrefix(line, entryIndentB) {
			break
		}
		trimmed := bytes.TrimSpace(line)
		switch {
		case len(trimmed) == 0, trimmed[0] == '#':
			// Blank line or comment: consumed, no classification signal.
		case bytes.HasPrefix(trimmed, []byte("<<:")):
			return envBlockUnrecognised, blockEnd, entryIndent, nil
		case trimmed[0] == '-':
			shape = envBlockList
			if m := pluginEnvListEntryKeyRE.FindSubmatch(trimmed); m != nil {
				existing[string(m[1])] = true
			}
		default:
			if m := pluginEnvEntryKeyRE.FindSubmatch(line); m != nil {
				existing[string(m[1])] = true
			}
		}
		blockEnd += len(line)
	}
	return shape, blockEnd, entryIndent, existing
}

// appendMapEntries renders the missing pluginCoreEnvKeys as map-form
// "KEY: value" lines and splices them in at blockEnd (see scanEnvBlock).
func appendMapEntries(content []byte, blockEnd int, entryIndent string, existing map[string]bool, pluginName string, port int) []byte {
	var toAdd bytes.Buffer
	for _, key := range pluginCoreEnvKeys {
		if existing[key] {
			continue
		}
		if val, ok := pluginCoreEnvValue(key, pluginName, port); ok {
			fmt.Fprintf(&toAdd, "%s%s: %s\n", entryIndent, key, val)
		}
	}
	return spliceAt(content, blockEnd, toAdd.Bytes())
}

// appendListEntries is appendMapEntries' list-form counterpart, rendering
// "- KEY=value" lines — the shape 40+ installed plugin fragments use (see
// plugins_core_env.go header).
func appendListEntries(content []byte, blockEnd int, entryIndent string, existing map[string]bool, pluginName string, port int) []byte {
	var toAdd bytes.Buffer
	for _, key := range pluginCoreEnvKeys {
		if existing[key] {
			continue
		}
		if line, ok := pluginCoreEnvListLine(key, pluginName, port); ok {
			fmt.Fprintf(&toAdd, "%s- %s\n", entryIndent, line)
		}
	}
	return spliceAt(content, blockEnd, toAdd.Bytes())
}

// injectIntoEmptyListEnvBlock rewrites an inline "environment: []" into a
// multi-line list-form block containing every pluginCoreEnvKeys entry. loc
// is the pluginEnvEmptyListRE match, spanning the whole "  environment: []"
// line including its trailing newline.
func injectIntoEmptyListEnvBlock(content []byte, loc []int, pluginName string, port int) []byte {
	envIndent := string(content[loc[2]:loc[3]])
	entryIndent := envIndent + "  "

	var block bytes.Buffer
	fmt.Fprintf(&block, "%senvironment:\n", envIndent)
	for _, key := range pluginCoreEnvKeys {
		if line, ok := pluginCoreEnvListLine(key, pluginName, port); ok {
			fmt.Fprintf(&block, "%s- %s\n", entryIndent, line)
		}
	}

	var out bytes.Buffer
	out.Grow(len(content) - (loc[1] - loc[0]) + block.Len())
	out.Write(content[:loc[0]])
	out.Write(block.Bytes())
	out.Write(content[loc[1]:])
	return out.Bytes()
}

// injectNewEnvBlock inserts a brand-new map-form environment: block,
// anchored immediately before the service's short-form "networks:" list
// (see shortNetworkListRE in plugins_network_alias.go), for a fragment that
// has no environment: block of its own yet. ok is false when no such anchor
// exists — the caller must warn rather than silently drop the injection.
func injectNewEnvBlock(content []byte, pluginName string, port int) (out []byte, ok bool) {
	match := shortNetworkListRE.FindSubmatchIndex(content)
	if match == nil {
		return content, false // no anchor to insert at
	}
	svcIndent := string(content[match[2]:match[3]])
	entryIndent := svcIndent + "  "

	var block bytes.Buffer
	fmt.Fprintf(&block, "%senvironment:\n", svcIndent)
	for _, key := range pluginCoreEnvKeys {
		if val, ok := pluginCoreEnvValue(key, pluginName, port); ok {
			fmt.Fprintf(&block, "%s%s: %s\n", entryIndent, key, val)
		}
	}

	return spliceAt(content, match[0], block.Bytes()), true
}

// spliceAt inserts add at byte offset at within content, or returns content
// unchanged if add is empty (nothing left to inject).
func spliceAt(content []byte, at int, add []byte) []byte {
	if len(add) == 0 {
		return content
	}
	var out bytes.Buffer
	out.Grow(len(content) + len(add))
	out.Write(content[:at])
	out.Write(add)
	out.Write(content[at:])
	return out.Bytes()
}

// injectEntitlementEnv enforces the container path in mapping or list environments.
// Inputs: licensed service mapping. Outputs: fixed path; unsupported shapes fail.
// Constraints: a fragment cannot override the guard into the absence rule.
// SPORT: contract:plugin.entitlement-token.
func injectEntitlementEnv(svc *yaml.Node) error {
	env := yamlGet(svc, "environment")
	if env == nil {
		env = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		svc.Content = append(svc.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "environment"}, env)
	}
	const key = "NSELF_ENTITLEMENT_FILE"
	switch env.Kind {
	case yaml.MappingNode:
		kept := env.Content[:0]
		for i := 0; i < len(env.Content); i += 2 {
			if env.Content[i].Value == key {
				continue
			}
			if env.Content[i].Value == "<<" {
				return errs.New("E125", "licensed environment merge is unsupported")
			}
			kept = append(kept, env.Content[i:i+2]...)
		}
		env.Content = append(kept, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entitlementContainerFile})
	case yaml.SequenceNode:
		kept := env.Content[:0]
		for _, item := range env.Content {
			if item.Kind != yaml.ScalarNode {
				return errs.New("E125", "licensed environment list must contain strings")
			}
			if item.Value != key && !strings.HasPrefix(item.Value, key+"=") {
				kept = append(kept, item)
			}
		}
		env.Content = append(kept, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key + "=" + entitlementContainerFile})
	default:
		return errs.New("E125", "licensed environment must be a map or list")
	}
	return nil
}

// normalizeServiceCoreEnv applies the core-env rewrite to bytes that start at
// a service's direct environment/networks key line (anchored true), or to a
// whole fragment when no service range could be found (anchored false). When
// anchored, every match the rewrite would act on must begin at offset 0: a
// match further in belongs to a nested key (or the direct key has a shape no
// regex matches), so the fragment is left unchanged with a warning.
func normalizeServiceCoreEnv(content []byte, anchored bool, pluginDir, pluginName string, port int) []byte {
	atStart := func(loc []int) bool { return !anchored || (loc != nil && loc[0] == 0) }
	refuse := func() []byte {
		warnUnrecognisedEnvBlock(pluginDir, pluginName)
		return content
	}
	if loc := pluginEnvAnchoredRE.FindIndex(content); loc != nil && atStart(loc) {
		return refuse()
	}
	if loc := pluginEnvEmptyListRE.FindSubmatchIndex(content); loc != nil {
		if !atStart(loc) {
			return refuse()
		}
		return injectIntoEmptyListEnvBlock(content, loc, pluginName, port)
	}
	if loc := pluginEnvBlockRE.FindSubmatchIndex(content); loc != nil {
		if !atStart(loc) {
			return refuse()
		}
		shape, blockEnd, entryIndent, existing := scanEnvBlock(content, loc)
		switch shape {
		case envBlockList:
			return appendListEntries(content, blockEnd, entryIndent, existing, pluginName, port)
		case envBlockUnrecognised:
			return refuse()
		default: // envBlockMap
			return appendMapEntries(content, blockEnd, entryIndent, existing, pluginName, port)
		}
	}
	if !atStart(shortNetworkListRE.FindIndex(content)) {
		return refuse()
	}
	if out, ok := injectNewEnvBlock(content, pluginName, port); ok {
		return out
	}
	return refuse()
}
