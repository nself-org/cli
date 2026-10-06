package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestErrorCodesPageMatchesRegistry(t *testing.T) {
	entries := errs.Registry

	page, err := renderErrorCodesPage(entries, errCodesProse{})
	if err != nil {
		t.Fatalf("renderErrorCodesPage failed: %v", err)
	}

	for code, e := range entries {
		expectedId := strings.Split(e.DocsPath, "#")[1]
		if expectedId != strings.ToLower(code) {
			t.Errorf("DocsPath fragment for %s is %q, want %q", code, expectedId, strings.ToLower(code))
		}

		anchor := fmt.Sprintf(`<a id="%s"></a>%s`, expectedId, code)
		if !strings.Contains(page, anchor) {
			t.Errorf("page missing anchor for code %s: expected %q", code, anchor)
		}
	}
}

func TestErrorCodesPageFixture(t *testing.T) {
	fixture := map[string]errs.CodeEntry{
		"E001": {Category: "docker", Summary: "test", DefaultWhy: "test", DefaultFix: "test", DocsPath: "reference/error-codes#e001", Exit: 2},
	}

	// Create a temp file
	tmp := t.TempDir() + "/error-codes.md"

	// 1. Initial write
	changed, err := writeErrorCodesPage(tmp, fixture, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("expected changed=true on first write")
	}

	// 2. Check should be false
	changed, err = writeErrorCodesPage(tmp, fixture, true)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		b, _ := os.ReadFile(tmp)
		p, _ := renderErrorCodesPage(fixture, readErrorCodesProse(string(b)))
		t.Errorf("expected changed=false. Current:\n%s\nNew:\n%s", string(b), p)
	}

	// 3. Add a code, check should be true
	fixture["E002"] = errs.CodeEntry{Category: "docker", Summary: "test2", DefaultWhy: "test2", DefaultFix: "test2", DocsPath: "reference/error-codes#e002", Exit: 2}
	changed, err = writeErrorCodesPage(tmp, fixture, true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("expected changed=true when fixture gains a code")
	}
}
