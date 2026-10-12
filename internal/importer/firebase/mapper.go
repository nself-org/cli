package firebase

// Purpose: map collected Firestore tables to Postgres tables per EPIC D11: schema "firebase", one
// table per collection (sub-collections as "<parent>__<sub>"), id TEXT, data JSONB, plus a typed
// column for each top-level scalar field whose type agrees in every document.
// Inputs: collected tables. Outputs: DDL, COPY spools and exemption notes for what was not
// promoted. Constraints: a conflict is reported and the value stays in data; nothing is guessed.
import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/importer/scratch"
	"github.com/nself-org/cli/internal/portable"
)

const (
	schemaName = "firebase"
	// maxTypedColumns bounds the promoted columns per table (Postgres stops at 1600).
	maxTypedColumns = 200
)

var (
	badIdent = regexp.MustCompile(`[^a-z0-9_]`)
	// reserved are the columns every mapped table owns.
	reserved = map[string]bool{"id": true, "data": true, "parent_id": true, "parent_path": true}
	sqlTypes = map[kind]string{kString: "text", kInteger: "bigint", kDouble: "double precision", kBoolean: "boolean", kTimestamp: "timestamptz"}
)

type column struct {
	Field, Name string
	K           kind
}

// sanitize maps a Firestore name to ^[a-z_][a-z0-9_]*$.
func sanitize(s string) string {
	s = badIdent.ReplaceAllString(strings.ToLower(s), "_")
	if s == "" || (s[0] >= '0' && s[0] <= '9') {
		s = "_" + s
	}
	return s
}

// fit shortens a name to 63 bytes with a stable hash suffix.
func fit(name, key string) string {
	if len(name) <= 63 {
		return name
	}
	sum := sha256.Sum256([]byte(key))
	return name[:54] + "_" + hex.EncodeToString(sum[:4])
}

// assignNames sets every table name; the first pattern in sorted order keeps a contested name,
// later ones get _2, _3 ... and an exemption note says so.
func assignNames(tables []*tableAcc) []portable.Exemption {
	used := map[string]bool{}
	var notes []portable.Exemption
	for _, t := range tables {
		id := t.pattern[strings.LastIndex(t.pattern, "/")+1:]
		base := sanitize(id)
		if t.parent != nil {
			base = t.parent.name + "__" + base
		}
		name := fit(base, t.pattern)
		// The counter is part of the hash key: a long base is cut to the same 54 bytes for every
		// n, and only the hash suffix tells the candidates apart.
		for n := 2; used[name]; n++ {
			name = fit(fmt.Sprintf("%s_%d", base, n), t.pattern+"#"+strconv.Itoa(n))
		}
		used[name] = true
		t.name = name
		if name != base {
			notes = append(notes, portable.Exemption{Kind: "metadata", ID: schemaName + "." + name + "#renamed", Reason: "collection " + t.pattern + " renamed (name collision or length)"})
		}
	}
	return notes
}

// columnsFor picks the promoted columns and reports every conflict.
func columnsFor(t *tableAcc) ([]column, []portable.Exemption) {
	var cols []column
	var notes []portable.Exemption
	groups := map[string][]column{}
	fields := make([]string, 0, len(t.kinds))
	for f := range t.kinds {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	note := func(field, reason string) {
		notes = append(notes, portable.Exemption{Kind: "metadata", ID: schemaName + "." + t.name + "." + field, Reason: reason})
	}
	for _, f := range fields {
		var seen []string
		for k := range t.kinds[f] {
			if k != kNull {
				seen = append(seen, string(k))
			}
		}
		sort.Strings(seen)
		if len(seen) > 1 {
			note(f, "field types differ ("+strings.Join(seen, ",")+"); kept in data only")
			continue
		}
		if len(seen) == 1 && promotable(kind(seen[0])) && t.unsafe[f] {
			note(f, "a text value has a line psql cannot copy (backslash dot); kept in data only")
			continue
		}
		if len(seen) == 1 && promotable(kind(seen[0])) {
			c := column{Field: f, Name: fit(sanitize(f), f), K: kind(seen[0])}
			groups[c.Name] = append(groups[c.Name], c)
		}
	}
	names := make([]string, 0, len(groups))
	for n := range groups {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		g := groups[n]
		if len(g) > 1 || reserved[n] {
			for _, c := range g {
				note(c.Field, "column name collides after sanitising; kept in data only")
			}
			continue
		}
		cols = append(cols, g[0])
	}
	// Postgres allows 1600 columns; a collection keyed by date or uid at the top level can have
	// thousands of fields that each appear once. Keep the most common ones.
	if len(cols) > maxTypedColumns {
		sort.SliceStable(cols, func(i, j int) bool { return t.present[cols[i].Field] > t.present[cols[j].Field] })
		for _, c := range cols[maxTypedColumns:] {
			note(c.Field, fmt.Sprintf("more than %d typed columns; kept in data only", maxTypedColumns))
		}
		cols = cols[:maxTypedColumns]
		sort.Slice(cols, func(i, j int) bool { return cols[i].Name < cols[j].Name })
	}
	return cols, notes
}

// ddl returns the CREATE TABLE statement and the COPY column order of a table.
func ddl(t *tableAcc, cols []column) (string, []string, error) {
	q := func(s string) (string, error) { return database.SanitizeIdentifier(s) }
	schema, _ := q(schemaName)
	name, err := q(t.name)
	if err != nil {
		return "", nil, errors.New("mapped table name invalid")
	}
	order := []string{"id", "data"}
	defs := []string{`"id" text NOT NULL`, `"data" jsonb NOT NULL`}
	pk := `"id"`
	if t.parent != nil {
		order = append([]string{"parent_path", "parent_id"}, order...)
		defs = append([]string{`"parent_path" text NOT NULL`, `"parent_id" text NOT NULL`}, defs...)
		pk = `"parent_path", "id"`
	}
	for _, c := range cols {
		col, err := q(c.Name)
		if err != nil {
			return "", nil, errors.New("mapped column name invalid")
		}
		defs = append(defs, col+" "+sqlTypes[c.K])
		order = append(order, c.Name)
	}
	return fmt.Sprintf("CREATE TABLE %s.%s (\n  %s,\n  PRIMARY KEY (%s)\n);\n", schema, name, strings.Join(defs, ",\n  "), pk), order, nil
}

// csvField quotes a value so an empty string stays distinct from NULL (unquoted empty).
func csvField(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// writeCSV converts a table spool into the COPY file for its column order.
func writeCSV(t *tableAcc, cols []column, out string) error {
	in, err := os.Open(t.spool)
	if err != nil {
		return errors.New("firestore spool unreadable")
	}
	defer func() { _ = in.Close() }()
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("firestore COPY file unavailable")
	}
	w := bufio.NewWriterSize(f, 1<<20)
	dec := json.NewDecoder(in)
	dec.UseNumber()
	for {
		var r record
		if err = dec.Decode(&r); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			_ = f.Close()
			return errors.New("firestore spool invalid")
		}
		data, merr := json.Marshal(r.Data)
		if merr != nil {
			_ = f.Close()
			return errors.New("document JSON encoding failed")
		}
		fields := []string{csvField(r.ID), csvField(string(data))}
		if t.parent != nil {
			fields = append([]string{csvField(r.ParentPath), csvField(r.ParentID)}, fields...)
		}
		for _, c := range cols {
			if v, ok := r.Scalars[c.Field]; ok {
				fields = append(fields, csvField(v))
			} else {
				fields = append(fields, "")
			}
		}
		if _, err = w.WriteString(strings.Join(fields, ",") + "\n"); err != nil {
			_ = f.Close()
			return errors.New("firestore COPY write failed")
		}
	}
	if err = w.Flush(); err != nil {
		_ = f.Close()
		return errors.New("firestore COPY write failed")
	}
	return f.Close()
}

// emit builds the DDL script and COPY files for every table.
func emit(tables []*tableAcc, dir string) (string, []scratch.CopyFile, []portable.Exemption, error) {
	notes := assignNames(tables)
	var script strings.Builder
	script.WriteString("CREATE SCHEMA " + schemaName + ";\n")
	var copies []scratch.CopyFile
	for i, t := range tables {
		cols, n := columnsFor(t)
		notes = append(notes, n...)
		stmt, order, err := ddl(t, cols)
		if err != nil {
			return "", nil, nil, err
		}
		script.WriteString(stmt)
		path := filepath.Join(dir, fmt.Sprintf("copy-%d.csv", i))
		if err = writeCSV(t, cols, path); err != nil {
			return "", nil, nil, err
		}
		copies = append(copies, scratch.CopyFile{Schema: schemaName, Table: t.name, Path: path, Columns: order})
	}
	return script.String(), copies, notes, nil
}
