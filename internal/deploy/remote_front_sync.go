package deploy

// Purpose: copy the staged front files (nginx/sites/<name>.conf) into the
//          fronting stack's directory (D-0425). rsyncList cannot be used: it
//          resolves the layout with ResolveDirs, which looks at names of the
//          fronting stack outside nginx/sites.
// Inputs:  a Remote, the front root, its stage dir, the relative files.
// Outputs: nil when every file was copied.
// Constraints: no --delete; one rsync per destination dir over bare file names
//              (as rsyncList); parents are made with umask 022, never
//              transferred; a dir behind a symlink that stays inside the front
//              root is written through its real path, re-resolved here right
//              before the write. -c: files are compared by checksum, not size
//              and mtime (a restore leaves whole-second mtimes, so a same-size
//              new conf could otherwise be skipped).

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
)

func syncFrontFiles(ctx context.Context, r Remote, front, stage string, rels []string) error {
	if dirs := layoutDirsOnly(rels); len(dirs) > 0 {
		mk := fmt.Sprintf(`cd %s && (umask 022; mkdir -p %s)`, q(front), strings.Join(quoteAll(dirs), " "))
		if _, err := r.Run(ctx, shScript(mk)); err != nil {
			return fmt.Errorf("creating the fronting stack's nginx dirs on %s: %w", r.Label(), err)
		}
	}
	linked, err := ResolveFrontDirs(ctx, r, front, rels)
	if err != nil {
		return err
	}
	byDir := map[string][]string{}
	for _, rel := range rels {
		byDir[path.Dir(rel)] = append(byDir[path.Dir(rel)], path.Base(rel))
	}
	dirs := make([]string, 0, len(byDir))
	for d := range byDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		dest := path.Join(front, d)
		if real, ok := linked[d]; ok {
			dest = real
		}
		if err := rsyncFiles(ctx, r, []string{"-c"}, byDir[d], path.Join(stage, d), dest); err != nil {
			return err
		}
	}
	return nil
}
