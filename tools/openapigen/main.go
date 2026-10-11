package main

// Purpose: regenerate the committed HTTP OpenAPI artifact.
// Inputs: machine registry under an isolated HOME and plugin directory.
// Outputs: .github/openapi.json, or a drift verdict with -check.
// Constraints: hermetic registry; exit 1 means drift, exit 2 means tool error.

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/cmd/commands"
	"github.com/nself-org/cli/internal/httpapi"
	"github.com/nself-org/cli/schemas"
)

func main() {
	check := flag.Bool("check", false, "fail if the committed OpenAPI document is stale")
	flag.Parse()
	code, err := run(*check)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

func run(check bool) (int, error) {
	undo, err := prepare()
	if err != nil {
		return 2, err
	}
	defer undo()
	reg, err := commands.MachineRegistry()
	if err != nil {
		return 2, err
	}
	b, err := httpapi.OpenAPI(reg, schemas.Lookup)
	if err != nil {
		return 2, err
	}
	path := filepath.Join(".github", "openapi.json")
	if check {
		current, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return 1, fmt.Errorf("%s is missing: run go run -mod=vendor ./tools/openapigen", path)
		}
		if err != nil {
			return 2, err
		}
		if !bytes.Equal(current, b) {
			return 1, fmt.Errorf("%s is stale: run go run -mod=vendor ./tools/openapigen", path)
		}
		return 0, nil
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		return 2, err
	}
	return 0, nil
}

// prepare isolates user state before preparing the v1.5 command tree.
func prepare() (func(), error) {
	home, err := os.MkdirTemp("", "nself-openapigen-")
	if err != nil {
		return nil, err
	}
	previous := map[string]*string{}
	for key, value := range map[string]string{
		"HOME": home, "NSELF_PLUGIN_DIR": filepath.Join(home, "plugins"), "NSELF_V15": "1",
	} {
		if old, ok := os.LookupEnv(key); ok {
			copy := old
			previous[key] = &copy
		} else {
			previous[key] = nil
		}
		_ = os.Setenv(key, value)
	}
	undoTree := commands.PrepareTreeForGeneration()
	return func() {
		undoTree()
		for key, value := range previous {
			if value == nil {
				_ = os.Unsetenv(key)
			} else {
				_ = os.Setenv(key, *value)
			}
		}
		_ = os.RemoveAll(home)
	}, nil
}
