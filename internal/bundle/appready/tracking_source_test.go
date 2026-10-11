package appready

// Purpose: TrackedTables counts only the default Hasura source (D-0388).
// Inputs: small export_metadata JSON documents.
// Outputs: pass/fail.
// Constraints: no network; fixtures use fake table names.

import "testing"

func tableSet(t *testing.T, raw string) map[string]bool {
	t.Helper()
	set, err := TrackedTables([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// A table tracked only in a second source is not tracked for install.
func TestTrackedTablesIgnoresOtherSource(t *testing.T) {
	set := tableSet(t, `{"sources":[
	 {"name":"default","kind":"postgres","tables":[]},
	 {"name":"other","kind":"postgres","tables":[{"table":{"schema":"public","name":"np_x"}}]}]}`)
	if set["public.np_x"] || len(set) != 0 {
		t.Errorf("np_x tracked only on source other must not count: %v", set)
	}
}

// The same table tracked on default counts, even when another source has it too.
func TestTrackedTablesCountsDefaultSource(t *testing.T) {
	set := tableSet(t, `{"sources":[
	 {"name":"other","tables":[{"table":{"schema":"public","name":"np_y"}}]},
	 {"name":"default","kind":"postgres","tables":[{"table":{"schema":"public","name":"np_x"}}]}]}`)
	if !set["public.np_x"] || set["public.np_y"] {
		t.Errorf("want only public.np_x: %v", set)
	}
}

// A source with no name is hand-made or fixture metadata: it counts as default.
func TestTrackedTablesNamelessSourceIsDefault(t *testing.T) {
	set := tableSet(t, `{"sources":[{"tables":[{"table":{"schema":"public","name":"np_x"}}]}]}`)
	if !set["public.np_x"] {
		t.Errorf("a nameless source must count: %v", set)
	}
}

// A non-postgres source named default is still ignored (kind filter stays).
func TestTrackedTablesDefaultNamedNonPostgresIgnored(t *testing.T) {
	set := tableSet(t, `{"sources":[{"name":"default","kind":"bigquery","tables":[{"table":{"schema":"public","name":"np_x"}}]}]}`)
	if len(set) != 0 {
		t.Errorf("a bigquery source must be ignored even when named default: %v", set)
	}
}

// The legacy top-level tables array is single-source metadata, so it is default.
func TestTrackedTablesLegacyTopLevelIsDefault(t *testing.T) {
	set := tableSet(t, `{"tables":[{"table":{"schema":"public","name":"np_x"}}]}`)
	if !set["public.np_x"] {
		t.Errorf("legacy tables array must count: %v", set)
	}
}
