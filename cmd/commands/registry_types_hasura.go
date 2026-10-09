package commands

import "github.com/nself-org/cli/internal/hasura"

func init() {
	registerJSONType("db hasura schema", hasura.SchemaSnapshot{})
	registerJSONType("db hasura permissions", hasura.PermissionSnapshot{})
}
