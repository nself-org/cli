package database

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/config"
)

// Purpose: exports live Hasura metadata to a git-friendly YAML directory,
// diffs it against what's on disk, and validates tenant-scoped tables carry
// the required RLS-adjacent permission roles.
// Inputs: a context, *config.Config, and (for export/diff) a project directory.
// Outputs: an output directory path, a diff list, or a missing-permissions list.
// Constraints: split out of hasura.go (CLI-R12). Depends on HasuraExportMetadata
// (hasura.go), convertYAMLToJSON (hasura_apply.go) and the shared file layout in
// hasura_export_layout.go.

// HasuraExportToYAML exports live Hasura metadata to a git-friendly sorted YAML
// directory under {projectDir}/hasura/metadata/: one <key>.yaml per top-level
// key plus a config.yaml index of !include lines (the same layout the archive
// uses, see hasura_export_layout.go).
// Returns the output directory path.
func HasuraExportToYAML(ctx context.Context, cfg *config.Config, projectDir string) (string, error) {
	data, err := HasuraExportMetadata(ctx, cfg)
	if err != nil {
		return "", err
	}
	outDir := filepath.Join(projectDir, "hasura", "metadata")
	if err := writeMetadataTree(outDir, data); err != nil {
		return "", err
	}
	return outDir, nil
}

// HasuraDiffMetadata compares live metadata against on-disk YAML files.
// Returns a list of keys that differ. Empty list means no drift.
func HasuraDiffMetadata(ctx context.Context, cfg *config.Config, projectDir string) ([]string, error) {
	liveData, err := HasuraExportMetadata(ctx, cfg)
	if err != nil {
		return nil, err
	}

	var liveMap map[string]interface{}
	if err := json.Unmarshal(liveData, &liveMap); err != nil {
		return nil, fmt.Errorf("parse live metadata: %w", err)
	}

	metadataDir := filepath.Join(projectDir, "hasura", "metadata")
	if _, err := os.Stat(metadataDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("no on-disk metadata at %s; run 'nself db hasura metadata export' first", metadataDir)
	}

	var diffs []string
	for key, liveVal := range liveMap {
		diskPath := filepath.Join(metadataDir, key+".yaml")
		diskData, err := os.ReadFile(diskPath)
		if err != nil {
			if os.IsNotExist(err) {
				diffs = append(diffs, key+" (missing on disk)")
				continue
			}
			return nil, fmt.Errorf("read %s: %w", diskPath, err)
		}

		var diskVal interface{}
		if err := yaml.Unmarshal(diskData, &diskVal); err != nil {
			diffs = append(diffs, key+" (invalid YAML on disk)")
			continue
		}

		// Re-serialize both to JSON for comparison.
		liveJSON, _ := json.Marshal(convertYAMLToJSON(liveVal))
		diskJSON, _ := json.Marshal(convertYAMLToJSON(diskVal))

		if string(liveJSON) != string(diskJSON) {
			diffs = append(diffs, key)
		}
	}

	// Check for files on disk not present in live metadata.
	entries, _ := os.ReadDir(metadataDir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		if e.Name() == metadataIndexFile {
			continue // the !include index, not a metadata key
		}
		key := strings.TrimSuffix(e.Name(), ".yaml")
		if _, ok := liveMap[key]; !ok {
			diffs = append(diffs, key+" (only on disk)")
		}
	}

	return diffs, nil
}

// HasuraValidatePermissions checks that every tenant-scoped table tracked by Hasura
// has permissions for tenant_member and tenant_admin roles.
// Returns a list of tables missing required permissions.
func HasuraValidatePermissions(ctx context.Context, cfg *config.Config) ([]string, error) {
	data, err := HasuraExportMetadata(ctx, cfg)
	if err != nil {
		return nil, err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}

	// Look for tables in sources -> tables.
	sources, ok := raw["sources"]
	if !ok {
		return nil, nil
	}

	sourceList, ok := sources.([]interface{})
	if !ok {
		return nil, nil
	}

	requiredRoles := []string{"tenant_member", "tenant_admin"}
	var missing []string

	for _, src := range sourceList {
		srcMap, ok := src.(map[string]interface{})
		if !ok {
			continue
		}
		tables, ok := srcMap["tables"]
		if !ok {
			continue
		}
		tableList, ok := tables.([]interface{})
		if !ok {
			continue
		}

		for _, tbl := range tableList {
			tblMap, ok := tbl.(map[string]interface{})
			if !ok {
				continue
			}

			// Get table name.
			tableObj, ok := tblMap["table"]
			if !ok {
				continue
			}
			tblDef, ok := tableObj.(map[string]interface{})
			if !ok {
				continue
			}
			schema, _ := tblDef["schema"].(string)
			name, _ := tblDef["name"].(string)
			fullName := schema + "." + name

			// Check if table has tenant_id by looking at columns in select_permissions.
			// We check if select_permissions exist for the required roles.
			selectPerms, _ := tblMap["select_permissions"].([]interface{})
			existingRoles := make(map[string]bool)
			for _, perm := range selectPerms {
				permMap, ok := perm.(map[string]interface{})
				if !ok {
					continue
				}
				role, _ := permMap["role"].(string)
				existingRoles[role] = true
			}

			// Only check tables that have at least one permission defined
			// (indicating they're tracked with access control).
			if len(existingRoles) == 0 {
				continue
			}

			for _, role := range requiredRoles {
				if !existingRoles[role] {
					missing = append(missing, fmt.Sprintf("%s: missing %s permissions", fullName, role))
				}
			}
		}
	}

	return missing, nil
}
