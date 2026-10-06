package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

func writeStatusTaxonomyPage(path string, check bool) (bool, error) {
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	page := string(current)
	if page == "" {
		page = "# Status Taxonomy\n\n"
	}

	schemaBytes, err := os.ReadFile("../../schemas/plugin-manifest.v2.schema.json")
	if err != nil {
		schemaBytes, err = os.ReadFile("schemas/plugin-manifest.v2.schema.json")
		if err != nil {
			return false, fmt.Errorf("read schema: %v", err)
		}
	}

	var schema struct {
		Properties struct {
			Maturity struct {
				Enum []string `json:"enum"`
			} `json:"maturity"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return false, fmt.Errorf("parse schema: %v", err)
	}

	var maturityTable strings.Builder
	maturityTable.WriteString("| Maturity |\n")
	maturityTable.WriteString("|----------|\n")
	for _, m := range schema.Properties.Maturity.Enum {
		fmt.Fprintf(&maturityTable, "| `%s` |\n", m)
	}

	var mappingTable strings.Builder
	mappingTable.WriteString("| v1 Status | v2 Maturity | v2 State |\n")
	mappingTable.WriteString("|-----------|-------------|----------|\n")

	v1Statuses := []string{"", "stable", "beta", "experimental", "alpha", "planned", "deprecated", "eol"}
	for _, status := range v1Statuses {
		jsonStr := fmt.Sprintf(`{"name": "test", "version": "1.0.0", "status": "%s", "deprecation": {"announcedDate": "2026-01-01", "eolDate": "2026-02-01", "migrationGuide": "https://example.com"}}`, status)
		m, err := manifestv2.Normalize([]byte(jsonStr))
		if err != nil {
			return false, fmt.Errorf("normalize %q: %v", status, err)
		}

		state := ""
		if m.Deprecation != nil {
			state = m.Deprecation.State
		}

		var displayStatus string
		if status == "" {
			displayStatus = "(empty)"
		} else {
			displayStatus = "`" + status + "`"
		}

		if state == "" {
			fmt.Fprintf(&mappingTable, "| %s | `%s` | |\n", displayStatus, m.Maturity)
		} else {
			fmt.Fprintf(&mappingTable, "| %s | `%s` | `%s` |\n", displayStatus, m.Maturity, state)
		}
	}

	page = replaceOrInsert(page, "maturity-enum", maturityTable.String())
	page = replaceOrInsert(page, "v1-mapping", mappingTable.String())

	if page == string(current) {
		return false, nil
	}
	if check {
		return true, nil
	}
	return true, os.WriteFile(path, []byte(page), 0o644)
}
