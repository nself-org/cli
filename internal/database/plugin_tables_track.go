package database

// Purpose: track every installed plugin's manifest `tables` in Hasura after the
// plugin applied its own migrations at boot (P7-PLUG-78, EPIC D17).
// Inputs: context, *config.Config, the plugins' manifest tables as written, and
// the export_metadata parser (internal/database may not import
// internal/bundle/appready: layers.yaml has no such edge, so cmd injects
// appready.TrackedTables as TrackedSetFunc, which keeps one parser).
// Outputs: a TrackResult (qualified schema.table names), or an error.
// Constraints:
//   - One export_metadata read; a non-2xx reply (401/403 included) or a body
//     that is not JSON is an error, never "nothing to track".
//   - Every untracked table is checked to exist in Postgres BEFORE any write; a
//     missing table fails the whole call and no track request is sent.
//   - The only write is one `bulk` request on source `default` made of
//     pg_track_table ops, plus at most one retry bulk after a fresh export when
//     Hasura reports a concurrent change (already-tracked and friends). Never
//     replace_metadata, an untrack, a relationship or a permission op (see
//     metadata_reconcile.go for the outage that causes).
//   - bulk is treated as non-atomic: a rerun tracks only what is still missing.
//   - The admin secret comes from the running Hasura container (the way
//     `nself db reconcile` reads it) and is never put in an error string.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// PluginTables is one plugin's manifest `tables`, entries as written: "x" is
// public.x and "s.x" is schema s.
type PluginTables struct {
	Plugin string
	Tables []string
}

// TrackResult lists qualified "schema.table" names, sorted. Already were
// tracked at the first export. Tracked are the tables that were untracked then:
// they are all tracked now, by this call or, after a concurrent metadata write
// and one retry, partly by another writer.
type TrackResult struct {
	Tracked []string
	Already []string
}

// TrackedSetFunc parses an export_metadata body into the set of tracked
// "schema.name" tables. cmd injects appready.TrackedTables.
type TrackedSetFunc func(raw []byte) (map[string]bool, error)

// TrackError says which plugin and table a TrackPluginTables failure is about.
// Plugin and Table are empty when the failure is not about one table.
type TrackError struct {
	Plugin string
	Table  string
	Err    error
}

func (e *TrackError) Error() string { return e.Err.Error() }
func (e *TrackError) Unwrap() error { return e.Err }

// Seams for unit tests: no Docker and no Postgres.
var (
	trackReadSecret  = readHasuraAdminSecretFromContainer
	trackTableExists = postgresTableExists
)

// QualifyPluginTable is the manifest-table rule: "x" is "public.x", "s.x" stays.
// It must match appready.Missing (a test in cmd/commands pins that).
func QualifyPluginTable(t string) string {
	if strings.Contains(t, ".") {
		return t
	}
	return "public." + t
}

// TrackPluginTables tracks the manifest tables of plugins that Hasura does not
// track yet, with one targeted bulk request of pg_track_table ops. Tables that
// are already tracked cost nothing: with none untracked it sends no request
// after the export. A manifest table missing from Postgres, an unreadable
// export, or a Hasura rejection is an error (a *TrackError naming the plugin
// and table where known); nothing is ever untracked.
func TrackPluginTables(ctx context.Context, cfg *config.Config, plugins []PluginTables, parse TrackedSetFunc) (TrackResult, error) {
	secret, err := trackReadSecret(ctx, cfg)
	if err != nil {
		return TrackResult{}, fmt.Errorf("track plugin tables: %w", err)
	}
	tracked, err := readTrackedSet(ctx, cfg, secret, parse)
	if err != nil {
		return TrackResult{}, err
	}
	pending, already := splitUntracked(plugins, tracked)
	if len(pending) == 0 {
		return TrackResult{Already: already}, nil
	}
	if err := checkTablesExist(ctx, cfg, pending); err != nil {
		return TrackResult{}, err
	}
	if err := trackBulk(ctx, cfg, secret, pending); err != nil {
		if !isConcurrentChange(err) {
			return TrackResult{}, bulkRejection(err, pending)
		}
		// Another writer changed the metadata between our export and the bulk.
		// Re-read once and send one more bulk for what is still untracked. The
		// retry covers only the first pending list, so no second existence
		// check: every table in it was checked above.
		fresh, rerr := readTrackedSet(ctx, cfg, secret, parse)
		if rerr != nil {
			return TrackResult{}, rerr
		}
		var retry []pendingTable
		for _, p := range pending {
			if !fresh[p.table] {
				retry = append(retry, p)
			}
		}
		if len(retry) > 0 {
			if err := trackBulk(ctx, cfg, secret, retry); err != nil {
				return TrackResult{}, bulkRejection(err, retry)
			}
		}
	}
	out := TrackResult{Already: already}
	for _, p := range pending {
		out.Tracked = append(out.Tracked, p.table)
	}
	sort.Strings(out.Tracked)
	return out, nil
}

// readTrackedSet reads export_metadata and parses it into the tracked set.
func readTrackedSet(ctx context.Context, cfg *config.Config, secret string, parse TrackedSetFunc) (map[string]bool, error) {
	body, err := postMetadataAdmin(ctx, cfg, secret, "export_metadata", struct{}{})
	if err != nil {
		return nil, fmt.Errorf("track plugin tables: export metadata: %w", err)
	}
	tracked, err := parse(body)
	if err != nil {
		return nil, fmt.Errorf("track plugin tables: %w", err)
	}
	return tracked, nil
}

// trackBulk sends one bulk of pg_track_table ops on source default.
func trackBulk(ctx context.Context, cfg *config.Config, secret string, pending []pendingTable) error {
	ops := make([]map[string]interface{}, 0, len(pending))
	for _, p := range pending {
		schema, name := splitQualified(p.table)
		ops = append(ops, map[string]interface{}{
			"type": "pg_track_table",
			"args": map[string]interface{}{
				"source": "default",
				"table":  map[string]interface{}{"schema": schema, "name": name},
			},
		})
	}
	_, err := postMetadataAdmin(ctx, cfg, secret, "bulk", ops)
	return err
}

// pendingTable is one untracked qualified table and the plugin that declares it.
type pendingTable struct{ plugin, table string }

// splitUntracked returns the untracked tables (first declaring plugin wins, in
// sorted table order) and the sorted already-tracked tables, both deduplicated.
func splitUntracked(plugins []PluginTables, tracked map[string]bool) ([]pendingTable, []string) {
	seen := map[string]bool{}
	var pending []pendingTable
	var already []string
	for _, pl := range plugins {
		for _, t := range pl.Tables {
			q := QualifyPluginTable(t)
			if seen[q] {
				continue
			}
			seen[q] = true
			if tracked[q] {
				already = append(already, q)
			} else {
				pending = append(pending, pendingTable{pl.Plugin, q})
			}
		}
	}
	sort.Strings(already)
	sort.Slice(pending, func(i, j int) bool { return pending[i].table < pending[j].table })
	return pending, already
}

// checkTablesExist fails, naming every missing plugin table, before any write.
func checkTablesExist(ctx context.Context, cfg *config.Config, pending []pendingTable) error {
	var missing []string
	var first *pendingTable
	for i, p := range pending {
		schema, name := splitQualified(p.table)
		ok, err := trackTableExists(ctx, cfg, schema, name)
		if err != nil {
			return &TrackError{Plugin: p.plugin, Table: p.table, Err: fmt.Errorf("plugin %s: cannot check that table %s exists in Postgres: %w", p.plugin, p.table, err)}
		}
		if !ok {
			missing = append(missing, fmt.Sprintf("plugin %s: table %s does not exist in Postgres", p.plugin, p.table))
			if first == nil {
				first = &pending[i]
			}
		}
	}
	if first == nil {
		return nil
	}
	return &TrackError{Plugin: first.plugin, Table: first.table, Err: fmt.Errorf("%s (nothing was tracked; the plugin's boot migrations did not create it)", strings.Join(missing, "; "))}
}

func splitQualified(q string) (schema, name string) {
	i := strings.Index(q, ".")
	return q[:i], q[i+1:]
}

var bulkPathRE = regexp.MustCompile(`\$\.args\[(\d+)\]`)

// hasuraReply is the JSON error body Hasura sends on a rejected metadata call.
type hasuraReply struct {
	Path  string `json:"path"`
	Error string `json:"error"`
	Code  string `json:"code"`
}

// parseHasuraReply extracts the JSON body that postMetadataAdmin puts after the
// first "{" in its error text. ok is false when there is no parsable body.
func parseHasuraReply(err error) (hasuraReply, bool) {
	msg := err.Error()
	i := strings.Index(msg, "{")
	if i < 0 {
		return hasuraReply{}, false
	}
	var reply hasuraReply
	if json.Unmarshal([]byte(msg[i:]), &reply) != nil {
		return hasuraReply{}, false
	}
	return reply, true
}

// isConcurrentChange reports whether a bulk error means another writer changed
// the metadata after our export (the one case worth a single retry).
func isConcurrentChange(err error) bool {
	reply, ok := parseHasuraReply(err)
	if !ok {
		return false
	}
	switch reply.Code {
	case "already-tracked", "already-exists", "conflict", "inconsistent-metadata":
		return true
	}
	return false
}

// bulkRejection names the table Hasura rejected, from the error path
// ("$.args[N]") of the bulk reply, when the reply carries one.
func bulkRejection(err error, pending []pendingTable) error {
	if reply, ok := parseHasuraReply(err); ok {
		if m := bulkPathRE.FindStringSubmatch(reply.Path); m != nil {
			if n, cerr := strconv.Atoi(m[1]); cerr == nil && n < len(pending) {
				p := pending[n]
				return &TrackError{Plugin: p.plugin, Table: p.table, Err: fmt.Errorf("plugin %s: hasura rejected tracking %s (%s); tables before it in the request may already be tracked, rerun to track the rest: %w", p.plugin, p.table, reply.Error, err)}
			}
		}
	}
	return &TrackError{Err: fmt.Errorf("hasura rejected the track request: %w", err)}
}
