package doctor

import (
	"context"
	"fmt"
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
// admin secret).
// Outputs: one failing CheckResult (section "hasura", name
// VECTOR-SEARCH-<schema.function>) per unsafe function, one passing result
// when every tracked search function is safe, nothing when none is tracked,
// and one "skip" naming the reason when live metadata cannot be read (never a
// silent pass).
// Constraints: one read-only export_metadata call with a 10s timeout.

const vectorSearchTimeout = 10 * time.Second

// exportLiveMetadata reads live Hasura metadata; tests replace it so no unit
// test touches the network.
var exportLiveMetadata = database.HasuraExportMetadata

// VectorSearchChecks is the DeepChecks entry point.
func VectorSearchChecks(ctx context.Context, projectDir string) []CheckResult {
	if projectDir == "" {
		return nil
	}
	cfg, err := LoadConfigIsolated(projectDir)
	if err != nil {
		return []CheckResult{vectorSearchSkip(fmt.Sprintf("project config unreadable: %v", err))}
	}
	ctx, cancel := context.WithTimeout(ctx, vectorSearchTimeout)
	defer cancel()
	live, err := exportLiveMetadata(ctx, cfg)
	if err != nil {
		return []CheckResult{vectorSearchSkip(fmt.Sprintf("live Hasura metadata unreadable (is the stack running?): %v", err))}
	}
	return vectorSearchResults(live)
}

// vectorSearchResults turns an export_metadata document into check results.
func vectorSearchResults(live []byte) []CheckResult {
	findings, seen, err := vectorsearch.AuditLive(live)
	if err != nil {
		return []CheckResult{vectorSearchSkip(err.Error())}
	}
	if seen == 0 {
		return nil
	}
	if len(findings) == 0 {
		return []CheckResult{{Section: "hasura", Name: "VECTOR-SEARCH", Status: "pass",
			Message: fmt.Sprintf("%d tracked vector-search function(s): every select role sees the vector column and the root select field", seen)}}
	}
	results := make([]CheckResult, 0, len(findings))
	for _, f := range findings {
		results = append(results, CheckResult{Section: "hasura", Name: "VECTOR-SEARCH-" + f.Function, Status: "fail",
			Message: fmt.Sprintf("E514: %s orders %s by %s, and Hasura exposes it to every role with select on the table: %s. These roles could learn hidden data from the result order",
				f.Function, f.Table, f.Column, strings.Join(f.Problems, "; ")),
			FixCmd: "give those roles the vector column and root select, or remove their select permission on " + f.Table + "; then run nself db generate vector-search --track again"})
	}
	return results
}

func vectorSearchSkip(why string) CheckResult {
	return CheckResult{Section: "hasura", Name: "VECTOR-SEARCH", Status: "skip", Message: "vector-search permissions not checked: " + why}
}
