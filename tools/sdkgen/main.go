package main

// Purpose: generate the typed control-API client from .github/openapi.json.
// Inputs: the committed OpenAPI document and the package README.
// Outputs: sdk/ts-cli-api/src/generated/{types,client}.ts and the README region, or a drift verdict with -check.
// Constraints: no network; exit 1 means drift, exit 2 means tool error.

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	lang := flag.String("lang", "", "target language (ts)")
	check := flag.Bool("check", false, "fail if the generated files are stale")
	spec := flag.String("openapi", filepath.Join(".github", "openapi.json"), "OpenAPI document")
	out := flag.String("out", filepath.Join("sdk", "ts-cli-api"), "package directory")
	flag.Parse()
	code, err := run(*lang, *spec, *out, *check)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

func run(lang, spec, out string, check bool) (int, error) {
	if lang != "ts" {
		return 2, fmt.Errorf("sdkgen: -lang ts is required (got %q)", lang)
	}
	files, err := generate(spec, out)
	if err != nil {
		return 2, err
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var stale []string
	for _, p := range paths {
		current, err := os.ReadFile(p)
		if err == nil && bytes.Equal(current, files[p]) {
			continue
		}
		if check {
			stale = append(stale, p)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return 2, err
		}
		if err := os.WriteFile(p, files[p], 0o644); err != nil {
			return 2, err
		}
	}
	if len(stale) > 0 {
		return 1, fmt.Errorf("sdkgen: stale (run go run -mod=vendor ./tools/sdkgen -lang ts): %v", stale)
	}
	return 0, nil
}

// generate returns the full set of output files keyed by their path.
func generate(spec, out string) (map[string][]byte, error) {
	raw, err := os.ReadFile(spec)
	if err != nil {
		return nil, err
	}
	m, err := ParseOpenAPI(raw)
	if err != nil {
		return nil, err
	}
	rel, ops, err := Render(m)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for p, b := range rel {
		files[filepath.Join(out, filepath.FromSlash(p))] = b
	}
	readmePath := filepath.Join(out, "README.md")
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		return nil, err
	}
	if files[readmePath], err = ReplaceReadmeRegion(readme, ops); err != nil {
		return nil, err
	}
	return files, nil
}
