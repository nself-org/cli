package requires

import "strings"

// contrib is the set the official postgres images ship (PG 14-17 contrib).
var contrib = []string{"adminpack", "amcheck", "autoinc", "bloom", "btree_gin", "btree_gist", "citext", "cube",
	"dblink", "dict_int", "dict_xsyn", "earthdistance", "file_fdw", "fuzzystrmatch", "hstore", "insert_username",
	"intagg", "intarray", "isn", "lo", "ltree", "moddatetime", "pageinspect", "pg_buffercache", "pg_freespacemap",
	"pg_prewarm", "pg_stat_statements", "pg_surgery", "pg_trgm", "pg_visibility", "pg_walinspect", "pgcrypto",
	"pgrowlocks", "pgstattuple", "plpgsql", "postgres_fdw", "refint", "seg", "sslinfo", "tablefunc", "tcn",
	"tsm_system_rows", "tsm_system_time", "unaccent", "uuid-ossp", "xml2"}

// imageProvides returns the extensions an image ships and whether the image is
// one nself knows: pgvector/pgvector:* ships vector plus contrib, postgres:*
// ships contrib only, anything else is unknown.
func imageProvides(image string) (map[string]bool, bool) {
	repo := strings.ToLower(strings.TrimSpace(image))
	if i := strings.IndexByte(repo, '@'); i >= 0 {
		repo = repo[:i]
	}
	if i := strings.LastIndexByte(repo, ':'); i >= 0 && !strings.Contains(repo[i:], "/") {
		repo = repo[:i]
	}
	for _, p := range []string{"docker.io/", "index.docker.io/", "registry-1.docker.io/"} {
		repo = strings.TrimPrefix(repo, p)
	}
	repo = strings.TrimPrefix(repo, "library/")
	set := make(map[string]bool, len(contrib)+1)
	switch repo {
	case "pgvector/pgvector":
		set["vector"] = true
	case "postgres":
	default:
		return nil, false
	}
	for _, c := range contrib {
		set[c] = true
	}
	return set, true
}
