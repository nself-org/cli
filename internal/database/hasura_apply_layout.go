package database

// Purpose: decide which applier reads a hasura/metadata/ directory (debt
// D-0564). nself writes its own layout (a config.yaml index of !include lines
// at the metadata root, one <key>.yaml per top-level key; hasura_export_layout.go),
// which only the in-process include resolver reads. The Hasura CLI expects its
// own project (hasura/config.yaml, optional metadata_directory, then version.yaml
// and databases/), so it is used only for a real Hasura CLI project.
// Inputs: the hasura/ project directory and its metadata/ directory.
// Outputs: isHasuraCLIProject.
// Constraints: reads two small files; any read or parse failure answers "not a
// Hasura CLI project", which falls back to the include resolver.

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// hasuraCLIProjectConfig is the part of a Hasura CLI project's config.yaml
// this check reads.
type hasuraCLIProjectConfig struct {
	MetadataDirectory string `yaml:"metadata_directory"`
}

// isHasuraCLIProject reports whether hasuraDir is a Hasura CLI project whose
// metadata the hasura binary should apply: hasuraDir/config.yaml names a
// metadata_directory (default metadata) identifying metadataDir, which carries no
// nself config.yaml index (that index is nself's layout, which the Hasura CLI
// cannot read).
func isHasuraCLIProject(hasuraDir, metadataDir string) bool {
	if _, err := os.Stat(filepath.Join(metadataDir, metadataIndexFile)); err == nil {
		return false
	}
	raw, err := os.ReadFile(filepath.Join(hasuraDir, "config.yaml"))
	if err != nil {
		return false
	}
	var c hasuraCLIProjectConfig
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return false
	}
	dir := strings.TrimSpace(c.MetadataDirectory)
	if dir == "" {
		dir = "metadata"
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(hasuraDir, dir)
	}
	configured, err := os.Stat(dir)
	if err != nil || !configured.IsDir() {
		return false
	}
	metadata, err := os.Stat(metadataDir)
	return err == nil && metadata.IsDir() && os.SameFile(configured, metadata)
}
