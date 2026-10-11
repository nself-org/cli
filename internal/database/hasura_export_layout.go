package database

// Purpose: the one on-disk layout for Hasura metadata, shared by the local
// export (hasura/metadata/) and the tar archive (sync --env, D-0272).
// Inputs: export_metadata JSON bytes.
// Outputs: an ordered file set: <key>.yaml per top-level key, plus config.yaml
// holding one `<key>: !include <key>.yaml` line per key (sorted).
// Constraints: pure and deterministic (sorted keys, no clock, no disk) except
// writeMetadataTree, which only writes the files it is given. A key that is not
// a safe file name is rejected. The layout is what applyViaIncludeResolver reads.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const metadataIndexFile = "config.yaml"

// metadataFile is one file of the layout; Name is relative to the metadata dir.
type metadataFile struct {
	Name string
	Data []byte
}

// metadataLayout turns export_metadata JSON into the shared file set, sorted by
// file name (config.yaml included). It fails when the JSON is not an object or
// a top-level key cannot be a plain file name.
func metadataLayout(data []byte) ([]metadataFile, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse exported metadata: %w", err)
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		if err := checkMetadataKey(k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	files := make([]metadataFile, 0, len(keys)+1)
	var index strings.Builder
	for _, k := range keys {
		body, err := yaml.Marshal(raw[k])
		if err != nil {
			return nil, fmt.Errorf("marshal %s to YAML: %w", k, err)
		}
		files = append(files, metadataFile{Name: k + ".yaml", Data: body})
		index.WriteString(k + ": !include " + k + ".yaml\n")
	}
	files = append(files, metadataFile{Name: metadataIndexFile, Data: []byte(index.String())})
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

// checkMetadataKey rejects a top-level key that would escape the directory,
// collide with the index file, or break an `!include` line.
func checkMetadataKey(k string) error {
	if k == "" || k == "config" || strings.HasPrefix(k, ".") || strings.Contains(k, "..") ||
		strings.ContainsAny(k, "/\\:#") || strings.TrimSpace(k) != k || strings.ContainsAny(k, " \t\r\n") {
		return fmt.Errorf("metadata key %q cannot be a file name in the metadata layout", k)
	}
	return nil
}

// writeMetadataTree writes the layout for data into dir (created if needed).
// Files already in dir that the layout does not name are left alone.
func writeMetadataTree(dir string, data []byte) error {
	files, err := metadataLayout(data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create metadata dir: %w", err)
	}
	for _, f := range files {
		p := filepath.Join(dir, f.Name)
		if err := os.WriteFile(p, f.Data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", p, err)
		}
	}
	return nil
}
