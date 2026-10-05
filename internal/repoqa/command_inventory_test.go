package repoqa

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// inventoryEntry mirrors the top-level shape emitted by tools/cmdinventory.
type inventoryEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// TestCommandInventoryIsCurrent fails when .github/command-inventory.json no
// longer matches the cobra tree.
//
// Before CLI-R06 every published command count was hand-typed and every one of
// them was wrong: SPORT F02 and the PRI both claimed 84 while the binary
// registered 92. Regenerating from the tree makes the drift impossible; this
// test makes forgetting to regenerate impossible too.
func TestCommandInventoryIsCurrent(t *testing.T) {
	root := repoRoot(t)
	committedPath := filepath.Join(root, ".github", "command-inventory.json")

	committed, err := os.ReadFile(committedPath)
	if err != nil {
		t.Fatalf("read committed inventory: %v (run `make cmd-inventory`)", err)
	}

	gen := exec.Command("go", "run", "-mod=vendor", "./tools/cmdinventory", "-format", "json", "-depth", "2")
	gen.Dir = root
	gen.Env = append(os.Environ(), "CGO_ENABLED=0")
	live, err := gen.Output()
	if err != nil {
		t.Fatalf("regenerate inventory: %v", err)
	}

	if normalizeEOL(string(committed)) != normalizeEOL(string(live)) {
		t.Fatalf("command inventory is stale — run `make cmd-inventory` and commit the result.\n"+
			"committed %d bytes, regenerated %d bytes", len(committed), len(live))
	}
}

// TestCommandCanonPageIsCurrent fails when .github/wiki/Command-Canon.md no
// longer matches the canon rows (a fragment changed without `make
// cmd-inventory`, or someone hand-edited the generated table). The page's
// removal-plan PROSE block is the one hand-written part and is carried over.
func TestCommandCanonPageIsCurrent(t *testing.T) {
	root := repoRoot(t)
	page := filepath.Join(root, ".github", "wiki", "Command-Canon.md")
	committed, err := os.ReadFile(page)
	if err != nil {
		t.Fatalf("read Command-Canon.md: %v (run `make cmd-inventory`)", err)
	}
	gen := exec.Command("go", "run", "-mod=vendor", "./tools/cmdinventory", "-format", "canon", "-prose-from", page)
	gen.Dir = root
	gen.Env = append(os.Environ(), "CGO_ENABLED=0")
	live, err := gen.Output()
	if err != nil {
		t.Fatalf("regenerate canon page: %v", err)
	}
	if normalizeEOL(string(committed)) != normalizeEOL(string(live)) {
		t.Fatal("Command-Canon.md is stale or hand-edited — run `make cmd-inventory` and commit the result")
	}
}

// TestPublishedCommandCountMatchesBinary checks the human-readable count in the
// generated wiki index against the machine-readable inventory, so the two
// generated artifacts can never disagree with each other.
func TestPublishedCommandCountMatchesBinary(t *testing.T) {
	root := repoRoot(t)

	data, err := os.ReadFile(filepath.Join(root, ".github", "command-inventory.json"))
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	var entries []inventoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("parse inventory: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("inventory is empty")
	}

	wiki, err := os.ReadFile(filepath.Join(root, ".github", "wiki", "Commands.md"))
	if err != nil {
		t.Fatalf("read wiki index: %v", err)
	}

	want := "**Total top-level commands: " + strconv.Itoa(len(entries)) + "**"
	if !strings.Contains(normalizeEOL(string(wiki)), want) {
		t.Fatalf(".github/wiki/Commands.md does not state %q — run `make cmd-inventory`", want)
	}

	// Every inventory entry must have a row in the index.
	for _, e := range entries {
		if !strings.Contains(normalizeEOL(string(wiki)), "`"+e.Path+"`") {
			t.Errorf("command %q is missing from the generated wiki index", e.Path)
		}
	}
}

// TestCoreServicesPageIsCurrent fails when .github/wiki/Core-Services.md no
// longer matches internal/compose's catalog (CLI-R07). The page is generated so
// that the published required/optional split cannot drift from the generator
// the way SPORT F02's command count did.
func TestCoreServicesPageIsCurrent(t *testing.T) {
	root := repoRoot(t)

	committed, err := os.ReadFile(filepath.Join(root, ".github", "wiki", "Core-Services.md"))
	if err != nil {
		t.Fatalf("read Core-Services.md: %v (run `make core-services`)", err)
	}

	gen := exec.Command("go", "run", "-mod=vendor", "./tools/servicecatalog", "-format", "markdown")
	gen.Dir = root
	gen.Env = append(os.Environ(), "CGO_ENABLED=0")
	live, err := gen.Output()
	if err != nil {
		t.Fatalf("regenerate catalog: %v", err)
	}

	if !strings.Contains(normalizeEOL(string(committed)), normalizeEOL(strings.TrimSpace(string(live)))) {
		t.Fatal("core-services page is stale — run `make core-services` and commit the result")
	}
}

// normalizeEOL strips carriage returns before comparing generated output.
//
// The repo is LF-only (.gitattributes enforces it) and Go tooling emits LF on
// every platform, but a checkout configured otherwise would turn every one of
// these drift tests into a false failure that says "stale" when nothing is
// stale. Comparing without \r keeps the tests reporting real drift only.
func normalizeEOL(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
