package doctor

// hasura_meta_reader.go — PERM-HASURA-01: Hasura row-filter check for np_* tables.
//
// S1.T10 (sprint-01-multi-tenant-fix):
//   Walk hasura/metadata/ YAML files and verify that every tracked np_* table
//   has the correct row filter in its select_permissions for the "user" role:
//     - Tables with source_account_id column: filter must reference source_account_id
//     - Tables with tenant_id column: filter must reference tenant_id
//     - Tables with neither isolation column: exempt (no false positive)
//
// This check reads from disk only. No Hasura API calls, no DB connections.
// Security-Always-Free Doctrine: runs without a license.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"gopkg.in/yaml.v3"
)

const (
	// HasuraMetaCheckID is the canonical check ID referenced in docs and wiki.
	HasuraMetaCheckID = "PERM-HASURA-01"
)

// hasuraTableYAML is the minimal structure we need from a per-table YAML file.
// Hasura CLI v2+ writes one file per table under
// hasura/metadata/databases/default/tables/<schema>_<table>.yaml
// or a flat hasura/metadata/tables.yaml in older layouts.
type hasuraTableYAML struct {
	Table struct {
		Schema string `yaml:"schema"`
		Name   string `yaml:"name"`
	} `yaml:"table"`
	SelectPermissions []struct {
		Role       string `yaml:"role"`
		Permission struct {
			Filter map[string]interface{} `yaml:"filter"`
		} `yaml:"permission"`
	} `yaml:"select_permissions"`
}

// hasuraTablePermsYAML holds the aggregated permission info for one table,
// built by walking select_permissions entries across one or more YAML files.
type hasuraTablePermsYAML struct {
	hasUserRole bool
	filterRefs  string // lowercase concatenation of filter map keys
	interchange string // role whose select filter swaps the two isolation variables (ADR 0004)
}

// CheckHasuraMetadataYAML implements PERM-HASURA-01.
//
// It walks hasura/metadata/ (and the subdirectory databases/default/tables/ if
// present) under projectDir, parses every *.yaml file that defines a table, and
// verifies that each tracked np_* table has the correct user-role row filter.
//
// Rules:
//   - np_* table with source_account_id in filter  → PASS
//   - np_* table with tenant_id in filter           → PASS
//   - np_* table with neither filter                → FAIL (PERM-HASURA-01)
//   - np_* table with no user select_permission     → FAIL (PERM-HASURA-01)
//   - non-np_* table or table with no isolation col → exempt
//
// Pass strict=true to report failures as "fail" (blocks deploy). Default "warn".
// ctx is reserved for future cancellation; disk reads are synchronous.
func CheckHasuraMetadataYAML(_ context.Context, projectDir string, strict bool) []CheckResult {
	status := func(fail bool) string {
		if fail && strict {
			return "fail"
		}
		return "warn"
	}

	metaDir := filepath.Join(projectDir, "hasura", "metadata")
	if _, err := os.Stat(metaDir); os.IsNotExist(err) {
		return []CheckResult{{
			Section: "security",
			Name:    HasuraMetaCheckID + ": metadata dir",
			Status:  "pass",
			Message: fmt.Sprintf("no hasura/metadata directory at %s — skipping YAML row-filter audit", projectDir),
		}}
	}

	// Collect all candidate YAML paths: first try the per-table subdirectory
	// (Hasura CLI v2 layout), then fall back to flat files at the root.
	var yamlPaths []string

	subDir := filepath.Join(metaDir, "databases", "default", "tables")
	if info, err := os.Stat(subDir); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(subDir)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
				yamlPaths = append(yamlPaths, filepath.Join(subDir, e.Name()))
			}
		}
	}

	// Also include YAML files at the metadata root (legacy flat layout or
	// tables.yaml that inlines all table definitions).
	rootEntries, _ := os.ReadDir(metaDir)
	for _, e := range rootEntries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		if e.Name() == "tables.yaml" || strings.HasPrefix(e.Name(), "public_") || strings.HasPrefix(e.Name(), "np_") {
			yamlPaths = append(yamlPaths, filepath.Join(metaDir, e.Name()))
		}
	}

	if len(yamlPaths) == 0 {
		return []CheckResult{{
			Section: "security",
			Name:    HasuraMetaCheckID + ": table files",
			Status:  "pass",
			Message: "no table YAML files found in hasura/metadata/ — skipping row-filter audit",
		}}
	}

	// Parse every YAML file and collect per-table permission info.
	tableMap := make(map[string]hasuraTablePermsYAML)

	for _, path := range yamlPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		// A file may contain a single table or a YAML list of tables.
		// Try single first, then list.
		var single hasuraTableYAML
		if err := yaml.Unmarshal(data, &single); err == nil && single.Table.Name != "" {
			mergeTablePerms(tableMap, single)
			continue
		}

		var many []hasuraTableYAML
		if err := yaml.Unmarshal(data, &many); err == nil {
			for _, t := range many {
				if t.Table.Name != "" {
					mergeTablePerms(tableMap, t)
				}
			}
		}
	}

	// Evaluate each np_* table.
	var violations []CheckResult
	passCount := 0

	for tableName, p := range tableMap {
		if !strings.HasPrefix(tableName, "np_") {
			continue
		}

		hasTID := strings.Contains(p.filterRefs, "tenant_id")
		hasSAID := strings.Contains(p.filterRefs, "source_account_id")

		// P7-ADOPT-05: source_account_id compared to X-Hasura-Tenant-Id (or the reverse)
		// is the interchange ADR 0004 forbids; any role's select filter counts.
		if p.interchange != "" {
			st := "warn"
			// compat.V15(P7-ADOPT-05): interchange passes -> fail
			if compat.V15() {
				st = "fail"
			}
			violations = append(violations, CheckResult{
				Section: "security",
				Name:    fmt.Sprintf("%s: %s", HasuraMetaCheckID, tableName),
				Status:  st,
				Message: fmt.Sprintf("HASURA-FILTER-INTERCHANGE table=%s role=%s: select filter compares source_account_id to X-Hasura-Tenant-Id or tenant_id to X-Hasura-Source-Account-Id — the two walls must not be interchanged", tableName, p.interchange),
				FixCmd:  fmt.Sprintf("compare source_account_id to X-Hasura-Source-Account-Id and tenant_id to X-Hasura-Tenant-Id in the %s select filter for %s", p.interchange, tableName),
			})
			continue
		}

		// No isolation column detected in the filter — check if the table even
		// has a user-role permission entry.
		if !p.hasUserRole {
			violations = append(violations, CheckResult{
				Section: "security",
				Name:    fmt.Sprintf("%s: %s", HasuraMetaCheckID, tableName),
				Status:  status(true),
				Message: fmt.Sprintf("HASURA-FILTER-MISSING table=%s: no select_permissions for 'user' role — GraphQL layer bypasses Postgres RLS", tableName),
				FixCmd:  fmt.Sprintf("add select_permissions.user.filter.source_account_id or tenant_id to hasura/metadata for %s", tableName),
			})
			continue
		}

		if !hasTID && !hasSAID {
			// User role exists but filter references neither isolation column.
			violations = append(violations, CheckResult{
				Section: "security",
				Name:    fmt.Sprintf("%s: %s", HasuraMetaCheckID, tableName),
				Status:  status(true),
				Message: fmt.Sprintf("HASURA-FILTER-MISSING table=%s role=user: select_permissions filter does not reference source_account_id or tenant_id — GraphQL layer bypasses Postgres RLS", tableName),
				FixCmd:  fmt.Sprintf("add filter: {source_account_id: {_eq: X-Source-Account}} or {tenant_id: {_eq: X-Hasura-Tenant-Id}} to 'user' select_permissions for %s", tableName),
			})
			continue
		}

		passCount++
	}

	if len(violations) == 0 {
		msg := "all tracked np_* tables have correct user-role row filter in Hasura metadata"
		if passCount == 0 {
			msg = "no np_* tables tracked in Hasura metadata — row-filter audit skipped"
		}
		return []CheckResult{{
			Section: "security",
			Name:    HasuraMetaCheckID + ": row filters",
			Status:  "pass",
			Message: msg,
		}}
	}

	return violations
}

// mergeTablePerms updates tableMap with the permission info from t.
func mergeTablePerms(tableMap map[string]hasuraTablePermsYAML, t hasuraTableYAML) {
	name := t.Table.Name
	if name == "" {
		return
	}
	p := tableMap[name]
	for _, sp := range t.SelectPermissions {
		if p.interchange == "" && interchange(sp.Permission.Filter) {
			p.interchange = sp.Role
		}
		if sp.Role != "user" {
			continue
		}
		p.hasUserRole = true
		// Collect filter keys to detect which isolation column is referenced.
		for k := range sp.Permission.Filter {
			p.filterRefs += " " + strings.ToLower(k)
		}
	}
	tableMap[name] = p
}
