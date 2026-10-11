package database

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/config"
)

// Purpose: applies Hasura metadata from disk, supporting both the Hasura CLI
// YAML directory format (with !include directives) and the legacy nSelf JSON
// format, plus the shared YAML->JSON conversion helper used by the export and
// diff paths in hasura_export.go.
// Inputs: a context, *config.Config, and a project/metadata directory path.
// Outputs: an error, or (for convertYAMLToJSON) a JSON-safe value tree.
// Constraints: split out of hasura.go (CLI-R12) as a pure move; no behavior
// changed. Depends on postMetadata/metadataRequest in hasura.go.

// HasuraApplyMetadata applies Hasura metadata from projectDir.
//
// It supports two metadata formats:
//
//  1. YAML directory format (standard Hasura CLI): projectDir/hasura/metadata/
//     Tables use !include directives referencing individual YAML files.
//     This format is detected first. For a real Hasura CLI project
//     (hasura/config.yaml naming metadata_directory) with the hasura CLI binary
//     in PATH, that binary applies it. Otherwise, including nSelf's own
//     config.yaml index layout, nSelf resolves !include directives in Go and
//     sends the result to the metadata API.
//
//  2. JSON format (legacy nSelf): projectDir/hasura/metadata.json
//     Sent directly to the replace_metadata API.
//
// If neither exists, reload_metadata is called (safe/idempotent).
func HasuraApplyMetadata(ctx context.Context, cfg *config.Config, projectDir string) error {
	hasuraDir := filepath.Join(projectDir, "hasura")
	metadataDir := filepath.Join(hasuraDir, "metadata")
	metadataFile := filepath.Join(hasuraDir, "metadata.json")

	// --- Format 1: YAML directory with !include directives ---
	if info, err := os.Stat(metadataDir); err == nil && info.IsDir() {
		return applyYAMLMetadata(ctx, cfg, hasuraDir, metadataDir)
	}

	// --- Format 2: legacy JSON file ---
	data, err := os.ReadFile(metadataFile)
	if err != nil {
		if os.IsNotExist(err) {
			_, reloadErr := postMetadata(ctx, cfg, metadataRequest{
				Type: "reload_metadata",
				Args: struct{}{},
			})
			if reloadErr != nil {
				return fmt.Errorf("reload metadata (no metadata found): %w", reloadErr)
			}
			return nil
		}
		return fmt.Errorf("read metadata file: %w", err)
	}

	var metadata json.RawMessage
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("parse metadata file: %w", err)
	}
	_, err = postMetadata(ctx, cfg, metadataRequest{
		Type: "replace_metadata",
		Args: metadata,
	})
	return err
}

// applyYAMLMetadata handles a YAML metadata directory. nself's own layout
// (config.yaml index at the metadata root) always goes through the Go
// !include resolver; the hasura CLI binary, when on PATH, is used only for a
// real Hasura CLI project (isHasuraCLIProject, D-0564), since it cannot read
// nself's index.
func applyYAMLMetadata(ctx context.Context, cfg *config.Config, hasuraDir, metadataDir string) error {
	if isHasuraCLIProject(hasuraDir, metadataDir) {
		if hasuraBin, err := exec.LookPath("hasura"); err == nil {
			return applyViaHasuraCLI(ctx, cfg, hasuraBin, hasuraDir)
		}
	}
	return applyViaIncludeResolver(ctx, cfg, metadataDir)
}

// applyViaHasuraCLI shells out to the hasura CLI binary. The binary handles
// !include resolution and all YAML features natively.
func applyViaHasuraCLI(ctx context.Context, cfg *config.Config, hasuraBin, hasuraDir string) error {
	cmd := hasuraMetadataApplyCmd(ctx, cfg, hasuraBin, hasuraDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("hasura metadata apply: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// hasuraMetadataApplyCmd constructs the hasura-cli metadata apply command.
//
// The admin secret is injected into the child process environment via cmd.Env
// (not visible in the host process table) and consumed by hasura-cli via the
// HASURA_GRAPHQL_ADMIN_SECRET environment variable, which it reads natively.
// No --admin-secret argv element is used, preventing CWE-214 process table
// exposure.
func hasuraMetadataApplyCmd(ctx context.Context, cfg *config.Config, hasuraBin, hasuraDir string) *exec.Cmd {
	port := cfg.Hasura.Port
	if port == 0 {
		port = 8080
	}
	endpoint := fmt.Sprintf("http://localhost:%d", port)

	cmd := exec.CommandContext(ctx, hasuraBin,
		"metadata", "apply",
		"--endpoint", endpoint,
		"--project", hasuraDir,
	)
	cmd.Env = append(os.Environ(), "HASURA_GRAPHQL_ADMIN_SECRET="+cfg.Hasura.AdminSecret)
	return cmd
}

// applyViaIncludeResolver resolves !include directives in Go, assembles the
// full metadata object, and sends it to the Hasura replace_metadata API.
//
// The Hasura CLI YAML metadata format uses a config.yaml at the metadata root
// that !include-s sub-files (tables.yaml, actions.yaml, etc.). Each of those
// can in turn !include individual table/action YAML files.
//
// gopkg.in/yaml.v3 does not natively handle !include. We implement a two-pass
// approach: raw bytes are preprocessed to inline !include references before
// being decoded as YAML, then the result is re-encoded as JSON for the API.
func applyViaIncludeResolver(ctx context.Context, cfg *config.Config, metadataDir string) error {
	configFile := filepath.Join(metadataDir, "config.yaml")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// Some projects use tables.yaml directly at the metadata root.
		configFile = filepath.Join(metadataDir, "tables.yaml")
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			return fmt.Errorf("no config.yaml or tables.yaml found in %s", metadataDir)
		}
	}

	resolved, err := resolveIncludes(metadataDir, configFile)
	if err != nil {
		return fmt.Errorf("resolve !include directives: %w", err)
	}

	// Decode the resolved YAML into a generic structure, then re-encode as JSON.
	var doc interface{}
	if err := yaml.Unmarshal(resolved, &doc); err != nil {
		return fmt.Errorf("parse resolved metadata YAML: %w", err)
	}
	if err := validateNoListWrappedEntries(doc); err != nil {
		return err
	}
	jsonBytes, err := json.Marshal(convertYAMLToJSON(doc))
	if err != nil {
		return fmt.Errorf("encode metadata as JSON: %w", err)
	}

	_, err = postMetadata(ctx, cfg, metadataRequest{
		Type: "replace_metadata",
		Args: json.RawMessage(jsonBytes),
	})
	return err
}

// resolveIncludes reads a YAML file and recursively inlines every
// `!include <filename>` tag, returning the fully expanded bytes.
func resolveIncludes(baseDir, filePath string) ([]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filePath, err)
	}

	lines := strings.Split(string(data), "\n")
	var out strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Match:  - !include foo.yaml  or  key: !include foo.yaml
		if idx := strings.Index(trimmed, "!include "); idx != -1 {
			includedFile := strings.TrimSpace(trimmed[idx+len("!include "):])
			includedPath := filepath.Join(baseDir, includedFile)
			includedData, err := resolveIncludes(filepath.Dir(includedPath), includedPath)
			if err != nil {
				return nil, err
			}
			// Preserve the leading indent from the original line, but replace
			// the !include token with the inlined content (indented to match).
			leadingSpaces := len(line) - len(strings.TrimLeft(line, " \t"))
			baseIndent := line[:leadingSpaces]
			includedLines := strings.Split(strings.TrimRight(string(includedData), "\n"), "\n")

			switch {
			case strings.HasPrefix(trimmed, "- !include"):
				// List item ("- !include foo.yaml"): "- " prefix on the first
				// inlined line, remaining lines indented one level under it.
				itemIndent := baseIndent + "  "
				for i, il := range includedLines {
					if i == 0 {
						out.WriteString(baseIndent + "- " + il + "\n")
					} else {
						out.WriteString(itemIndent + il + "\n")
					}
				}
			case len(includedLines) == 1:
				// "key: !include foo.yaml" where foo.yaml is a one-line
				// scalar: inline it on the same line as the key.
				out.WriteString(baseIndent + trimmed[:idx] + includedLines[0] + "\n")
			default:
				// "key: !include foo.yaml" where foo.yaml is a multi-line
				// block (mapping/list): emit "key:" on its own line, then the
				// block indented one level under it — otherwise the included
				// block's own lines would land at the SAME indent as the key,
				// which is not valid YAML nesting (this was the bug behind
				// TestResolveIncludes_NestedConfigTree: "tables: !include
				// tables.yaml" previously dropped the "tables:" key entirely
				// and emitted the included list at column 0).
				keyPart := strings.TrimRight(trimmed[:idx], " ")
				blockIndent := baseIndent + "  "
				out.WriteString(baseIndent + keyPart + "\n")
				for _, il := range includedLines {
					out.WriteString(blockIndent + il + "\n")
				}
			}
		} else {
			out.WriteString(line + "\n")
		}
	}

	return []byte(out.String()), nil
}

// convertYAMLToJSON recursively converts a yaml.v3-decoded value (which uses
// map[string]interface{} for mappings and []interface{} for sequences) into a
// JSON-safe structure. yaml.v3 uses map[string]interface{} so this is mostly
// a pass-through, but map keys decoded as interface{} need string conversion.
func convertYAMLToJSON(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, mv := range val {
			out[k] = convertYAMLToJSON(mv)
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, mv := range val {
			out[fmt.Sprintf("%v", k)] = convertYAMLToJSON(mv)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, item := range val {
			out[i] = convertYAMLToJSON(item)
		}
		return out
	default:
		return val
	}
}
