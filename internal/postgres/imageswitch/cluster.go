package imageswitch

// cluster.go — the cluster document capcluster.sql prints, and the restore
// files built from it.
//
// Purpose: the old cluster's databases (owner, locale, ACL, settings) and roles
// are captured as one JSON line; the restore creates the same databases and
// replays what pg_restore does not carry (database ACLs and settings), and the
// verify compares the captured document byte for byte.
// Inputs: the JSON line. Outputs: clusterDoc, and the SQL files of the restore.
// Constraints: identifiers and literals are quoted here, never concatenated raw.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// clusterDoc is the parsed capcluster.sql output (fields the switch acts on).
type clusterDoc struct {
	Major       int      `json:"major"`
	Bootstrap   string   `json:"bootstrap"`
	Tablespaces []string `json:"tablespaces"`
	Databases   []dbDoc  `json:"databases"`
}

// dbDoc is one database of the cluster document.
type dbDoc struct {
	Name             string     `json:"name"`
	Owner            string     `json:"owner"`
	Provider         string     `json:"provider"`
	AllowConnections bool       `json:"allow_connections"`
	Collate          string     `json:"collate"`
	Ctype            string     `json:"ctype"`
	Create           string     `json:"create"`
	ExplicitACL      bool       `json:"explicit_acl"`
	ACL              []string   `json:"acl"`
	Settings         [][]string `json:"settings"`
}

// parseCluster reads the cluster document.
func parseCluster(doc string) (*clusterDoc, error) {
	var c clusterDoc
	if err := json.Unmarshal([]byte(strings.TrimSpace(doc)), &c); err != nil {
		return nil, fmt.Errorf("the cluster capture is unreadable: %w", err)
	}
	return &c, nil
}

// names returns the database names in capture order (sorted by name).
func (c *clusterDoc) names() []string {
	out := make([]string, len(c.Databases))
	for i, d := range c.Databases {
		out[i] = d.Name
	}
	return out
}

// hasDB reports whether the cluster has a database of that name.
func (c *clusterDoc) hasDB(name string) bool {
	for _, d := range c.Databases {
		if d.Name == name {
			return true
		}
	}
	return false
}

// collation returns the lc_collate and lc_ctype of the bootstrap database
// (postgres), else of the first database, else "C".
func (c *clusterDoc) collation() (collate, ctype string) {
	for _, d := range c.Databases {
		if d.Name == "postgres" {
			return d.Collate, d.Ctype
		}
	}
	if len(c.Databases) > 0 {
		return c.Databases[0].Collate, c.Databases[0].Ctype
	}
	return "C", "C"
}

// quoteIdent quotes an SQL identifier.
func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// quoteLiteral quotes an SQL string literal.
func quoteLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// listGUCs are the settings whose stored value is already a quoted list and is
// written back unquoted, as pg_dump does (a quoted "public, foo" would be one schema).
var listGUCs = map[string]bool{
	"datestyle": true, "search_path": true, "temp_tablespaces": true,
	"session_preload_libraries": true, "shared_preload_libraries": true,
	"local_preload_libraries": true, "synchronous_standby_names": true,
}

// restoreFiles holds the SQL the restore server runs and the statements it skipped.
type restoreFiles struct {
	Globals   string // pg_dumpall --globals-only without the bootstrap CREATE ROLE
	CreateDBs string // CREATE DATABASE for every database but "postgres"
	Replay    string // ownership of "postgres", database ACLs and settings, drop of an absent "postgres"
	Tolerated []string
}

// buildRestoreFiles turns the dump-time globals and cluster document into the
// restore server's SQL. Only the bootstrap role's CREATE ROLE is skipped (the
// role exists after initdb); every other statement must succeed.
func buildRestoreFiles(globals string, c *clusterDoc) restoreFiles {
	var rf restoreFiles
	skip := map[string]bool{
		"CREATE ROLE " + c.Bootstrap + ";":             true,
		"CREATE ROLE " + quoteIdent(c.Bootstrap) + ";": true,
	}
	var kept []string
	for _, line := range strings.Split(globals, "\n") {
		if skip[strings.TrimRight(line, "\r")] {
			rf.Tolerated = append(rf.Tolerated, "skipped `"+strings.TrimSpace(line)+"`: initdb already created the bootstrap role")
			continue
		}
		kept = append(kept, line)
	}
	rf.Globals = strings.Join(kept, "\n")
	var create, replay []string
	for _, d := range c.Databases {
		if d.Name != "postgres" {
			create = append(create, d.Create+";")
		} else {
			replay = append(replay, "ALTER DATABASE postgres OWNER TO "+quoteIdent(d.Owner)+";")
		}
		if d.ExplicitACL {
			replay = append(replay, "REVOKE ALL ON DATABASE "+quoteIdent(d.Name)+" FROM PUBLIC;")
			for _, g := range d.ACL {
				replay = append(replay, g+";")
			}
		}
		for _, set := range d.Settings {
			if len(set) != 2 {
				continue
			}
			name, val, ok := strings.Cut(set[1], "=")
			if !ok {
				continue
			}
			lit := quoteLiteral(val)
			if listGUCs[strings.ToLower(name)] {
				lit = val
			}
			if set[0] == "" {
				replay = append(replay, "ALTER DATABASE "+quoteIdent(d.Name)+" SET "+name+" TO "+lit+";")
			} else {
				replay = append(replay, "ALTER ROLE "+quoteIdent(set[0])+" IN DATABASE "+quoteIdent(d.Name)+" SET "+name+" TO "+lit+";")
			}
		}
	}
	if !c.hasDB("postgres") {
		replay = append(replay, "DROP DATABASE postgres;")
	}
	rf.CreateDBs = strings.Join(create, "\n") + "\n"
	rf.Replay = strings.Join(replay, "\n") + "\n"
	return rf
}
