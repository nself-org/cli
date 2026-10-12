package migrate

// env_order.go — CLI-R18 migration shim: drift detection.
//
// Purpose: `nself migrate` must never silently change a project's resolved
//          config when the cascade order flips (legacy: .env.dev ->
//          .env.{staging|prod} -> .env.secrets -> .env.local -> .env ->
//          .env.ai; canonical: .env -> .env.{dev|staging|prod} ->
//          .env.secrets -> .env.local). DetectEnvOrder scans a project and
//          reports every variable whose winning file/value would differ
//          between the two orders, deciding for each whether it is safe to
//          auto-fix or must be flagged for manual review. The actual writes
//          happen in env_order_apply.go (Apply/Migrate).
// Inputs:  projectDir — the nSelf project root.
// Outputs: *EnvOrderReport — one entry per affected variable/env, before and
//          after effective values, and the intended action (or why not).
// Constraints: Read-only — DetectEnvOrder never touches the filesystem
//              beyond os.Stat/godotenv.Read. Deterministic iteration order
//              (sorted) so reports are diffable/testable.
// SPORT:   cli/internal/migrate — CLI-R18.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/nself-org/cli/internal/config"
)

// Action describes what the shim did (or intends to do) for one variable.
type Action string

const (
	// ActionFixed means the shim wrote OldValue into FixedFile so the
	// effective value survives the order flip unchanged.
	ActionFixed Action = "fixed"
	// ActionManualReview means the shim could not safely auto-resolve this
	// variable and left every file untouched — see Reason.
	ActionManualReview Action = "manual_review_required"
)

// EnvOrderVarChange describes one variable whose effective value would
// change (or already differs) between the legacy and canonical cascade
// orders for a given environment name.
type EnvOrderVarChange struct {
	EnvName   string // "dev", "staging", "prod"
	Var       string
	OldWinner string // file that won under the legacy order
	OldValue  string
	NewWinner string // file that would win under the canonical order, pre-fix
	NewValue  string
	Action    Action
	FixedFile string // set when Action == ActionFixed (always ".env.secrets")
	Reason    string // human-readable explanation, always set
}

// EnvOrderReport is the result of scanning (and optionally fixing) a project
// for CLI-R18 cascade-order drift.
type EnvOrderReport struct {
	ProjectDir string
	// NoChangeNeeded is true when every checked variable already resolves to
	// the same value under both orders — nothing to report or rewrite.
	NoChangeNeeded bool
	Changes        []EnvOrderVarChange
	// AIArchived is true when a legacy .env.ai file existed, every one of its
	// keys was folded (Action == ActionFixed), and it was renamed to
	// .env.ai.migrated.
	AIArchived bool
}

// FixedCount returns the number of changes the shim resolved automatically.
func (r *EnvOrderReport) FixedCount() int {
	n := 0
	for _, c := range r.Changes {
		if c.Action == ActionFixed {
			n++
		}
	}
	return n
}

// ManualReviewCount returns the number of changes left for the human to
// resolve.
func (r *EnvOrderReport) ManualReviewCount() int {
	n := 0
	for _, c := range r.Changes {
		if c.Action == ActionManualReview {
			n++
		}
	}
	return n
}

// knownEnvNames are the standard environment names checked for drift; a
// project only has variants for the env-specific files it actually created.
var knownEnvNames = []string{"dev", "staging", "prod"}

// DetectEnvOrder scans projectDir for every .env.{dev,staging,prod} variant
// present and reports, per variable, whether the legacy cascade order and
// the canonical order resolve it to different values. It makes no changes to
// any file — call Apply (env_order_apply.go) with the returned report to
// perform the safe fixes.
func DetectEnvOrder(projectDir string) (*EnvOrderReport, error) {
	report := &EnvOrderReport{ProjectDir: projectDir, NoChangeNeeded: true}

	values, err := readAllCascadeFiles(projectDir)
	if err != nil {
		return nil, err
	}

	for _, envName := range knownEnvNames {
		envFile := filepath.Join(projectDir, ".env."+envName)
		if _, err := os.Stat(envFile); err != nil {
			continue // this project never created this env variant
		}

		oldOrder := config.EnvCascadeOrder(envName, true)
		newOrder := config.EnvCascadeOrder(envName, false)

		for _, key := range unionKeys(values) {
			oldWinner, oldVal := winningFile(oldOrder, values, key)
			newWinner, newVal := winningFile(newOrder, values, key)
			if oldVal == newVal {
				continue // same effective value either way — not a drift
			}

			change := EnvOrderVarChange{
				EnvName:   envName,
				Var:       key,
				OldWinner: oldWinner,
				OldValue:  oldVal,
				NewWinner: newWinner,
				NewValue:  newVal,
			}

			switch {
			case oldWinner == ".env.dev" && envName != "dev":
				// The legacy order always loaded .env.dev as a base layer
				// regardless of the active environment — a bug in itself.
				// Baking that leaked dev-only value into .env.secrets for
				// staging/prod would just relocate the bug, not fix it.
				change.Action = ActionManualReview
				change.Reason = fmt.Sprintf(
					".env.dev was leaking %s=%q into the %s environment under the legacy cascade (a bug the reorder removes). "+
						"If %s genuinely needs this value, set it explicitly in .env.%s or .env.secrets.",
					key, oldVal, envName, envName, envName,
				)
			case newWinner == ".env.local":
				// .env.local's entire purpose is to be a personal, uncommitted
				// override. Under the legacy order it was incorrectly shadowed
				// by .env/.env.ai; the reorder fixes that. Forcing .env.local
				// back to the old value would silently defeat a developer's
				// intentional local override, which is worse than the bug
				// being fixed. Flag for human review instead of guessing.
				change.Action = ActionManualReview
				change.Reason = fmt.Sprintf(
					".env.local sets %s=%q, which the legacy cascade order was incorrectly ignoring in favor of %s=%q. "+
						"Under the new order your personal .env.local value now wins. Review whether that's what you want.",
					key, newVal, oldWinner, oldVal,
				)
			default:
				change.Action = ActionFixed
				change.FixedFile = ".env.secrets"
				change.Reason = fmt.Sprintf(
					"%s used to win (from %s=%q); .env.secrets now carries that value so it keeps winning under the new order.",
					key, oldWinner, oldVal,
				)
			}

			report.NoChangeNeeded = false
			report.Changes = append(report.Changes, change)
		}
	}

	sort.Slice(report.Changes, func(i, j int) bool {
		if report.Changes[i].EnvName != report.Changes[j].EnvName {
			return report.Changes[i].EnvName < report.Changes[j].EnvName
		}
		return report.Changes[i].Var < report.Changes[j].Var
	})

	return report, nil
}

// readAllCascadeFiles reads every filename that appears in either cascade
// order, across every environment, once. Returns a map keyed by filename
// (e.g. ".env.secrets") to that file's key/value pairs; files that don't
// exist or fail to parse are simply absent from the map.
func readAllCascadeFiles(projectDir string) (map[string]map[string]string, error) {
	values := make(map[string]map[string]string, len(config.AllCascadeFilenames))
	for _, name := range config.AllCascadeFilenames {
		path := filepath.Join(projectDir, name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		m, err := config.ReadEnvFile(path)
		if err != nil {
			return nil, config.EnvReadError(path, err)
		}
		values[name] = m
	}
	return values, nil
}

// unionKeys returns the sorted union of every key across every file in
// values, for deterministic iteration order.
func unionKeys(values map[string]map[string]string) []string {
	seen := make(map[string]bool)
	for _, m := range values {
		for k := range m {
			seen[k] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// winningFile scans order from the end (highest precedence) and returns the
// first (in scan order) file that defines key, i.e. the file that actually
// wins under that cascade order. Returns ("", "") if no file in order and
// values defines key.
func winningFile(order []string, values map[string]map[string]string, key string) (winner, val string) {
	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		m, ok := values[name]
		if !ok {
			continue
		}
		if v, ok := m[key]; ok {
			return name, v
		}
	}
	return "", ""
}
