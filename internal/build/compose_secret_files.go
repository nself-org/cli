package build

// Purpose: materialize Compose file secrets without exposing host-readable data.
// Inputs: generated Compose and resolved config. Outputs: raw files under a
// private generated directory. Constraints: plan mode uses Sink; no symlinks.
// SPORT: cli/build secret transport (D-0551).

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"gopkg.in/yaml.v3"
)

const composeSecretsDir = ".nself/secrets"

// writeComposeSecretFilesVia writes only secrets referenced by this render.
// Files are 0444 so the existing non-root image UIDs can read their mounts;
// the host directory must be 0700 BEFORE any credential reaches a file.
func writeComposeSecretFilesVia(sink Sink, workdir string, cfg *config.Config, raw []byte) (int, error) {
	var doc struct {
		Secrets map[string]struct {
			File string `yaml:"file"`
		} `yaml:"secrets"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return 0, err
	}
	if len(doc.Secrets) == 0 {
		return 0, nil
	}
	for _, rel := range []string{".nself", composeSecretsDir} {
		info, err := os.Lstat(filepath.Join(workdir, rel))
		if err != nil && !os.IsNotExist(err) {
			return 0, err
		}
		if err == nil && !info.IsDir() {
			return 0, fmt.Errorf("secret directory %s must be a real directory", rel)
		}
	}
	dir := filepath.Join(workdir, composeSecretsDir)
	if err := sink.MkdirAll(dir, 0700); err != nil {
		return 0, err
	}
	if err := sink.Chmod(dir, 0700); err != nil {
		return 0, err
	}
	values := compose.SecretFileValues(cfg)
	for _, secret := range doc.Secrets {
		key := filepath.Base(secret.File)
		if _, known := values[key]; !known || secret.File != "./"+composeSecretsDir+"/"+key {
			return 0, fmt.Errorf("unexpected generated secret source %s", key)
		}
		if err := sink.WriteAtomic(filepath.Join(dir, key), []byte(values[key]), 0444); err != nil {
			return 0, err
		}
	}
	return len(doc.Secrets), nil
}
