package imageswitch

// compare.go — decide whether two captures are the same cluster.
//
// Purpose: verify (dump-time capture of the old cluster against the restored
// cluster) and recheck (the same capture against the old cluster again) use one
// comparison. Equal means: the cluster document, every database's capture
// (table counts and hashes, sequence values, extensions), every schema-only
// dump and the roles dump are the same.
// Inputs: stage file suffixes. Outputs: a list of differences, empty when equal.
// Constraints: a difference names the thing and the line number, never a row, a
// password hash or a statement of the roles dump.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	// dropLine matches the lines pg_dump and pg_dumpall write that vary between runs or
	// servers: comments, session settings, and the random \restrict token of recent minors.
	dropLine = regexp.MustCompile(`^(--|SET |SELECT pg_catalog\.set_config|\\restrict|\\unrestrict)`)
)

// normalizeSQL removes the lines that legitimately differ between two dumps of one schema.
func normalizeSQL(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimRight(l, " \t\r")
		if l == "" || dropLine.MatchString(l) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// firstDiff returns the 1-based index of the first differing line of a and b, 0 when equal.
func firstDiff(a, b []string) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		if i >= len(a) || i >= len(b) || a[i] != b[i] {
			return i + 1
		}
	}
	return 0
}

// capDoc is the per-database capture.
type capDoc struct {
	Encoding string `json:"encoding"`
	Tables   []struct {
		Table string `json:"table"`
		Count string `json:"count"`
		Hash  string `json:"hash"`
	} `json:"tables"`
	LargeObjs  []any   `json:"large_objects"`
	Sequences  [][]any `json:"sequences"`
	Extensions [][]any `json:"extensions"`
}

// compareSets compares the stage files with suffix a against those with suffix b
// ("old" against "new" for verify, "old" against "chk" for recheck).
func (s *Switch) compareSets(a, b string) ([]string, error) {
	var diffs []string
	ca, err := s.readStage("cluster." + a + ".json")
	if err != nil {
		return nil, err
	}
	cb, err := s.readStage("cluster." + b + ".json")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(ca) != strings.TrimSpace(cb) {
		diffs = append(diffs, "roles, memberships, database settings or ACLs differ")
	}
	ga, err := s.readStage("globals." + a + ".sql")
	if err != nil {
		return nil, err
	}
	gb, err := s.readStage("globals." + b + ".sql")
	if err != nil {
		return nil, err
	}
	if n := firstDiff(normalizeSQL(ga), normalizeSQL(gb)); n > 0 {
		diffs = append(diffs, fmt.Sprintf("the roles dump differs (normalized line %d)", n))
	}
	for i, db := range s.st.Databases {
		d, err := s.compareDatabase(i, db, a, b)
		if err != nil {
			return nil, err
		}
		diffs = append(diffs, d...)
	}
	return diffs, nil
}

// compareDatabase compares one database's capture and schema dump.
func (s *Switch) compareDatabase(i int, db, a, b string) ([]string, error) {
	var diffs []string
	ca, err := s.readStage(dbFile(i, "cap."+a+".json"))
	if err != nil {
		return nil, err
	}
	cb, err := s.readStage(dbFile(i, "cap."+b+".json"))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(ca) != strings.TrimSpace(cb) {
		diffs = append(diffs, describeCapDiff(db, ca, cb)...)
	}
	sa, err := s.readStage(dbFile(i, "schema."+a+".sql"))
	if err != nil {
		return nil, err
	}
	sb, err := s.readStage(dbFile(i, "schema."+b+".sql"))
	if err != nil {
		return nil, err
	}
	if n := firstDiff(normalizeSQL(sa), normalizeSQL(sb)); n > 0 {
		diffs = append(diffs, fmt.Sprintf("%s: the schema, owners, grants, default privileges or policies differ (normalized line %d)", db, n))
	}
	return diffs, nil
}

// describeCapDiff names what differs between two captures of one database.
func describeCapDiff(db, a, b string) []string {
	var da, dbb capDoc
	if json.Unmarshal([]byte(a), &da) != nil || json.Unmarshal([]byte(b), &dbb) != nil {
		return []string{db + ": the capture is unreadable"}
	}
	var out []string
	seen := map[string]bool{}
	bt := map[string][2]string{}
	for _, t := range dbb.Tables {
		bt[t.Table] = [2]string{t.Count, t.Hash}
	}
	for _, t := range da.Tables {
		seen[t.Table] = true
		got, ok := bt[t.Table]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("%s: table %s is missing", db, t.Table))
		case got[0] != t.Count:
			out = append(out, fmt.Sprintf("%s: table %s has %s rows, expected %s", db, t.Table, got[0], t.Count))
		case got[1] != t.Hash:
			out = append(out, fmt.Sprintf("%s: table %s has the same row count but different content", db, t.Table))
		}
	}
	for _, t := range dbb.Tables {
		if !seen[t.Table] {
			out = append(out, fmt.Sprintf("%s: table %s is new", db, t.Table))
		}
	}
	if fmt.Sprint(da.LargeObjs) != fmt.Sprint(dbb.LargeObjs) {
		out = append(out, db+": large objects differ")
	}
	if fmt.Sprint(da.Sequences) != fmt.Sprint(dbb.Sequences) {
		out = append(out, db+": sequence values differ")
	}
	if fmt.Sprint(da.Extensions) != fmt.Sprint(dbb.Extensions) {
		out = append(out, db+": extensions or their versions differ")
	}
	if len(out) == 0 {
		out = append(out, db+": the capture differs")
	}
	return out
}

// summarize counts tables and sequences over the dump-time captures.
func (s *Switch) summarize() (tables, sequences int) {
	for i := range s.st.Databases {
		raw, err := s.readStage(dbFile(i, "cap.old.json"))
		if err != nil {
			continue
		}
		var d capDoc
		if json.Unmarshal([]byte(raw), &d) == nil {
			tables += len(d.Tables)
			sequences += len(d.Sequences)
		}
	}
	return tables, sequences
}
