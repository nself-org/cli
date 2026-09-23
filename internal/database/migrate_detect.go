package database

import (
	"regexp"
	"strings"
)

// Purpose: parse migration SQL for the schema objects it creates, and
// classify a migration against a live-catalog presence map. Pure text/data
// functions only — see migrate_detect_query.go for the live pg_catalog query
// that supplies the presence map DetectMigrations uses.
// Inputs: raw SQL text (extraction) or an ObjectRef list + presence map
// (classification).
// Outputs: []ObjectRef, or (DetectClass, present, missing).
// Constraints: extraction is best-effort regex, not a SQL parser — it is
// intentionally conservative (may under-detect quoted/computed identifiers)
// but must never fabricate an object that isn't in the text. Classification
// is honest by construction: BASELINE requires every object present, APPLY
// requires none present; anything else is CONFLICT, never silently resolved.

// dollarQuoteTagRe matches a dollar-quote opening/closing delimiter: "$$" or
// a tagged form like "$body$". Used by stripDollarQuotedBodies to find each
// delimiter's extent; RE2 (Go's regexp engine) has no backreferences, so the
// matching close delimiter for a given open tag is found with strings.Index,
// not a second regex.
var dollarQuoteTagRe = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*\$|\$\$`)

// stripDollarQuotedBodies blanks the contents of every dollar-quoted string
// (DO $$ ... $$ blocks, function bodies) while preserving newlines and byte
// offsets, so line numbers stay meaningful and the surrounding SQL is
// untouched. The delimiters themselves are left in place (they never match
// an object-reference regex).
//
// WHY: a DO block's ALTER/CREATE statements normally run inside a runtime
// existence check ("IF NOT EXISTS (SELECT ...) THEN ALTER TABLE ...") that
// the static regex extractor cannot evaluate. Left unstripped, an ALTER
// TABLE inside such a guard was extracted as an unconditional prerequisite,
// producing a refusal for a migration that would have skipped that ALTER
// safely at runtime. Stripping the whole dollar-quoted body removes it from
// static scanning entirely — a table referenced only inside a DO block is
// never treated as a hard prerequisite.
func stripDollarQuotedBodies(sql string) string {
	var out strings.Builder
	out.Grow(len(sql))

	i := 0
	for i < len(sql) {
		loc := dollarQuoteTagRe.FindStringIndex(sql[i:])
		if loc == nil {
			out.WriteString(sql[i:])
			break
		}
		tagStart, tagEnd := i+loc[0], i+loc[1]
		tag := sql[tagStart:tagEnd]

		closeRel := strings.Index(sql[tagEnd:], tag)
		if closeRel < 0 {
			// Unterminated dollar-quote (malformed SQL, or just the end of a
			// truncated fragment in a test) — emit the rest untouched rather
			// than blanking real content on a guess.
			out.WriteString(sql[i:])
			break
		}
		bodyStart, bodyEnd := tagEnd, tagEnd+closeRel

		out.WriteString(sql[i:tagStart])
		out.WriteString(tag)
		out.WriteString(blankPreservingNewlines(sql[bodyStart:bodyEnd]))
		out.WriteString(tag)
		i = bodyEnd + len(tag)
	}
	return out.String()
}

// blankPreservingNewlines replaces every non-newline byte with a space,
// keeping newline positions so line-numbered error messages built from the
// cleaned text still line up with the original file.
func blankPreservingNewlines(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c != '\n' {
			b[i] = ' '
		}
	}
	return string(b)
}

// cleanSQLForObjectScan removes comments and dollar-quoted bodies before any
// object-reference regex (CREATE/ALTER extraction) runs over sqlContent.
// Without this, commentary text ("-- ALTER TABLE\n--   ENABLE ...") can be
// misread as a real statement, and conditionally-guarded DDL inside a DO
// block is misread as an unconditional prerequisite. Shared by
// ExtractCreatedObjects and ExtractAlteredObjects so both extractors agree
// on what counts as "real" SQL.
func cleanSQLForObjectScan(sqlContent string) string {
	return stripDollarQuotedBodies(stripSQLComments(sqlContent))
}

// ObjectKind identifies the kind of schema object a CREATE statement makes.
type ObjectKind string

const (
	ObjectTable      ObjectKind = "table"
	ObjectIndex      ObjectKind = "index"
	ObjectType       ObjectKind = "type"
	ObjectView       ObjectKind = "view"
	ObjectMatView    ObjectKind = "materialized view"
	ObjectSequence   ObjectKind = "sequence"
	ObjectSchemaKind ObjectKind = "schema"
)

// ObjectRef is one schema object a migration file creates.
type ObjectRef struct {
	Kind ObjectKind
	Name string // as written in the SQL; may be schema-qualified and/or quoted
}

// Key returns the unique map key used to dedupe/look up presence for o.
// Uses normalizeIdentifier so "public.np_waitlist", "np_waitlist", and
// `"public"."np_waitlist"` all collapse to the same key — otherwise a CREATE
// in one migration file and an ALTER on the same table in another, written
// with different (but equivalent) qualification, would be treated as two
// different objects and the ALTER would be falsely refused as missing its
// prerequisite.
func (o ObjectRef) Key() string {
	return string(o.Kind) + ":" + normalizeIdentifier(o.Name)
}

// identPart matches one identifier segment: either a double-quoted,
// case-preserving identifier ("Np_Waitlist") or a plain unquoted one.
const identPart = `(?:"[^"]+"|[A-Za-z_]\w*)`

// qualifiedIdent matches a table/view/type/sequence name that may carry one
// schema qualifier, each segment optionally quoted: np_waitlist,
// public.np_waitlist, "public"."np_waitlist", app."Tasks".
const qualifiedIdent = identPart + `(?:\.` + identPart + `)?`

var (
	createTableRe   = regexp.MustCompile(`(?i)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(` + qualifiedIdent + `)`)
	createIndexRe   = regexp.MustCompile(`(?i)\bCREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:CONCURRENTLY\s+)?(?:IF\s+NOT\s+EXISTS\s+)?(` + identPart + `)`)
	createTypeRe    = regexp.MustCompile(`(?i)\bCREATE\s+TYPE\s+(` + qualifiedIdent + `)`)
	createMatViewRe = regexp.MustCompile(`(?i)\bCREATE\s+MATERIALIZED\s+VIEW\s+(?:IF\s+NOT\s+EXISTS\s+)?(` + qualifiedIdent + `)`)
	createViewRe    = regexp.MustCompile(`(?i)\bCREATE\s+(?:OR\s+REPLACE\s+)?VIEW\s+(` + qualifiedIdent + `)`)
	createSeqRe     = regexp.MustCompile(`(?i)\bCREATE\s+SEQUENCE\s+(?:IF\s+NOT\s+EXISTS\s+)?(` + qualifiedIdent + `)`)
	createSchemaRe  = regexp.MustCompile(`(?i)\bCREATE\s+SCHEMA\s+(?:IF\s+NOT\s+EXISTS\s+)?(` + identPart + `)`)
)

// ExtractCreatedObjects returns every schema object sqlContent creates.
// Scans a comment- and dollar-quoted-body-stripped copy (see
// cleanSQLForObjectScan) so commentary text and conditionally-guarded DDL
// inside a DO block are never mistaken for a real, unconditional CREATE.
func ExtractCreatedObjects(sqlContent string) []ObjectRef {
	cleaned := cleanSQLForObjectScan(sqlContent)
	var out []ObjectRef
	collect := func(kind ObjectKind, re *regexp.Regexp) {
		for _, m := range re.FindAllStringSubmatch(cleaned, -1) {
			out = append(out, ObjectRef{Kind: kind, Name: m[1]})
		}
	}
	collect(ObjectTable, createTableRe)
	collect(ObjectIndex, createIndexRe)
	collect(ObjectType, createTypeRe)
	collect(ObjectMatView, createMatViewRe)
	collect(ObjectView, createViewRe)
	collect(ObjectSequence, createSeqRe)
	collect(ObjectSchemaKind, createSchemaRe)
	return out
}

// normalizeIdentifier puts a possibly schema-qualified, possibly quoted SQL
// identifier into one canonical form so two spellings of the same object
// compare equal:
//   - each dot-separated segment has surrounding double quotes stripped
//     (a quoted segment is case-sensitive in Postgres, so its case is kept
//     as written);
//   - an unquoted segment is lower-cased (Postgres folds unquoted
//     identifiers to lower case at parse time, so "Foo" and "foo" already
//     name the same object);
//   - a leading "public." schema qualifier is dropped, since that is
//     Postgres's default schema and the same table is routinely written
//     both as "np_waitlist" and "public.np_waitlist" across this codebase's
//     migrations. Any other schema qualifier is kept — "app.tasks" and
//     "tasks" are NOT the same object.
func normalizeIdentifier(raw string) string {
	parts := splitQualifiedIdentifier(strings.TrimSpace(raw))
	for i, p := range parts {
		parts[i] = normalizeIdentifierPart(p)
	}
	if len(parts) == 2 && parts[0] == "public" {
		return parts[1]
	}
	return strings.Join(parts, ".")
}

// splitQualifiedIdentifier splits s on '.' at the top level only — a '.'
// inside a double-quoted segment does not split, since Postgres allows one
// there (a rare but legal identifier like "my.table").
func splitQualifiedIdentifier(s string) []string {
	var parts []string
	var cur strings.Builder
	inQuotes := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			cur.WriteRune(r)
		case r == '.' && !inQuotes:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	parts = append(parts, cur.String())
	return parts
}

// normalizeIdentifierPart strips surrounding double quotes from one segment
// (preserving its case) or lower-cases it if unquoted.
func normalizeIdentifierPart(p string) string {
	p = strings.TrimSpace(p)
	if len(p) >= 2 && strings.HasPrefix(p, `"`) && strings.HasSuffix(p, `"`) {
		return p[1 : len(p)-1]
	}
	return strings.ToLower(p)
}

// DetectClass is the honest three-way (plus unknown) classification of a
// pending migration against the live schema.
type DetectClass string

const (
	DetectApply    DetectClass = "APPLY"
	DetectBaseline DetectClass = "BASELINE"
	DetectConflict DetectClass = "CONFLICT"
	DetectUnknown  DetectClass = "UNKNOWN"
)

// ClassifyByPresence classifies a migration by how many of the objects it
// creates already exist (existing, keyed by ObjectRef.Key()). BASELINE is
// only reachable when every object is present and APPLY only when none
// are — any partial match is CONFLICT, which callers must never
// auto-resolve. UNKNOWN means extraction found no objects to check (e.g. a
// data-only migration) — callers should fall back to ledger-only status.
func ClassifyByPresence(objects []ObjectRef, existing map[string]bool) (class DetectClass, present, missing []ObjectRef) {
	if len(objects) == 0 {
		return DetectUnknown, nil, nil
	}
	for _, o := range objects {
		if existing[o.Key()] {
			present = append(present, o)
		} else {
			missing = append(missing, o)
		}
	}
	if len(missing) == 0 {
		return DetectBaseline, present, missing
	}
	if len(present) == 0 {
		return DetectApply, present, missing
	}
	return DetectConflict, present, missing
}
