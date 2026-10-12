package load

// Purpose: bind archive tables to the portable manifest before any restore.
// Inputs: pg_restore list and manifest. Outputs: refusal on undeclared/protected data.
// Constraints: identifiers validated; data-only archives cannot bypass the schema guard.
import (
	"errors"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/portable"
	"strings"
)

func validateArchive(toc string, m portable.Manifest) error {
	allowed := map[string]bool{}
	schemas := map[string]bool{}
	for _, s := range m.DB.Schemas {
		schemas[s] = true
	}
	for _, t := range m.DB.Tables {
		if !schemas[t.Schema] {
			return errors.New("bundle table outside declared schemas")
		}
		allowed[t.Schema+"."+t.Name] = true
	}
	for _, line := range strings.Split(toc, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			return errors.New("unparsable archive record")
		}
		if fields[3] != "TABLE" {
			continue
		}
		if len(fields) < 6 {
			return errors.New("unparsable archive table record")
		}
		if fields[4] == "ATTACH" {
			return errors.New("archive contains partition attachments (partitioned tables are not supported yet)")
		}
		index := 4
		if fields[4] == "DATA" {
			index = 5
		}
		if len(fields) <= index+1 {
			return errors.New("unparsable archive table record")
		}
		schema, name := fields[index], fields[index+1]
		if _, e := database.SanitizeIdentifier(schema); e != nil {
			return errors.New("unsafe archive schema")
		}
		if _, e := database.SanitizeIdentifier(name); e != nil {
			return errors.New("unsafe archive table")
		}
		if !allowed[schema+"."+name] || SystemTable(schema, name, false) {
			return errors.New("archive contains undeclared or protected tables")
		}
	}
	return nil
}
