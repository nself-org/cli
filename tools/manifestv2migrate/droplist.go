package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

// loadDropList reads the -drop file: one v1 key per line, optional "# comment"
// after it, blank lines and whole-line comments ignored. An empty path is an
// empty list. A key the normalizer maps or converts cannot be listed: dropping
// it could discard data the tool would otherwise carry.
func loadDropList(path string) (map[string]bool, error) {
	out := map[string]bool{}
	if path == "" {
		return out, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	for i, line := range strings.Split(string(b), "\n") {
		key, _, _ := strings.Cut(line, "#")
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if strings.ContainsAny(key, " \t") {
			return nil, fmt.Errorf("%s:%d: one key per line, got %q", path, i+1, key)
		}
		if manifestv2.IsMappedV1Key(key) {
			return nil, fmt.Errorf("%s:%d: %q is mapped to v2 and cannot be dropped", path, i+1, key)
		}
		out[key] = true
	}
	return out, nil
}
