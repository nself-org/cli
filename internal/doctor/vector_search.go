package doctor

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/database/vectorsearch"
)

// Purpose: doctor --deep re-check of tracked vector-search functions (D-0339).
// `nself db generate vector-search --track` refuses (E514) while any select
// role hides the vector column or the table's root select field, but Hasura
// re-infers function access on every metadata change, so a permission added
// or narrowed later reopens the ordering oracle. This check re-runs those
// rules against live metadata.
// Inputs: the project directory (its config gives the Hasura endpoint and
// admin secret; its .env cascade gives HASURA_GRAPHQL_INFER_FUNCTION_PERMISSIONS).
// Outputs: one failing CheckResult (section "hasura", name
// VECTOR-SEARCH-<schema.function>) per unsafe function, one passing result
// when every tracked search function is safe, nothing when none is tracked.
// When live metadata cannot be read it gives one "skip" naming the reason if
// the on-disk metadata names a search function (never a silent pass where
// one is known to exist), and nothing otherwise, so a stopped stack with no
// vector search adds no noise.
// Constraints: one read-only export_metadata call with a 10s timeout.

const (
	vectorSearchTimeout = 10 * time.Second
	inferFunctionPerms  = "HASURA_GRAPHQL_INFER_FUNCTION_PERMISSIONS"
	diskScanFileCap     = 8 << 20
)

// exportLiveMetadata reads live Hasura metadata; tests replace it so no unit
// test touches the network.
var exportLiveMetadata = database.HasuraExportMetadata

// VectorSearchChecks is the DeepChecks entry point.
func VectorSearchChecks(ctx context.Context, projectDir string) []CheckResult {
	if projectDir == "" {
		return nil
	}
	cfg, env, err := LoadConfigIsolatedLookup(projectDir, inferFunctionPerms)
	if err != nil {
		return vectorSearchUnread(projectDir, fmt.Sprintf("project config unreadable: %v", err))
	}
	ctx, cancel := context.WithTimeout(ctx, vectorSearchTimeout)
	defer cancel()
	live, err := exportLiveMetadata(ctx, cfg)
	if err != nil {
		return vectorSearchUnread(projectDir, fmt.Sprintf("live Hasura metadata unreadable (is the stack running?): %v", err))
	}
	return vectorSearchResults(live, !strings.EqualFold(strings.TrimSpace(env[inferFunctionPerms]), "false"))
}

// vectorSearchResults turns an export_metadata document into check results.
func vectorSearchResults(live []byte, inferred bool) []CheckResult {
	findings, seen, err := vectorsearch.AuditLive(live, inferred)
	if err != nil {
		return []CheckResult{vectorSearchSkip(err.Error())}
	}
	if seen == 0 {
		return nil
	}
	if len(findings) == 0 {
		return []CheckResult{{Section: "hasura", Name: "VECTOR-SEARCH", Status: "pass",
			Message: fmt.Sprintf("%d tracked vector-search function(s): every role that can call one sees the vector column and the root select field", seen)}}
	}
	results := make([]CheckResult, 0, len(findings))
	for _, f := range findings {
		results = append(results, CheckResult{Section: "hasura", Name: "VECTOR-SEARCH-" + f.Function, Status: "fail",
			Message: fmt.Sprintf("E514: %s orders %s by %s, and these roles can call it: %s. They could learn hidden data from the result order. (A function named <table>_nearest_<column> is read as a vector search; rename a function that is not one.)",
				f.Function, f.Table, f.Column, strings.Join(f.Problems, "; ")),
			FixCmd: "give those roles the vector column and root select, or remove their select permission on " + f.Table + "; then run nself db generate vector-search --track again"})
	}
	return results
}

// vectorSearchUnread is the result when live metadata cannot be read: a skip
// when the on-disk metadata names a search function, nothing otherwise.
func vectorSearchUnread(projectDir, why string) []CheckResult {
	if !diskNamesSearchFunction(projectDir) {
		return nil
	}
	return []CheckResult{vectorSearchSkip(why)}
}

// diskNamesSearchFunction reports whether any file of the on-disk Hasura
// metadata (hasura/metadata/**, hasura/metadata.json) contains "_nearest_".
// Unreadable or oversized files are skipped.
func diskNamesSearchFunction(projectDir string) bool {
	found := false
	scan := func(path string) {
		if st, err := os.Stat(path); err != nil || st.Size() > diskScanFileCap {
			return
		}
		if b, err := os.ReadFile(path); err == nil && bytes.Contains(b, []byte("_nearest_")) {
			found = true
		}
	}
	scan(filepath.Join(projectDir, "hasura", "metadata.json"))
	_ = filepath.WalkDir(filepath.Join(projectDir, "hasura", "metadata"), func(path string, d fs.DirEntry, err error) error {
		if found {
			return fs.SkipAll
		}
		if err != nil {
			if path == filepath.Join(projectDir, "hasura", "metadata") {
				return fs.SkipAll // no metadata directory
			}
			return nil // one unreadable entry must not hide the rest
		}
		if d.Type().IsRegular() {
			scan(path)
		}
		return nil
	})
	return found
}

func vectorSearchSkip(why string) CheckResult {
	return CheckResult{Section: "hasura", Name: "VECTOR-SEARCH", Status: "skip", Message: "vector-search permissions not checked: " + why}
}
