package supabase

// Purpose: write db/compat.sql, the shims a plain Postgres target needs for objects the
// Supabase schema still references (extensions.* functions and auth.uid()).
// Inputs: the schema-only dump text. Outputs: SQL, the shim names and exemptions for
// references that have no shim. Constraints: pure; names come from the dump and are matched
// against fixed lists, never interpolated.
import (
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/portable"
)

// authUID reads the signed-in user's id from the session Hasura sets on every request.
const authUID = `CREATE OR REPLACE FUNCTION auth.uid() RETURNS uuid LANGUAGE sql STABLE SET search_path = pg_catalog AS $$ SELECT (nullif(current_setting('hasura.user', true), '')::json->>'x-hasura-user-id')::uuid $$;
`

// extensionShims are the extensions.* functions with a plain-Postgres equivalent.
var extensionShims = map[string]string{
	"uuid_generate_v4": "CREATE OR REPLACE FUNCTION extensions.uuid_generate_v4() RETURNS uuid LANGUAGE sql VOLATILE SET search_path = pg_catalog AS $$ SELECT pg_catalog.gen_random_uuid() $$;\n",
	"gen_random_uuid":  "CREATE OR REPLACE FUNCTION extensions.gen_random_uuid() RETURNS uuid LANGUAGE sql VOLATILE SET search_path = pg_catalog AS $$ SELECT pg_catalog.gen_random_uuid() $$;\n",
}

var (
	// Any extensions.<name> reference (function, type or operator): shimmed when a plain
	// equivalent exists, otherwise listed.
	extRef    = regexp.MustCompile(`(?i)\bextensions\.([a-z_][a-z0-9_]*)`)
	authUIDRe = regexp.MustCompile(`(?i)\bauth\.uid\s*\(`)
	// A trigger that calls a function of a schema the import does not move would stop the restore
	// half way, so it is refused before anything is written.
	foreignTrigger = regexp.MustCompile(`(?i)EXECUTE\s+(?:FUNCTION|PROCEDURE)\s+"?(supabase_functions|net|vault|pgsodium|realtime|graphql|graphql_public|storage|auth)"?\.`)
	authOther      = regexp.MustCompile(`(?i)\bauth\.(role|jwt|email)\s*\(`)
)

// Compat is the generated shim file and what it covers.
type Compat struct {
	SQL        string
	Shims      []string
	Exemptions []portable.Exemption
}

// BuildCompat scans the schema dump. SQL is empty when nothing is referenced.
func BuildCompat(schemaSQL string) Compat {
	var out Compat
	var body strings.Builder
	names := map[string]bool{}
	missing := map[string]bool{}
	for _, m := range extRef.FindAllStringSubmatch(schemaSQL, -1) {
		name := strings.ToLower(m[1])
		if _, ok := extensionShims[name]; ok {
			names[name] = true
		} else {
			missing["extensions."+name] = true
		}
	}
	if len(names) > 0 {
		body.WriteString("CREATE SCHEMA IF NOT EXISTS extensions;\n")
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	for _, n := range sorted {
		body.WriteString(extensionShims[n])
		out.Shims = append(out.Shims, "extensions."+n)
	}
	if authUIDRe.MatchString(schemaSQL) {
		body.WriteString(authUID)
		out.Shims = append(out.Shims, "auth.uid")
	}
	for _, m := range authOther.FindAllStringSubmatch(schemaSQL, -1) {
		missing["auth."+strings.ToLower(m[1])] = true
	}
	for name := range missing {
		out.Exemptions = append(out.Exemptions, portable.Exemption{Kind: "metadata", ID: "compat:" + name, Reason: "referenced by the schema, no shim: define it in the target before relying on it"})
	}
	sort.Slice(out.Exemptions, func(i, j int) bool { return out.Exemptions[i].ID < out.Exemptions[j].ID })
	out.SQL = body.String()
	return out
}

// CheckForeignTriggers refuses a schema whose triggers call functions of Supabase-owned schemas
// (for example database webhooks: supabase_functions.http_request).
func CheckForeignTriggers(schemaSQL string) error {
	if m := foreignTrigger.FindStringSubmatch(schemaSQL); m != nil {
		return errs.New("E520", "source has triggers that call Supabase-owned functions; drop or replace them in the source first")
	}
	return nil
}
