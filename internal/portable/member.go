package portable

import (
	"os"
	"strings"
)

// openMemberHandle walks a validated manifest path using directory handles.
// Inputs: the caller's resolved root and a safe relative member path.
// Outputs: a file owned by the caller. Constraints: no component follows links;
// each parent remains pinned across renames. SPORT: cli.portable-export v1.
func openMemberHandle(root, rel string) (*os.File, error) {
	parent, err := openMemberDirectory(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = parent.Close() }()
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		next, err := openMemberAt(parent, part, i < len(parts)-1)
		if err != nil {
			return nil, err
		}
		if i == len(parts)-1 {
			return next, nil
		}
		_ = parent.Close()
		parent = next
	}
	return nil, os.ErrNotExist
}
