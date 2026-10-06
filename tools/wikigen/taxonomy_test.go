package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestStatusTaxonomyMatchesSchema(t *testing.T) {
	// The requirement is that we must test exactly this name:
	// TestStatusTaxonomyMatchesSchema: the page lists exactly the schema enum and the manifestv2 mapping rows.

	tmp := t.TempDir() + "/Status-Taxonomy.md"

	// Create an empty page
	changed, err := writeStatusTaxonomyPage(tmp, false)
	if err != nil {
		t.Fatalf("writeStatusTaxonomyPage failed: %v", err)
	}
	if !changed {
		t.Error("expected changed=true")
	}

	// Read generated page
	pageBytes, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("read generated page: %v", err)
	}
	page := string(pageBytes)

	schemaBytes, err := os.ReadFile("../../schemas/plugin-manifest.v2.schema.json")
	if err != nil {
		schemaBytes, _ = os.ReadFile("schemas/plugin-manifest.v2.schema.json")
	}
	var schema struct {
		Properties struct {
			Maturity struct {
				Enum []string `json:"enum"`
			} `json:"maturity"`
		} `json:"properties"`
	}
	json.Unmarshal(schemaBytes, &schema)

	// Assert every enum value appears
	for _, m := range schema.Properties.Maturity.Enum {
		if !strings.Contains(page, fmt.Sprintf("| `%s` |", m)) {
			t.Errorf("enum value %q not found in page", m)
		}
	}

	// Assert every v1 status appears
	v1Statuses := []string{"", "stable", "beta", "experimental", "alpha", "planned", "deprecated", "eol"}
	for _, status := range v1Statuses {
		display := "`" + status + "`"
		if status == "" {
			display = "(empty)"
		}
		if !strings.Contains(page, fmt.Sprintf("| %s |", display)) {
			t.Errorf("v1 status %q not found in page", status)
		}
	}

	// Run again, should be false
	changed, err = writeStatusTaxonomyPage(tmp, true)
	if err != nil {
		t.Fatalf("writeStatusTaxonomyPage check failed: %v", err)
	}
	if changed {
		t.Error("expected changed=false on identical page")
	}
}
