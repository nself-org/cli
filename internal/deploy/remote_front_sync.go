package deploy

// Purpose: copy the staged front files (nginx/sites/<name>.conf) into the
//          fronting stack's directory (D-0425). rsyncList cannot be used: it
//          resolves the layout with ResolveDirs, which looks at names of the
//          fronting stack outside nginx/sites.
// Inputs:  a Remote, the front root, its stage dir, the relative files.
// Outputs: nil when every file was copied.
// Constraints: no --delete; one rsync per destination dir over bare file names
//              (as rsyncList); the layout is resolved first, then a missing
//              leaf (nginx/sites) is made with plain mkdir under the proven real
//              nginx dir (nginx itself is never created); a sites dir that is a
//              symlink to another dir under nginx is written through its real path. -c: files are compared by checksum, not size
//              and mtime (a restore leaves whole-second mtimes, so a same-size
//              new conf could otherwise be skipped).

import (
	"context"
	"fmt"
	"path"
	"sort"
)

func syncFrontFiles(ctx context.Context, r Remote, front, stage string, rels []string) error {
	// Resolve first: nothing is created before the nginx dirs are proven to stay inside the front root.
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
		} else if err := makeFrontLeaf(ctx, r, front, d); err != nil {
			return err
		}
		if err := rsyncFiles(ctx, r, []string{"-c"}, byDir[d], path.Join(stage, d), dest); err != nil {
			return err
		}
	}
	return nil
}

// makeFrontLeaf creates the last element of d (a missing nginx/sites) with a plain
// mkdir under the real, already proven parent. The parent itself is never created.
func makeFrontLeaf(ctx context.Context, r Remote, front, d string) error {
	script := fmt.Sprintf(`b=$(cd -P %s && pwd -P) || exit 1; cd -P "$b"/%s || exit 1; [ -d %s ] || (umask 022; mkdir %s)`,
		q(front), q(path.Dir(d)), q(path.Base(d)), q(path.Base(d)))
	if _, err := r.Run(ctx, shScript(script)); err != nil {
		return fmt.Errorf("creating %s in the fronting stack's directory on %s: %w", d, r.Label(), err)
	}
	return nil
}
