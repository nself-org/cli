package database

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// MetadataDrift describes the difference between live Hasura metadata and committed files.
type MetadataDrift struct {
	// TablesAdded are tracked tables in live Hasura not in committed metadata.
	TablesAdded []string
	// TablesRemoved are tables in committed metadata not in live Hasura.
	TablesRemoved []string
	// PermissionsChanged are tables where permissions differ.
	PermissionsChanged []string
	// RelationshipsChanged are tables where relationships differ.
	RelationshipsChanged []string
	// IsClean is true when live matches committed exactly.
	IsClean bool
}

// HasuraGitStatus describes the git state of metadata files.
type HasuraGitStatus struct {
	Branch       string
	CommitHash   string
	LastExportAt time.Time
	Modified     []string // metadata files with uncommitted changes
	IsClean      bool
}

// metadataTableEntry holds the table name and schema extracted from live metadata.
type metadataTableEntry struct {
	Schema string
	Name   string
}

// extractTablesFromMetadata parses a raw Hasura export_metadata JSON response
// and returns the list of tracked tables across all sources.
func extractTablesFromMetadata(data []byte) ([]metadataTableEntry, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse metadata JSON: %w", err)
	}

	sources, ok := raw["sources"]
	if !ok {
		return nil, nil
	}
	sourceList, ok := sources.([]interface{})
	if !ok {
		return nil, nil
	}

	var entries []metadataTableEntry
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
			if name != "" {
				entries = append(entries, metadataTableEntry{Schema: schema, Name: name})
			}
		}
	}
	return entries, nil
}

// extractTableNamesFromYAMLDir scans the on-disk YAML table files under
// {tablesDir} and returns the table names encoded in each filename.
// Hasura CLI stores tables as {schema}_{name}.yaml files.
func extractTableNamesFromYAMLDir(tablesDir string) ([]string, error) {
	entries, err := os.ReadDir(tablesDir)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP. Hasura only creates the tables/ directory once metadata is
			// exported, so absent == no tables tracked on disk. ENOENT only:
			// this list is diffed against live metadata, and an unreadable
			// directory read as "zero tables" would make every live table look
			// newly added.
			return nil, nil
		}
		return nil, fmt.Errorf("read tables dir %s: %w", tablesDir, err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		// Strip .yaml suffix; the remainder is used as the table identifier.
		names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	return names, nil
}

// DetectMetadataDrift compares live Hasura metadata against committed files in hasura/metadata/.
// It exports current metadata via the Hasura API and diffs against the on-disk files.
func DetectMetadataDrift(ctx context.Context, cfg *config.Config, projectDir string) (MetadataDrift, error) {
	liveData, err := postMetadata(ctx, cfg, metadataRequest{
		Type: "export_metadata",
		Args: map[string]interface{}{},
	})
	if err != nil {
		return MetadataDrift{}, fmt.Errorf("export live metadata: %w", err)
	}

	liveTables, err := extractTablesFromMetadata(liveData)
	if err != nil {
		return MetadataDrift{}, fmt.Errorf("parse live metadata tables: %w", err)
	}

	tablesDir := filepath.Join(projectDir, "hasura", "metadata", "databases", "default", "tables")
	diskNames, err := extractTableNamesFromYAMLDir(tablesDir)
	if err != nil {
		return MetadataDrift{}, err
	}

	// Build lookup sets.
	liveSet := make(map[string]bool, len(liveTables))
	for _, t := range liveTables {
		key := t.Schema + "_" + t.Name
		liveSet[key] = true
	}

	diskSet := make(map[string]bool, len(diskNames))
	for _, n := range diskNames {
		diskSet[n] = true
	}

	var added, removed []string
	for key := range liveSet {
		if !diskSet[key] {
			added = append(added, key)
		}
	}
	for key := range diskSet {
		if !liveSet[key] {
			removed = append(removed, key)
		}
	}

	drift := MetadataDrift{
		TablesAdded:   added,
		TablesRemoved: removed,
		IsClean:       len(added) == 0 && len(removed) == 0,
	}
	return drift, nil
}
