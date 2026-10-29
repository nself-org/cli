package repoqa

import "fmt"

// acceptedBasis pins complete entries, including pending state. A fixture-only
// addition, replacement, duplicate, or pending tag cannot widen the guard.
// Changes require an explicit test-code review against overview section 4.2.
var acceptedBasis = acceptedEntries([]edgeEntry{
	{From: "internal/access", To: "internal/hostpin"},
	{From: "internal/admin", To: "internal/hostpin"},
	{From: "internal/exporter", To: "internal/database"},
	{From: "internal/upgrade", To: "internal/build"},
	{From: "internal/upgrade", To: "internal/backup"},
	{From: "internal/upgrade", To: "internal/database"},
	{From: "internal/upgrade", To: "internal/reconcile"},
	{From: "internal/upgrade", To: "internal/plugin"},
	{From: "internal/importer", To: "internal/database"},
	{From: "internal/backup", To: "internal/controlplane"},
	{From: "internal/build", To: "internal/bundle"},
	{From: "internal/build", To: "internal/plugin"},
	{From: "internal/bundle", To: "internal/plugin"},
	{From: "internal/controlplane", To: "internal/deploy"},
	{From: "internal/database", To: "internal/backup"},
	{From: "internal/deploy", To: "internal/health"},
	{From: "internal/doctor", To: "internal/database"},
	{From: "internal/doctor", To: "internal/health"},
	{From: "internal/doctor", To: "internal/plugin"},
	{From: "internal/hasura", To: "internal/database"},
	{From: "internal/runner", To: "internal/deploy"},
	{From: "internal/trust", To: "internal/ssl"},
	{From: "internal/ssl", To: "internal/compose"},
	{From: "internal/reconcile", To: "internal/build"},
	{From: "internal/builtin", To: "internal/httpapi"},
	{From: "internal/httpapi", To: "internal/invoke"},
	{From: "internal/upgrade", To: "internal/lock"},
	{From: "internal/upgrade", To: "internal/bundle"},
	{From: "internal/mcpgen", To: "internal/invoke"},
})

func acceptedEntries(entries []edgeEntry) map[edgeEntry]bool {
	out := make(map[edgeEntry]bool, len(entries))
	for _, e := range entries {
		out[e] = true
	}
	return out
}

func checkAcceptedEdges(entries []edgeEntry, basis map[edgeEntry]bool) []string {
	var out []string
	seen := map[edgeEntry]bool{}
	for _, e := range entries {
		if !basis[e] || seen[e] {
			out = append(out, fmt.Sprintf("acceptedBasis: unapproved or duplicate edge %+v; fixture edits cannot widen accepted edges", e))
		}
		seen[e] = true
	}
	for e := range basis {
		if !seen[e] {
			out = append(out, fmt.Sprintf("acceptedBasis: missing edge %+v; remove its basis entry after architecture review", e))
		}
	}
	return out
}
