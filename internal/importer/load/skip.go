package load

// Purpose: let a source keep source-only archive entries out of the restore list.
// Inputs: the optional bundle member db/restore.skip (one entry kind per line) and the
// pg_restore list. Outputs: the list with those entries commented out.
// Constraints: only the kinds in skippable are accepted, so an untrusted bundle cannot hide
// tables or functions; the member is part of the hashed file list.
import (
	"errors"
	"github.com/nself-org/cli/internal/importer/verify"
	"github.com/nself-org/cli/internal/portable"
	"io"
	"regexp"
	"strings"
)

// RestoreSkipMember is the bundle member that names the archive entry kinds to leave out.
const RestoreSkipMember = "db/restore.skip"

// skippable are the entry kinds a source may leave out: row level security policies and the
// ENABLE ROW LEVEL SECURITY switch name roles and functions the target does not have.
var skippable = map[string]bool{"POLICY": true, "ROW SECURITY": true}

// SkipKinds reads the bundle's restore.skip member; a bundle without one skips nothing.
func SkipKinds(r *portable.Reader) ([]string, error) {
	if !verify.HasMember(r, RestoreSkipMember) {
		return nil, nil
	}
	f, err := r.Open(RestoreSkipMember)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, 4096))
	if err != nil {
		return nil, err
	}
	kinds := []string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !skippable[line] {
			return nil, errors.New("restore skip list names an unsupported entry kind")
		}
		kinds = append(kinds, line)
	}
	return kinds, nil
}

// dropEntries comments out every list line whose entry kind is in kinds.
func dropEntries(toc string, kinds []string) string {
	if len(kinds) == 0 {
		return toc
	}
	re := regexp.MustCompile(`^[0-9]+; [0-9]+ [0-9]+ (` + strings.Join(kinds, "|") + `) `)
	lines := strings.Split(toc, "\n")
	for i, line := range lines {
		if re.MatchString(line) {
			lines[i] = ";" + line
		}
	}
	return strings.Join(lines, "\n")
}
