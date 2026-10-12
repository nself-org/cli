package importer

// Purpose: resolve secret inputs exclusively by environment name.
// Inputs: source flags. Outputs: in-memory secret. Constraints: no value in errors.
import (
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer/load"
	"os"
	"regexp"
)

// PartitionedPrefix starts the E520 message that names an unsupported partitioned source table.
const PartitionedPrefix = "source table is partitioned (not supported yet): "

var nameRe = regexp.MustCompile(`^[A-Za-z0-9_$.]{1,200}$`)

// safeName reports whether a schema.table name is plain enough to show in an error.
func safeName(n string) bool { return nameRe.MatchString(n) }

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (in Input) Secret(flag string) (string, error) {
	name := in.Values[flag]
	if !envName.MatchString(name) {
		return "", errs.New("E517", "source requires an environment variable name")
	}
	env := in.Env
	if env == nil {
		env = os.Getenv
	}
	value := env(name)
	if value == "" {
		return "", errs.New("E517", "source environment variable is empty")
	}
	return value, nil
}

// SystemTables excludes nSelf-owned data, with storage excluded only when enabled.
func SystemTables(schema, name string, storage bool) bool {
	return load.SystemTable(schema, name, storage)
}
