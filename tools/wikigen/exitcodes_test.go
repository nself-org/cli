package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExitCodesPage(t *testing.T) {
	// Write a mock command-registry.json
	regJSON := `{
		"commands": [
			{
				"path": "nself status",
				"name": "status",
				"exit_codes": {"10": "unhealthy"}
			}
		]
	}`

	// Create .github/command-registry.json in the current dir for the test
	os.MkdirAll(".github", 0755)
	os.WriteFile(".github/command-registry.json", []byte(regJSON), 0644)
	defer os.RemoveAll(".github")

	tmp := filepath.Join(t.TempDir(), "Exit-Codes.md")
	os.WriteFile(tmp, []byte("## The contract\n\n| Code | Class | Meaning |\n|---|---|---|\n| 0 | ok | Success. |\n\n## State codes\n\n| Command | Exit | `data.state` | Meaning |\n|---|---|---|---|\n| `status` | 0 | `ok` | every service is healthy |\n"), 0644)

	changed, err := writeExitCodesPage(tmp, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("expected changed=true")
	}

	changed, err = writeExitCodesPage(tmp, true)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("expected changed=false on identical")
	}
}
