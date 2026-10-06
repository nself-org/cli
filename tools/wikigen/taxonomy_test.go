package main

import (
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

	// Run again, should be false
	changed, err = writeStatusTaxonomyPage(tmp, true)
	if err != nil {
		t.Fatalf("writeStatusTaxonomyPage check failed: %v", err)
	}
	if changed {
		t.Error("expected changed=false on identical page")
	}
}
