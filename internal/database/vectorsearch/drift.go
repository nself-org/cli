package vectorsearch

// drift.go answers "does the on-disk metadata match live Hasura?" before a
// mirror. The YAML directory goes through database.HasuraDiffMetadata; the
// legacy hasura/metadata.json is compared as a whole document. resource_version
// is ignored on both sides: Hasura bumps it on every change, so a rerun after a
// mirror would otherwise report drift.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
)

// MetadataDrift lists what differs between live Hasura and the project's
// on-disk metadata. Empty means no drift.
func MetadataDrift(ctx context.Context, cfg *config.Config, projectDir string) ([]string, error) {
	if st, err := os.Stat(filepath.Join(projectDir, "hasura", "metadata")); err == nil && st.IsDir() {
		return database.HasuraDiffMetadata(ctx, cfg, projectDir)
	}
	disk, err := os.ReadFile(filepath.Join(projectDir, "hasura", "metadata.json"))
	if err != nil {
		return nil, fmt.Errorf("no on-disk metadata: %w", err)
	}
	live, err := database.HasuraExportMetadata(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return jsonDrift(live, disk), nil
}

// jsonDrift compares two metadata documents, ignoring resource_version.
func jsonDrift(live, disk []byte) []string {
	a, b := decodeMetadata(live), decodeMetadata(disk)
	if a == nil || b == nil {
		return []string{"metadata.json (unreadable)"}
	}
	if reflect.DeepEqual(a, b) {
		return nil
	}
	return []string{"metadata.json"}
}

// decodeMetadata decodes a JSON object keeping numbers exact and dropping the
// top-level resource_version; nil means invalid JSON or not an object.
func decodeMetadata(b []byte) map[string]any {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v map[string]any
	if err := dec.Decode(&v); err != nil {
		return nil
	}
	delete(v, "resource_version")
	return v
}
